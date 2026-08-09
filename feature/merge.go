package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/bare-devcontainer/decolint/containerdef"
	"github.com/tailscale/hujson"
)

// Merge resolves everything a devcontainer.json inherits and folds it into root in place, following
// the Dev Container specification's merge logic. The inputs are the Features referenced under
// "/features" (and, recursively, those their "dependsOn" names) and the metadata carried by the
// configuration's base image, resolved by [baseImageContributors]. Compose files named by
// "dockerComposeFile" are interpolated with localEnv as the environment (see [loadComposeService]).
//
// Every node Merge adds carries the byte offset of the key it was pulled in through, so findings on
// merged-in properties point at that reference. Any fetch or parse failure, or a dependency cycle,
// is returned as an error.
func Merge(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, localEnv map[string]string, root *hujson.Value) error {
	imageContribs, err := baseImageContributors(ctx, f, fsRoot, configDir, localEnv, root)
	if err != nil {
		return err
	}
	ordered, err := installSequence(ctx, f, fsRoot, configDir, root)
	if err != nil {
		return err
	}
	if len(imageContribs) == 0 && len(ordered) == 0 {
		return nil
	}

	state := newMergeState(root.Value.(*hujson.Object))
	// Image metadata is the lowest-precedence input of the specification's merge logic, so it is
	// applied before any Feature.
	for _, c := range imageContribs {
		state.apply(c)
	}
	for _, c := range ordered {
		state.apply(c)
	}
	state.finish()
	return nil
}

// baseImageContributors returns the metadata contributors of the configuration's base image. It
// tries the three declaration forms in order, and the first one root declares wins — even when it
// yields no contributors, so a later form is never used as a fallback:
//
//   - "dockerComposeFile" with "service" ([composeContributors])
//   - "build", or the legacy "dockerFile" ([dockerfileContributors])
//   - "image" ([imageContributors])
//
// Only one form is valid at a time, so the order matters only for an invalid config that declares
// several; it then resolves the one the real tooling would.
func baseImageContributors(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, localEnv map[string]string, root *hujson.Value) ([]*contributor, error) {
	contribs, declared, err := composeContributors(ctx, f, fsRoot, configDir, localEnv, root)
	if err != nil {
		return nil, err
	}
	if declared {
		return contribs, nil
	}
	contribs, declared, err = dockerfileContributors(ctx, f, fsRoot, configDir, root)
	if err != nil {
		return nil, err
	}
	if declared {
		return contribs, nil
	}
	contribs, err = imageContributors(ctx, f, root)
	if err != nil {
		return nil, err
	}
	return contribs, nil
}

// dockerfileContributors fetches the metadata the image built from the configuration's Dockerfile
// would carry, anchored at the key declaring the Dockerfile path. declared reports whether root
// declares a Dockerfile at all, which [baseImageContributors] uses to choose the base-image form.
func dockerfileContributors(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, root *hujson.Value) ([]*contributor, bool, error) {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, false, nil
	}
	build := containerdef.Build(obj)
	if build == nil {
		return nil, false, nil
	}
	path, anchor := build.Dockerfile, build.DockerfileDecl.KeyOffset
	src, err := readBounded(fsRoot, filepath.Join(configDir, path), maxDockerfileBytes)
	if err != nil {
		return nil, true, err
	}
	entries, err := f.FetchDockerfileMetadata(ctx, src, build.Args, nil, build.Target)
	if err != nil {
		return nil, true, fmt.Errorf("build %s: %w", path, err)
	}
	contribs := make([]*contributor, 0, len(entries))
	for _, md := range entries {
		contribs = append(contribs, &contributor{ref: path, anchor: anchor, md: md})
	}
	return contribs, true, nil
}

// readBounded reads the file at path through fsRoot, so its resolution cannot escape fsRoot's
// boundary, rejecting a file larger than maxBytes before reading it into memory.
func readBounded(fsRoot *os.Root, path string, maxBytes int64) ([]byte, error) {
	info, err := fsRoot.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxBytes)
	}
	src, err := fsRoot.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return src, nil
}

// imageContributors fetches the "devcontainer.metadata" label of the image named by "/image",
// anchored at the "image" key. It contributes nothing when root declares no usable image or the
// image carries no label.
func imageContributors(ctx context.Context, f *Fetcher, root *hujson.Value) ([]*contributor, error) {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, nil
	}
	image, decl, ok := containerdef.Image(obj)
	if !ok {
		return nil, nil
	}
	entries, err := f.FetchImageMetadata(ctx, image)
	if err != nil {
		return nil, err
	}
	anchor := decl.KeyOffset
	contribs := make([]*contributor, 0, len(entries))
	for _, md := range entries {
		contribs = append(contribs, &contributor{ref: image, anchor: anchor, md: md})
	}
	return contribs, nil
}

// installSequence resolves the Features referenced under "/features" of root, applies
// "overrideFeatureInstallOrder", and returns the resolved contributors in installation order. It
// returns an empty result when root declares no Features.
func installSequence(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, root *hujson.Value) ([]*contributor, error) {
	features := root.Find("/features")
	if features == nil {
		return nil, nil
	}
	obj, ok := features.Value.(*hujson.Object)
	if !ok {
		return nil, nil
	}

	var declared []*contributor
	for _, m := range obj.Members {
		name, ok := m.Name.Value.(hujson.Literal)
		if !ok || name.Kind() != '"' {
			continue
		}
		node, err := newNode(name.String(), parseOptions(m.Value), m.Name.StartOffset, configDir)
		if err != nil {
			return nil, err
		}
		declared = append(declared, node)
	}

	contribs, err := resolveAll(ctx, f, fsRoot, configDir, declared)
	if err != nil {
		return nil, err
	}
	if err := applyOverride(ctx, f, fsRoot, configDir, root, contribs); err != nil {
		return nil, err
	}
	return installOrder(contribs)
}

// newNode builds an unresolved contributor for a Feature reference requested with the given options,
// anchored at anchor. Only the fields that need no fetching are filled from the parsed reference;
// the digest and aliases are set later, once the Feature is fetched.
func newNode(ref string, options optionValue, anchor int, configDir string) (*contributor, error) {
	parsed, err := ParseRef(ref)
	if err != nil {
		return nil, err
	}
	c := &contributor{ref: ref, kind: parsed.Kind, options: options, anchor: anchor}
	switch parsed.Kind {
	case KindLocal:
		c.resolvedPath = filepath.Join(configDir, ref)
	case KindOCI:
		c.ociRef = parsed.OCI
	case KindTarball:
		c.tarballURI = ref
	}
	return c, nil
}

// resolveAll fetches every declared Feature and, recursively, the Features named by their
// "dependsOn". The result is the deduplicated set of contributors: two requests for the same Feature
// with different options are kept as distinct contributors, mirroring the specification's dependency
// graph. Soft dependencies ("installsAfter") are resolved for matching but are not pulled in.
//
// A Feature that is both declared directly and pulled in as a dependency anchors to its own
// declaration, so findings and inline suppressions land on its own "features" entry rather than on a
// dependent's.
//
// Dependency cycles are not rejected here; a duplicate is skipped without recursing, and a cycle
// surfaces later as an install-order error.
func resolveAll(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, declared []*contributor) ([]*contributor, error) {
	declaredAnchor := map[string]int{}
	for _, c := range declared {
		if _, ok := declaredAnchor[c.ref]; !ok {
			declaredAnchor[c.ref] = c.anchor
		}
	}

	var acc []*contributor
	worklist := slices.Clone(declared)
	for len(worklist) > 0 {
		current := worklist[0]
		worklist = worklist[1:]

		md, err := f.Fetch(ctx, current.ref, fsRoot, configDir)
		if err != nil {
			return nil, err
		}
		current.md = md
		current.digest = md.Digest
		current.aliases = md.Aliases

		if slices.ContainsFunc(acc, func(n *contributor) bool { return equals(n, current) }) {
			continue
		}

		for _, dep := range md.DependsOn {
			anchor := current.anchor
			// Prefer the dependency's own declaration so its findings land on the entry the user can act on.
			if a, ok := declaredAnchor[dep.Ref]; ok {
				anchor = a
			}
			node, err := newNode(dep.Ref, dep.Options, anchor, configDir)
			if err != nil {
				return nil, err
			}
			current.dependsOn = append(current.dependsOn, node)
			worklist = append(worklist, node)
		}
		for _, softRef := range md.InstallsAfter {
			node, err := newNode(softRef, optionValue{}, current.anchor, configDir)
			if err != nil {
				return nil, err
			}
			// Soft dependencies are not pulled into the merge, but their source information is still
			// needed to match them against the Features that are.
			softMD, err := f.Fetch(ctx, softRef, fsRoot, configDir)
			if err != nil {
				return nil, err
			}
			node.digest = softMD.Digest
			node.aliases = softMD.Aliases
			current.installsAfter = append(current.installsAfter, node)
		}
		acc = append(acc, current)
	}
	return acc, nil
}

// applyOverride raises the roundPriority of the contributors named by "overrideFeatureInstallOrder"
// so they install in an earlier round. The first listed Feature gets the highest priority; a Feature
// absent from the merge is a no-op.
func applyOverride(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, root *hujson.Value, contribs []*contributor) error {
	v := root.Find("/overrideFeatureInstallOrder")
	if v == nil {
		return nil
	}
	arr, ok := v.Value.(*hujson.Array)
	if !ok {
		return nil
	}

	var entries []string
	for _, e := range arr.Elements {
		if lit, ok := e.Value.(hujson.Literal); ok && lit.Kind() == '"' {
			entries = append(entries, lit.String())
		}
	}

	for i, ref := range entries {
		priority := len(entries) - i
		overrideNode, err := newNode(ref, optionValue{}, 0, configDir)
		if err != nil {
			return err
		}
		md, err := f.Fetch(ctx, ref, fsRoot, configDir)
		if err != nil {
			return err
		}
		overrideNode.digest = md.Digest
		overrideNode.aliases = md.Aliases
		for _, c := range contribs {
			if satisfiesSoftDependency(c, overrideNode) {
				c.roundPriority = max(c.roundPriority, priority)
			}
		}
	}
	return nil
}
