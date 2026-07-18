package feature

import (
	"context"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/tailscale/hujson"
)

// Merge fetches the Features referenced under "/features" of root, a devcontainer.json parsed from
// a file at fileDir within dir, and merges the properties they contribute into root in place,
// following the merge logic of the Dev Container specification. Features named by "dependsOn" are
// resolved recursively and contribute properties as well.
//
// dir and fileDir together locate the referencing devcontainer.json (see linter.Context.Dir and
// linter.Context.FileDir): a local Feature reference is resolved relative to fileDir and read
// through dir, so it cannot escape dir's boundary.
//
// Every node Merge adds to the tree carries the byte offset of the referencing Feature key in the
// original file, so findings on merged-in properties point at the Feature reference. Any fetch or
// parse failure is returned as an error.
func Merge(ctx context.Context, f *Fetcher, dir *os.Root, fileDir string, root *hujson.Value) error {
	features := root.Find("/features")
	if features == nil {
		return nil
	}
	obj, ok := features.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	var declared []*contributor
	for _, m := range obj.Members {
		name, ok := m.Name.Value.(hujson.Literal)
		if !ok || name.Kind() != '"' {
			continue
		}
		declared = append(declared, &contributor{ref: name.String(), anchor: m.Name.StartOffset})
	}

	contribs, err := resolveAll(ctx, f, dir, fileDir, declared)
	if err != nil {
		return err
	}
	ordered := installOrder(root, contribs)

	state := newMergeState(root.Value.(*hujson.Object))
	for _, c := range ordered {
		state.apply(c)
	}
	state.finish()
	return nil
}

// contributor is one resolved Feature that contributes properties to the effective configuration.
type contributor struct {
	// ref is the reference the Feature was fetched by.
	ref string
	// anchor is the byte offset, in the original file, of the "features" key that (directly or via
	// dependencies) pulled this Feature in.
	anchor int
	// declIdx is the declaration index of that key, used as an ordering tiebreak.
	declIdx int
	// deps are the refs of the Features named by this Feature's dependsOn.
	deps []string
	md   *Metadata
}

// id returns the identifier the Feature is matched by in "installsAfter" and
// "overrideFeatureInstallOrder": its reference without a version. The declared metadata ID is
// accepted as well.
func (c *contributor) matches(id string) bool {
	return id == refWithoutVersion(c.ref) || (c.md != nil && id == c.md.ID)
}

// displayID returns the identifier used for members synthesized on behalf of this Feature.
func (c *contributor) displayID() string {
	if c.md != nil && c.md.ID != "" {
		return c.md.ID
	}
	return c.ref
}

// refWithoutVersion strips the ":tag" and "@digest" suffixes off an OCI reference.
func refWithoutVersion(ref string) string {
	if at := strings.LastIndex(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		ref = ref[:colon]
	}
	return ref
}

// resolveAll fetches every declared Feature and, recursively, the Features they depend on. The
// result is in discovery order (dependencies before their dependents), deduplicated by reference.
func resolveAll(ctx context.Context, f *Fetcher, dir *os.Root, fileDir string, declared []*contributor) ([]*contributor, error) {
	seen := map[string]*contributor{}
	var out []*contributor

	var visit func(c *contributor, stack []string) error
	visit = func(c *contributor, stack []string) error {
		if slices.Contains(stack, c.ref) {
			return fmt.Errorf("feature dependency cycle: %s", strings.Join(append(stack, c.ref), " -> "))
		}
		if _, ok := seen[c.ref]; ok {
			return nil
		}
		md, err := f.Fetch(ctx, c.ref, dir, fileDir)
		if err != nil {
			return err
		}
		c.md = md
		seen[c.ref] = c
		stack = append(stack, c.ref)
		for _, dep := range md.DependsOn {
			c.deps = append(c.deps, dep)
			if err := visit(&contributor{ref: dep, anchor: c.anchor, declIdx: c.declIdx}, stack); err != nil {
				return err
			}
		}
		out = append(out, c)
		return nil
	}

	for i, c := range declared {
		c.declIdx = i
		if err := visit(c, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// installOrder sorts contribs into the order the Features would be installed in: dependencies
// always precede their dependents, "overrideFeatureInstallOrder" entries come first in the listed
// order, "installsAfter" preferences are honored when possible, and declaration order breaks the
// remaining ties. Later Features win merge conflicts, mirroring how a later installation
// overrides an earlier one.
func installOrder(root *hujson.Value, contribs []*contributor) []*contributor {
	override := map[string]int{}
	if v := root.Find("/overrideFeatureInstallOrder"); v != nil {
		if arr, ok := v.Value.(*hujson.Array); ok {
			for i, e := range arr.Elements {
				if lit, ok := e.Value.(hujson.Literal); ok && lit.Kind() == '"' {
					override[lit.String()] = i
				}
			}
		}
	}
	overrideIdx := func(c *contributor) int {
		for id, i := range override {
			if c.matches(id) {
				return i
			}
		}
		return math.MaxInt
	}

	emitted := map[string]bool{}
	byRef := map[string]*contributor{}
	for _, c := range contribs {
		byRef[c.ref] = c
	}
	// ready reports whether every dependency of c is already emitted.
	ready := func(c *contributor) bool {
		for _, dep := range c.deps {
			if !emitted[dep] {
				return false
			}
		}
		return true
	}
	// softSatisfied reports whether every Feature named by c's installsAfter that is part of this
	// merge is already emitted.
	softSatisfied := func(c *contributor) bool {
		for _, id := range c.md.InstallsAfter {
			for _, other := range contribs {
				if other != c && other.matches(id) && !emitted[other.ref] {
					return false
				}
			}
		}
		return true
	}

	out := make([]*contributor, 0, len(contribs))
	for len(out) < len(contribs) {
		var best *contributor
		bestSoft := false
		for _, c := range contribs {
			if emitted[c.ref] || !ready(c) {
				continue
			}
			soft := softSatisfied(c)
			if best == nil || (soft && !bestSoft) {
				best, bestSoft = c, soft
				continue
			}
			if soft != bestSoft {
				continue
			}
			if oi, boi := overrideIdx(c), overrideIdx(best); oi != boi {
				if oi < boi {
					best = c
				}
				continue
			}
			if c.declIdx < best.declIdx {
				best = c
			}
		}
		// resolveAll rejects dependency cycles, so a ready contributor always exists (installsAfter
		// is only a preference and never blocks).
		emitted[best.ref] = true
		out = append(out, best)
	}
	return out
}
