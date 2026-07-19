package feature

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tailscale/hujson"
	"oras.land/oras-go/v2/registry"
)

// contributor is one resolved Feature that contributes properties to the effective configuration and
// participates in install-order resolution.
type contributor struct {
	// ref is the reference the Feature was requested by: a "features" key or a "dependsOn" key.
	ref string
	// kind classifies ref, selecting the source-type-specific comparison used to order and
	// deduplicate Features.
	kind RefKind
	// options is the option set ref was requested with (the "features" value or a "dependsOn"
	// entry), normalized for comparison. Two requests for the same Feature with different options are
	// distinct contributors, as the specification's install-order algorithm treats them.
	options optionValue

	// resolvedPath is the local Feature directory, set for KindLocal.
	resolvedPath string
	// ociRef is the parsed registry reference, set for KindOCI.
	ociRef registry.Reference
	// digest is the resolved manifest digest, set for KindOCI. It distinguishes otherwise identical
	// references and is the final ordering tiebreak.
	digest string
	// aliases are the Feature's identifiers ([id, ...legacyIds]) used to match a renamed Feature
	// named by "installsAfter" or "overrideFeatureInstallOrder"; set for KindOCI.
	aliases []string
	// tarballURI is the direct HTTPS URI, set for KindTarball.
	tarballURI string

	// dependsOn and installsAfter are the resolved hard and soft dependency edges. A hard dependency
	// must install before this Feature; a soft dependency only orders this Feature after it when both
	// are part of the merge.
	dependsOn     []*contributor
	installsAfter []*contributor
	// roundPriority raises a Feature into an earlier install round. "overrideFeatureInstallOrder"
	// assigns it; the effective value is the maximum contributed. Zero when no override applies.
	roundPriority int

	// anchor is the byte offset, in the original file, of the "features" key that (directly or via
	// dependencies) pulled this Feature in, so findings on merged-in properties point at it.
	anchor int
	// md is the fetched metadata, the source of truth for the properties the Feature contributes.
	md *Metadata
}

// displayID returns the identifier used for members synthesized on behalf of this Feature.
func (c *contributor) displayID() string {
	if c.md != nil && c.md.ID != "" {
		return c.md.ID
	}
	return c.ref
}

// optionValue is a Feature's option set, normalized for the install-order comparison. It mirrors the
// reference implementation's shape: a string, a boolean, or an object mapping option names to scalar
// values.
type optionValue struct {
	// kind is 's' for a string, 'b' for a boolean, or 'o' for an object (the default, including the
	// empty option set "{}").
	kind byte
	str  string
	b    bool
	obj  map[string]optScalar
}

// optScalar is a single option value inside an object: a string, a boolean, or undefined (a null or
// non-scalar value).
type optScalar struct {
	// kind is 's' for a string, 'b' for a boolean, or 'u' for undefined.
	kind byte
	str  string
	b    bool
}

// parseOptions normalizes a "features" value or a "dependsOn" entry into an optionValue. A value that
// is neither a string nor a boolean (including "{}") is treated as an object.
func parseOptions(v hujson.Value) optionValue {
	switch t := v.Value.(type) {
	case hujson.Literal:
		switch t.Kind() {
		case '"':
			return optionValue{kind: 's', str: t.String()}
		case 't', 'f':
			return optionValue{kind: 'b', b: t.Bool()}
		}
	case *hujson.Object:
		obj := map[string]optScalar{}
		for _, m := range t.Members {
			if name, ok := m.Name.Value.(hujson.Literal); ok && name.Kind() == '"' {
				obj[name.String()] = parseScalar(m.Value)
			}
		}
		return optionValue{kind: 'o', obj: obj}
	}
	return optionValue{kind: 'o', obj: map[string]optScalar{}}
}

// parseScalar normalizes an option value inside an object.
func parseScalar(v hujson.Value) optScalar {
	if lit, ok := v.Value.(hujson.Literal); ok {
		switch lit.Kind() {
		case '"':
			return optScalar{kind: 's', str: lit.String()}
		case 't', 'f':
			return optScalar{kind: 'b', b: lit.Bool()}
		}
	}
	return optScalar{kind: 'u'}
}

// optionsCompareTo orders two option sets: strings and booleans compare directly, objects compare by
// size then key-by-key, and differing kinds fall back to their type names. It mirrors the reference
// implementation's optionsCompareTo.
func optionsCompareTo(a, b optionValue) int {
	switch {
	case a.kind == 's' && b.kind == 's':
		return strings.Compare(a.str, b.str)
	case a.kind == 'b' && b.kind == 'b':
		return boolCompare(a.b, b.b)
	case a.kind == 'o' && b.kind == 'o':
		if len(a.obj) != len(b.obj) {
			return len(a.obj) - len(b.obj)
		}
		aKeys := sortedKeys(a.obj)
		bKeys := sortedKeys(b.obj)
		for i := range aKeys {
			if aKeys[i] != bKeys[i] {
				return strings.Compare(aKeys[i], bKeys[i])
			}
			if v := scalarCompare(a.obj[aKeys[i]], b.obj[bKeys[i]]); v != 0 {
				return v
			}
		}
		return 0
	}
	return strings.Compare(optionTypeName(a.kind), optionTypeName(b.kind))
}

// scalarCompare orders two object option values. An undefined value sorts after a defined one.
func scalarCompare(a, b optScalar) int {
	switch {
	case a.kind == 's' && b.kind == 's':
		return strings.Compare(a.str, b.str)
	case a.kind == 'b' && b.kind == 'b':
		return boolCompare(a.b, b.b)
	case a.kind == 'u' || b.kind == 'u':
		if a.kind == b.kind {
			return 0
		}
		if a.kind == 'u' {
			return 1
		}
		return -1
	}
	return 0
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}

func optionTypeName(kind byte) string {
	switch kind {
	case 's':
		return "string"
	case 'b':
		return "boolean"
	default:
		return "object"
	}
}

func sortedKeys(m map[string]optScalar) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// compareTo defines the total order the specification sorts each install round by. A negative result
// places a before b, positive places a after b, and zero treats them as equal (the basis for
// deduplication). String comparison stands in for the reference implementation's locale-aware
// comparison; the two agree on the lowercase, digit, and digest identifiers Features use.
func compareTo(a, b *contributor) int {
	if a.kind != b.kind {
		return strings.Compare(a.ref, b.ref)
	}
	switch a.kind {
	case KindOCI:
		// Identical content requested with identical options is the same Feature.
		if a.digest == b.digest && optionsCompareTo(a.options, b.options) == 0 {
			return 0
		}
		if v := ociResourceCompareTo(a, b); v != 0 {
			return v
		}
		if aTag, bTag := ociTag(a.ociRef), ociTag(b.ociRef); aTag != "" && bTag != "" && aTag != bTag {
			return strings.Compare(aTag, bTag)
		}
		if v := optionsCompareTo(a.options, b.options); v != 0 {
			return v
		}
		return strings.Compare(a.digest, b.digest)
	case KindTarball:
		if v := strings.Compare(a.tarballURI, b.tarballURI); v != 0 {
			return v
		}
		return optionsCompareTo(a.options, b.options)
	default: // KindLocal
		if v := strings.Compare(a.resolvedPath, b.resolvedPath); v != 0 {
			return v
		}
		return optionsCompareTo(a.options, b.options)
	}
}

// ociResourceCompareTo orders two OCI Features by registry+namespace then by identifier, treating a
// pair that shares any alias (a renamed Feature) as the same resource.
func ociResourceCompareTo(a, b *contributor) int {
	aRegistryNamespace := a.ociRef.Registry + "/" + ociNamespace(a.ociRef.Repository)
	bRegistryNamespace := b.ociRef.Registry + "/" + ociNamespace(b.ociRef.Repository)
	if aRegistryNamespace != bRegistryNamespace {
		return strings.Compare(aRegistryNamespace, bRegistryNamespace)
	}
	aID, bID := ociID(a.ociRef.Repository), ociID(b.ociRef.Repository)
	bAliases := aliasesOr(b, bID)
	for _, x := range aliasesOr(a, aID) {
		if slices.Contains(bAliases, x) {
			return 0
		}
	}
	return strings.Compare(aID, bID)
}

// equals reports whether a and b are the same Feature, the basis for deduplicating the dependency
// graph and for matching a hard dependency against an already-installed Feature.
func equals(a, b *contributor) bool {
	return a.kind == b.kind && compareTo(a, b) == 0
}

// satisfiesSoftDependency reports whether node fulfills soft, a Feature named by an "installsAfter"
// or "overrideFeatureInstallOrder" entry. Unlike equals it ignores options and matches a renamed
// Feature through soft's aliases.
func satisfiesSoftDependency(node, soft *contributor) bool {
	if node.kind != soft.kind {
		return false
	}
	switch node.kind {
	case KindOCI:
		nodeResource := node.ociRef.Registry + "/" + node.ociRef.Repository
		if nodeResource == soft.ociRef.Registry+"/"+soft.ociRef.Repository {
			return true
		}
		softRegistryNamespace := soft.ociRef.Registry + "/" + ociNamespace(soft.ociRef.Repository)
		for _, legacy := range soft.aliases {
			if softRegistryNamespace+"/"+legacy == nodeResource {
				return true
			}
		}
		return false
	case KindTarball:
		return node.tarballURI == soft.tarballURI
	default: // KindLocal
		return node.resolvedPath == soft.resolvedPath
	}
}

// ociNamespace returns the repository without its final path segment (the identifier).
func ociNamespace(repository string) string {
	if i := strings.LastIndex(repository, "/"); i >= 0 {
		return repository[:i]
	}
	return ""
}

// ociID returns the final path segment of the repository, the Feature's identifier.
func ociID(repository string) string {
	if i := strings.LastIndex(repository, "/"); i >= 0 {
		return repository[i+1:]
	}
	return repository
}

// ociTag returns the tag of ref, or "" when ref is pinned by digest (which contains an algorithm
// separator that a tag cannot).
func ociTag(ref registry.Reference) string {
	if strings.Contains(ref.Reference, ":") {
		return ""
	}
	return ref.Reference
}

// aliasesOr returns c's aliases, or the single identifier id when it has none.
func aliasesOr(c *contributor, id string) []string {
	if len(c.aliases) > 0 {
		return c.aliases
	}
	return []string{id}
}

// installOrder returns nodes in the order the Features install in, following the Dev Container
// specification's round-based dependency installation algorithm: dependencies (both "dependsOn" and
// "installsAfter") always precede their dependents, a higher roundPriority (assigned by
// "overrideFeatureInstallOrder") installs in an earlier round, and remaining ties break by
// compareTo. It reports an error when a dependency cycle leaves a round with no installable Feature.
//
// Later Features win merge conflicts, mirroring how a later installation overrides an earlier one.
func installOrder(nodes []*contributor) ([]*contributor, error) {
	// Drop soft-dependency edges to Features absent from this merge; they never gate a round.
	for _, n := range nodes {
		n.installsAfter = slices.DeleteFunc(n.installsAfter, func(soft *contributor) bool {
			return !slices.ContainsFunc(nodes, func(m *contributor) bool {
				return satisfiesSoftDependency(m, soft)
			})
		})
	}

	worklist := slices.Clone(nodes)
	var order []*contributor
	installed := func(dep *contributor) bool {
		return slices.ContainsFunc(order, func(o *contributor) bool { return equals(o, dep) })
	}
	softInstalled := func(dep *contributor) bool {
		return slices.ContainsFunc(order, func(o *contributor) bool { return satisfiesSoftDependency(o, dep) })
	}

	for len(worklist) > 0 {
		var round []*contributor
		for _, n := range worklist {
			ready := (len(n.dependsOn) == 0 && len(n.installsAfter) == 0) ||
				(every(n.dependsOn, installed) && every(n.installsAfter, softInstalled))
			if ready {
				round = append(round, n)
			}
		}
		if len(round) == 0 {
			return nil, fmt.Errorf("feature dependency cycle among %s", strings.Join(refsOf(worklist), ", "))
		}

		// Commit only the highest-priority eligible Features this round; requeue the rest so
		// "overrideFeatureInstallOrder" (roundPriority) is honored alongside the dependency graph.
		maxPriority := round[0].roundPriority
		for _, n := range round {
			maxPriority = max(maxPriority, n.roundPriority)
		}
		round = slices.DeleteFunc(round, func(n *contributor) bool { return n.roundPriority != maxPriority })
		worklist = slices.DeleteFunc(worklist, func(n *contributor) bool { return slices.Contains(round, n) })

		slices.SortStableFunc(round, compareTo)
		order = append(order, round...)
	}
	return order, nil
}

// every reports whether f holds for every element of s.
func every[T any](s []T, f func(T) bool) bool {
	for _, x := range s {
		if !f(x) {
			return false
		}
	}
	return true
}

// refsOf returns the references of the given contributors, for diagnostics.
func refsOf(cs []*contributor) []string {
	refs := make([]string, len(cs))
	for i, c := range cs {
		refs[i] = c.ref
	}
	return refs
}
