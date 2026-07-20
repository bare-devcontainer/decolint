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
	// entry), reduced to its optionValue form for comparison. Two requests for the same Feature with
	// different options are distinct contributors, as the specification's install-order algorithm
	// treats them.
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

// valueKind classifies an optionValue or optScalar. kindObject is first so it is the zero value: a
// contributor built without options is left as optionValue{}, which then reads as the default empty
// object. Such nodes (soft-dependency and override entries) are matched by Feature identity rather
// than by their options, so their option value is never actually compared.
type valueKind int

const (
	kindObject valueKind = iota // objects only, i.e. optionValue
	kindString
	kindBool
	kindUndefined // undefined scalars only, i.e. optScalar
)

// optionValue is a Feature's option set reduced to the canonical form the install-order comparison
// (optionsCompareTo) distinguishes: a string, a boolean, or an object mapping option names to
// optScalar values. This mirrors the reference implementation's shape. Everything the comparison
// does not look at — JSON formatting, comments, and object key order — is dropped, and any value
// that is not a string, boolean, or object collapses to the empty object. See parseOptions for the
// exact mapping.
type optionValue struct {
	// kind is kindString, kindBool, or kindObject (the default, including the empty option set "{}").
	kind valueKind
	str  string
	b    bool
	obj  map[string]optScalar
}

// optScalar is a single option value inside an object: a string, a boolean, or undefined (a null or
// non-scalar value).
type optScalar struct {
	// kind is kindString, kindBool, or kindUndefined.
	kind valueKind
	str  string
	b    bool
}

// parseOptions reduces a raw "features" value or "dependsOn" entry to an optionValue: a string or
// boolean literal keeps its kind; an object maps each member value through parseScalar; any other
// value (a number, null, or array) collapses to the empty object, as "{}" also does.
func parseOptions(v hujson.Value) optionValue {
	switch t := v.Value.(type) {
	case hujson.Literal:
		switch t.Kind() {
		case '"':
			return optionValue{kind: kindString, str: t.String()}
		case 't', 'f':
			return optionValue{kind: kindBool, b: t.Bool()}
		}
	case *hujson.Object:
		obj := map[string]optScalar{}
		for _, m := range t.Members {
			if name, ok := m.Name.Value.(hujson.Literal); ok && name.Kind() == '"' {
				obj[name.String()] = parseScalar(m.Value)
			}
		}
		return optionValue{kind: kindObject, obj: obj}
	}
	return optionValue{kind: kindObject, obj: map[string]optScalar{}}
}

// parseScalar reduces an object member value to an optScalar: a string or boolean literal keeps its
// kind; any other value (a number, null, array, or nested object) becomes undefined.
func parseScalar(v hujson.Value) optScalar {
	if lit, ok := v.Value.(hujson.Literal); ok {
		switch lit.Kind() {
		case '"':
			return optScalar{kind: kindString, str: lit.String()}
		case 't', 'f':
			return optScalar{kind: kindBool, b: lit.Bool()}
		}
	}
	return optScalar{kind: kindUndefined}
}

// optionsCompareTo orders two option sets: strings and booleans compare directly, objects compare by
// size then key-by-key, and differing kinds fall back to their type names. It mirrors the reference
// implementation's optionsCompareTo.
func optionsCompareTo(a, b optionValue) int {
	switch {
	case a.kind == kindString && b.kind == kindString:
		return strings.Compare(a.str, b.str)
	case a.kind == kindBool && b.kind == kindBool:
		return boolCompare(a.b, b.b)
	case a.kind == kindObject && b.kind == kindObject:
		if len(a.obj) != len(b.obj) {
			// The spec prose orders the "greatest number of user-defined options" first, but the
			// reference implementation (optionsCompareTo) sorts fewest first; we follow the
			// implementation.
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
	// The spec prose is silent on comparing options of different kinds; the reference implementation
	// (optionsCompareTo) falls back to comparing the JavaScript `typeof` names, so we compare the
	// equivalent type names ("boolean" < "object" < "string").
	return strings.Compare(optionTypeName(a.kind), optionTypeName(b.kind))
}

// scalarCompare orders two object option values. An undefined value sorts after a defined one.
func scalarCompare(a, b optScalar) int {
	switch {
	case a.kind == kindString && b.kind == kindString:
		return strings.Compare(a.str, b.str)
	case a.kind == kindBool && b.kind == kindBool:
		return boolCompare(a.b, b.b)
	case a.kind == kindUndefined || b.kind == kindUndefined:
		if a.kind == b.kind {
			return 0
		}
		if a.kind == kindUndefined {
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

func optionTypeName(kind valueKind) string {
	switch kind {
	case kindString:
		return "string"
	case kindBool:
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
		// The spec prose only orders Features within a single source type; it is silent on comparing
		// across types. The reference implementation (compareTo) falls back to comparing the requested
		// reference strings (userFeatureId); we follow the implementation.
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
		// The spec prose orders tags "oldest to newest" with `latest` newest, but the reference
		// implementation compares them lexicographically (no semver, no `latest` special case); we
		// follow the implementation.
		if aTag, bTag := ociTag(a.ociRef), ociTag(b.ociRef); aTag != "" && bTag != "" && aTag != bTag {
			return strings.Compare(aTag, bTag)
		}
		if v := optionsCompareTo(a.options, b.options); v != 0 {
			return v
		}
		return strings.Compare(a.digest, b.digest)
	case KindTarball:
		// The spec prose keys tarball identity on the tgz content hash, but the reference
		// implementation keys on the tarball URI string; we follow the implementation.
		if v := strings.Compare(a.tarballURI, b.tarballURI); v != 0 {
			return v
		}
		return optionsCompareTo(a.options, b.options)
	default: // KindLocal
		// The spec prose says each local Feature is "unique and not equal to any other", but the
		// reference implementation treats same path + same options as equal (deduplicated); we
		// follow the implementation, so identical path and options return 0 here.
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
