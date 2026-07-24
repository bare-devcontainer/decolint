package schema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agext/levenshtein"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// printer renders the library's built-in error wording for kinds decolint does not phrase itself.
var printer = message.NewPrinter(language.English)

// leaf is one terminal validation error: a node in the error tree with no further causes.
type leaf struct {
	loc    []string // instance location: unescaped JSON Pointer segments
	kind   jsonschema.ErrorKind
	schema string // absolute keyword location the error came from
}

// diagnostics turns a validation error tree into positioned diagnostics.
//
// The tree is reduced to its relevant leaves (see [selectLeaves]), which fall into two classes:
// value errors (wrong type, bad enum, missing required property) and unknown-property errors. When
// any value error is present the unknown-property leaves are dropped: a failed combinator leaves
// every property unevaluated, so the schema reports each one as unknown, which is noise beside the
// real error. Unknown-property leaves are surfaced only when they are the sole reason validation
// failed — the case of a genuine typo or an unsupported property.
func diagnostics(root *jsonschema.ValidationError, props propertyNames, suppressExtensions bool, offsetFor func(loc []string) int) []Diagnostic {
	leaves := selectLeaves(root)

	var value, unknown []leaf
	for _, l := range leaves {
		if isUnknownProperty(l) {
			unknown = append(unknown, l)
		} else if !isStructural(l.kind) {
			value = append(value, l)
		}
	}

	var diags []Diagnostic
	if len(value) > 0 {
		for _, l := range value {
			diags = append(diags, Diagnostic{Message: valueMessage(l), Offset: offsetFor(l.loc)})
		}
	} else {
		for _, l := range unknown {
			diags = append(diags, unknownDiagnostics(l, props, suppressExtensions, offsetFor)...)
		}
	}
	return dedupe(diags)
}

// selectLeaves reduces an error tree to the leaves worth reporting. At an "oneOf"/"anyOf" node it
// keeps only the single best-matching branch instead of every branch: a devcontainer.json that is,
// say, an image container fails the Dockerfile and Compose branches too, and those failures are
// noise. Every other node — "allOf" and the structural wrappers — contributes all of its children,
// since each of their constraints genuinely applies.
func selectLeaves(e *jsonschema.ValidationError) []leaf {
	if len(e.Causes) == 0 {
		return []leaf{{loc: e.InstanceLocation, kind: e.ErrorKind, schema: e.SchemaURL}}
	}
	switch e.ErrorKind.(type) {
	case *kind.OneOf, *kind.AnyOf:
		var best []leaf
		first := true
		for _, c := range e.Causes {
			branch := selectLeaves(c)
			if first || betterBranch(branch, best) {
				best, first = branch, false
			}
		}
		return best
	default:
		var out []leaf
		for _, c := range e.Causes {
			out = append(out, selectLeaves(c)...)
		}
		return out
	}
}

// betterBranch reports whether the candidate branch is a closer match to the document than the
// incumbent. A branch matches better when it fails on fewer constraints; ties break toward the
// branch with fewer "unknown property" failures (a missing field is a nearer miss than a wrong
// shape), then toward the more deeply located failure, then lexically for determinism.
func betterBranch(candidate, incumbent []leaf) bool {
	if len(candidate) != len(incumbent) {
		return len(candidate) < len(incumbent)
	}
	if cu, iu := countUnknown(candidate), countUnknown(incumbent); cu != iu {
		return cu < iu
	}
	if cd, id := maxDepth(candidate), maxDepth(incumbent); cd != id {
		return cd > id
	}
	return joinMessages(candidate) < joinMessages(incumbent)
}

func countUnknown(leaves []leaf) int {
	n := 0
	for _, l := range leaves {
		if isUnknownProperty(l) {
			n++
		}
	}
	return n
}

func maxDepth(leaves []leaf) int {
	d := 0
	for _, l := range leaves {
		if len(l.loc) > d {
			d = len(l.loc)
		}
	}
	return d
}

func joinMessages(leaves []leaf) string {
	parts := make([]string, len(leaves))
	for i, l := range leaves {
		parts[i] = strings.Join(l.loc, "/") + ":" + l.schema
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// isStructural reports whether a kind merely records that a combinator failed, carrying no error of
// its own. Such kinds are dropped in favor of their more specific descendants.
func isStructural(k jsonschema.ErrorKind) bool {
	switch k.(type) {
	case *kind.AllOf, *kind.AnyOf, *kind.OneOf, *kind.Group, *kind.Not, *kind.Reference, *kind.Schema:
		return true
	default:
		return false
	}
}

// isUnknownProperty reports whether a leaf means "this property is not permitted here": either an
// additionalProperties violation, or a property rejected by a root "unevaluatedProperties": false.
func isUnknownProperty(l leaf) bool {
	switch l.kind.(type) {
	case *kind.AdditionalProperties:
		return true
	case *kind.FalseSchema:
		return strings.HasSuffix(l.schema, "/unevaluatedProperties") ||
			strings.HasSuffix(l.schema, "/additionalProperties")
	default:
		return false
	}
}

// unknownDiagnostics renders the unknown-property leaf as one diagnostic per offending property,
// each suggesting the nearest known property name. When suppressExtensions is set (the main variant),
// root properties contributed by the VS Code and Codespaces schemas are skipped: the base schema
// cannot see them and reports them as unknown even though the main variant accepts them (see
// [propertyNames]).
func unknownDiagnostics(l leaf, props propertyNames, suppressExtensions bool, offsetFor func(loc []string) int) []Diagnostic {
	var diags []Diagnostic
	switch k := l.kind.(type) {
	case *kind.AdditionalProperties:
		for _, name := range k.Properties {
			if suppressExtensions && props.extensionRoot[name] && len(l.loc) == 0 {
				continue
			}
			loc := append(append([]string{}, l.loc...), name)
			diags = append(diags, Diagnostic{Message: unknownMessage(name, props), Offset: offsetFor(loc)})
		}
	case *kind.FalseSchema:
		if len(l.loc) == 0 {
			return nil
		}
		name := l.loc[len(l.loc)-1]
		if suppressExtensions && props.extensionRoot[name] && len(l.loc) == 1 {
			return nil
		}
		diags = append(diags, Diagnostic{Message: unknownMessage(name, props), Offset: offsetFor(l.loc)})
	}
	return diags
}

// unknownMessage builds an "unknown property" message, appending a "did you mean" suggestion when a
// close known property name exists.
func unknownMessage(name string, props propertyNames) string {
	msg := fmt.Sprintf("unknown property %q", name)
	if suggestion := nearest(name, props.candidates); suggestion != "" {
		msg += fmt.Sprintf("; did you mean %q?", suggestion)
	}
	return msg
}

// nearest returns the candidate property name closest to name by edit distance, or "" when none is
// within a small threshold. A candidate equal to name is ignored, so a property that is valid
// elsewhere never suggests itself.
func nearest(name string, candidates map[string]bool) string {
	best := ""
	bestDist := len(name)/3 + 1 // allow roughly one edit per three characters, at least one
	for cand := range candidates {
		if cand == name {
			continue
		}
		if d := levenshtein.Distance(name, cand, nil); d <= bestDist {
			if d < bestDist || best == "" || cand < best {
				best, bestDist = cand, d
			}
		}
	}
	return best
}

// valueMessage phrases a value error (wrong type, bad enum, missing property, …). Kinds decolint
// does not special-case fall back to the library's own wording.
func valueMessage(l leaf) string {
	switch k := l.kind.(type) {
	case *kind.Type:
		return fmt.Sprintf("%s must be %s, but is %s", subject(l.loc), strings.Join(k.Want, " or "), k.Got)
	case *kind.Enum:
		return fmt.Sprintf("%s has an unsupported value", subject(l.loc))
	case *kind.Required:
		if len(k.Missing) == 1 {
			return fmt.Sprintf("missing required property %q", k.Missing[0])
		}
		return fmt.Sprintf("missing required properties %s", strings.Join(quoteAll(k.Missing), ", "))
	default:
		return fmt.Sprintf("%s: %s", subject(l.loc), k.LocalizedString(printer))
	}
}

// subject names the value an error is about: the property or element by its pointer, or "the
// document" at the root.
func subject(loc []string) string {
	if len(loc) == 0 {
		return "the document"
	}
	return "property " + fmt.Sprintf("%q", "/"+strings.Join(loc, "/"))
}

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = fmt.Sprintf("%q", n)
	}
	return out
}

// dedupe removes diagnostics that repeat the same message at the same offset (oneOf branches often
// produce the same error twice) and returns them ordered by position.
func dedupe(diags []Diagnostic) []Diagnostic {
	seen := map[string]bool{}
	var out []Diagnostic
	for _, d := range diags {
		key := fmt.Sprintf("%d\x00%s", d.Offset, d.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Offset != out[j].Offset {
			return out[i].Offset < out[j].Offset
		}
		return out[i].Message < out[j].Message
	})
	return out
}
