package feature

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/tailscale/hujson"
)

// Merge fetches the Features referenced under "/features" of root, a devcontainer.json parsed from
// a file at configDir within fsRoot, and merges the properties they contribute into root in place,
// following the merge logic of the Dev Container specification. Features named by "dependsOn" are
// resolved recursively and contribute properties as well.
//
// fsRoot and configDir together locate the referencing devcontainer.json (fsRoot is
// discovery.ConfigFile.Root and configDir is the directory of its Path): a local Feature reference
// is resolved relative to configDir and read through fsRoot, so it cannot escape fsRoot's boundary.
//
// Every node Merge adds to the tree carries the byte offset of the referencing Feature key in the
// original file, so findings on merged-in properties point at the Feature reference. Any fetch or
// parse failure, or a dependency cycle, is returned as an error.
func Merge(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, root *hujson.Value) error {
	ordered, err := installSequence(ctx, f, fsRoot, configDir, root)
	if err != nil {
		return err
	}
	if len(ordered) == 0 {
		return nil
	}

	state := newMergeState(root.Value.(*hujson.Object))
	for _, c := range ordered {
		state.apply(c)
	}
	state.finish()
	return nil
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
