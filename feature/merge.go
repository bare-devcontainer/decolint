package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tailscale/hujson"
)

// Merge fetches the Features referenced under "/features" of root, a devcontainer.json parsed from
// a file at configDir within fsRoot, along with the Dev Container metadata carried by the
// "devcontainer.metadata" label of the configuration's base image, and merges the properties they
// contribute into root in place, following the merge logic of the Dev Container specification.
// Features named by "dependsOn" are resolved recursively and contribute properties as well.
//
// The base image is the image named by "/image", or, for a Dockerfile configuration, the image
// built from the Dockerfile declared by "/build" (or the legacy "/dockerFile" property): its LABEL
// instructions and the label inherited from the base image its FROM names both contribute. A base
// image reachable only through "dockerComposeFile" is not resolved.
//
// fsRoot and configDir together locate the referencing devcontainer.json (fsRoot is
// discovery.ConfigFile.Root and configDir is the directory of its Path): a local Feature reference
// or a Dockerfile is resolved relative to configDir and read through fsRoot, so it cannot escape
// fsRoot's boundary.
//
// Every node Merge adds to the tree carries the byte offset of the key it was pulled in through
// (the referencing Feature key, or the "image" or "dockerfile" key for image metadata) in the
// original file, so findings on merged-in properties point at that reference. Any fetch or parse
// failure, or a dependency cycle, is returned as an error.
func Merge(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, root *hujson.Value) error {
	imageContribs, err := baseImageContributors(ctx, f, fsRoot, configDir, root)
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

// baseImageContributors returns the metadata contributors of the configuration's base image: the
// image built from the Dockerfile declared by "/build" (or the legacy "/dockerFile" property), or
// the image named by "/image". A declared Dockerfile takes precedence over "image", matching the
// reference implementation, which builds the Dockerfile when both are present.
func baseImageContributors(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, root *hujson.Value) ([]*contributor, error) {
	contribs, declared, err := dockerfileContributors(ctx, f, fsRoot, configDir, root)
	if err != nil || declared {
		return contribs, err
	}
	return imageContributors(ctx, f, root)
}

// dockerfileContributors fetches the metadata the image built from the configuration's Dockerfile
// would carry and returns one contributor per entry, in label order, anchored at the key declaring
// the Dockerfile path. declared reports whether root declares a Dockerfile at all, so the caller
// falls back to "/image" only when it does not; a declared Dockerfile that cannot be resolved at
// lint time (a variable substitution in its path or target) contributes nothing.
func dockerfileContributors(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, root *hujson.Value) ([]*contributor, bool, error) {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, false, nil
	}
	path, anchor, ok := dockerfilePath(obj)
	if !ok {
		return nil, false, nil
	}
	// Variable substitutions resolve at container creation time; linting cannot know their values,
	// so such a Dockerfile is skipped rather than rejected.
	if strings.Contains(path, "${") {
		return nil, true, nil
	}
	args, target, ok := buildOptions(obj)
	if !ok {
		return nil, true, nil
	}
	// Without a filesystem boundary there is nothing to read the Dockerfile through; a caller that
	// passes no root has no local files at all.
	if fsRoot == nil {
		return nil, true, nil
	}
	src, err := readDockerfile(fsRoot, filepath.Join(configDir, path))
	if err != nil {
		return nil, true, err
	}
	entries, err := f.FetchDockerfileMetadata(ctx, src, args, target)
	if err != nil {
		return nil, true, fmt.Errorf("build %s: %w", path, err)
	}
	contribs := make([]*contributor, 0, len(entries))
	for _, md := range entries {
		contribs = append(contribs, &contributor{ref: path, anchor: anchor, md: md})
	}
	return contribs, true, nil
}

// dockerfilePath returns the Dockerfile path root declares, with the byte offset of the declaring
// key. The Dev Container schema defines two mutually exclusive Dockerfile forms: the top-level
// "dockerFile" property and the nested "build.dockerfile". The reference implementation prefers the
// top-level property (getDockerfile: 'dockerFile' in config ? config.dockerFile :
// config.build.dockerfile), so it is checked first; a valid configuration declares only one.
func dockerfilePath(obj *hujson.Object) (string, int, bool) {
	if i := findMember(obj, "dockerFile"); i >= 0 {
		if lit, ok := obj.Members[i].Value.Value.(hujson.Literal); ok && lit.Kind() == '"' {
			return lit.String(), obj.Members[i].Name.StartOffset, true
		}
	}
	if i := findMember(obj, "build"); i >= 0 {
		if buildObj, ok := obj.Members[i].Value.Value.(*hujson.Object); ok {
			if j := findMember(buildObj, "dockerfile"); j >= 0 {
				if lit, ok := buildObj.Members[j].Value.Value.(hujson.Literal); ok && lit.Kind() == '"' {
					return lit.String(), buildObj.Members[j].Name.StartOffset, true
				}
			}
		}
	}
	return "", 0, false
}

// buildOptions extracts the "args" and "target" of the "/build" object. An arg whose value carries
// a variable substitution cannot be known at lint time and is dropped, approximating a build where
// the arg is unset so the Dockerfile's own ARG default applies. A target carrying one leaves the
// built stage itself unknowable, so ok is false and the whole Dockerfile is skipped.
func buildOptions(obj *hujson.Object) (args map[string]string, target string, ok bool) {
	i := findMember(obj, "build")
	if i < 0 {
		return nil, "", true
	}
	buildObj, isObj := obj.Members[i].Value.Value.(*hujson.Object)
	if !isObj {
		return nil, "", true
	}
	if j := findMember(buildObj, "args"); j >= 0 {
		if argsObj, isObj := buildObj.Members[j].Value.Value.(*hujson.Object); isObj {
			for _, m := range argsObj.Members {
				name, nameOK := m.Name.Value.(hujson.Literal)
				val, valOK := m.Value.Value.(hujson.Literal)
				if !nameOK || name.Kind() != '"' || !valOK || val.Kind() != '"' || strings.Contains(val.String(), "${") {
					continue
				}
				if args == nil {
					args = map[string]string{}
				}
				args[name.String()] = val.String()
			}
		}
	}
	if j := findMember(buildObj, "target"); j >= 0 {
		if lit, isStr := buildObj.Members[j].Value.Value.(hujson.Literal); isStr && lit.Kind() == '"' {
			target = lit.String()
			if strings.Contains(target, "${") {
				return nil, "", false
			}
		}
	}
	return args, target, true
}

// readDockerfile reads the Dockerfile at path through fsRoot, so its resolution cannot escape
// fsRoot's boundary.
func readDockerfile(fsRoot *os.Root, path string) ([]byte, error) {
	info, err := fsRoot.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() > maxDockerfileBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxDockerfileBytes)
	}
	src, err := fsRoot.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return src, nil
}

// imageContributors fetches the "devcontainer.metadata" label of the image named by "/image" of
// root and returns one contributor per metadata entry, in label order, anchored at the "image"
// key. There is nothing to contribute when root declares no image, when the image is not a plain
// string, or when the image carries no label.
func imageContributors(ctx context.Context, f *Fetcher, root *hujson.Value) ([]*contributor, error) {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, nil
	}
	i := findMember(obj, "image")
	if i < 0 {
		return nil, nil
	}
	lit, ok := obj.Members[i].Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil, nil
	}
	image := lit.String()
	// Variable substitutions resolve at container creation time; linting cannot know their values,
	// so such an image is skipped rather than rejected.
	if strings.Contains(image, "${") {
		return nil, nil
	}
	entries, err := f.FetchImageMetadata(ctx, image)
	if err != nil {
		return nil, err
	}
	anchor := obj.Members[i].Name.StartOffset
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
// anchored at anchor. Its source-type-specific fields that do not require fetching (the local path,
// the OCI reference, the tarball URI) are filled in; the digest and aliases are set once fetched.
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
