package linter

import (
	"slices"
	"strconv"
	"strings"

	"github.com/bare-devcontainer/decolint/dockerargs"
	"github.com/tailscale/hujson"
)

// pattern is one compiled entry of a rule's Paths.
type pattern struct {
	rule     *Rule
	segments []string // unescaped segments; "*" matches any segment
	flag     bool     // addresses a "docker run" flag occurrence (see isFlagPattern)
}

// compilePatterns compiles the path patterns of a rule.
func compilePatterns(r *Rule) []pattern {
	var out []pattern
	for _, p := range r.Paths {
		segs := splitPointer(p)
		out = append(out, pattern{rule: r, segments: segs, flag: isFlagPattern(segs)})
	}
	return out
}

// isFlagPattern reports whether segs addresses a "docker run" flag occurrence in "runArgs", e.g.
// "/runArgs/--volume". A "*" segment is not one: it matches whatever the traversal reaches there.
func isFlagPattern(segs []string) bool {
	return len(segs) == 2 && segs[0] == "runArgs" && strings.HasPrefix(segs[1], "--")
}

// splitPointer parses a JSON Pointer (RFC 6901) into unescaped segments. The empty pointer denotes
// the document root and yields nil.
func splitPointer(ptr string) []string {
	if ptr == "" {
		return nil
	}
	segs := strings.Split(strings.TrimPrefix(ptr, "/"), "/")
	for i, s := range segs {
		s = strings.ReplaceAll(s, "~1", "/")
		segs[i] = strings.ReplaceAll(s, "~0", "~")
	}
	return segs
}

// escapeSegment escapes a segment for use in a JSON Pointer.
func escapeSegment(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}

// matches reports whether a pattern matches the given path segments.
func matches(pat, segs []string) bool {
	if len(pat) != len(segs) {
		return false
	}
	for i := range pat {
		if pat[i] != "*" && pat[i] != segs[i] {
			return false
		}
	}
	return true
}

// walk traverses the syntax tree of a file of the given type depth-first exactly once and calls
// visit for every (rule, value) pair where one of the rule's patterns matches the value's path. A
// rule is visited once for a value however many of its patterns match, except for a "runArgs"
// element naming several flags, which is visited once per flag (see runArgsFlags).
func walk(root *hujson.Value, fileType FileType, patterns []pattern, visit func(*Rule, *Node)) {
	w := walker{patterns: patterns, runArgs: fileType == Devcontainer, visit: visit}
	w.value(root, "", nil)
}

// walker carries the state of one traversal.
type walker struct {
	patterns []pattern
	// runArgs reports whether the document's "runArgs" is traversed as an argv (see runArgsFlags).
	// Only a devcontainer.json has one: it is not a property of a Feature or a Template.
	runArgs bool
	visit   func(*Rule, *Node)
}

// value visits v and descends into it. pointer and segs describe the location of v; they must be ""
// and nil for the document root.
func (w *walker) value(v *hujson.Value, pointer string, segs []string) {
	w.dispatch(&Node{Pointer: pointer, Value: v}, segs)

	// append(segs, seg) here and in runArgsFlags may share segs's backing array across sibling calls,
	// so a later sibling can overwrite an element a previous sibling appended. This is safe only
	// because traversal is sequential and no walk call retains segs past its own return (matches
	// reads it synchronously via the visit callback). Parallelizing this traversal or having Node
	// retain segs would require copying it first.
	switch t := v.Value.(type) {
	case *hujson.Object:
		for i := range t.Members {
			m := &t.Members[i]
			name, ok := m.Name.Value.(hujson.Literal)
			if !ok {
				continue
			}
			seg := name.String()
			w.value(&m.Value, pointer+"/"+escapeSegment(seg), append(segs, seg))
		}
	case *hujson.Array:
		if w.runArgs && len(segs) == 1 && segs[0] == "runArgs" {
			w.runArgsFlags(t, pointer, segs)
			return
		}
		for i := range t.Elements {
			seg := strconv.Itoa(i)
			w.value(&t.Elements[i], pointer+"/"+seg, append(segs, seg))
		}
	}
}

// RunArgs returns every "docker run" flag occurrence in arr, a devcontainer.json's "runArgs", as
// [dockerargs.Parse] reads the argv the array becomes; [dockerargs.Arg.Index] indexes arr.Elements.
// An element that is not a string, which the devcontainer tooling could not hand to docker at all,
// stands in as an empty entry so that the elements around it keep the positions docker would read
// them at.
//
// It is the reading the traversal hands "/runArgs/--flag" patterns (see [Rule.Paths]), for the rules
// that cannot be driven by it because they report a flag's absence.
func RunArgs(arr *hujson.Array) []dockerargs.Arg {
	argv := make([]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		if lit, ok := elem.Value.(hujson.Literal); ok && lit.Kind() == '"' {
			argv[i] = lit.String()
		}
	}
	return dockerargs.Parse(argv)
}

// runArgsFlags visits the elements of arr, a devcontainer.json's "runArgs", as the "docker run" argv
// the array becomes: each flag occurrence is reached at the flag's long spelling, so "-v" and
// "--volume" alike are reached at "/runArgs/--volume", on the element the flag's value is written
// in.
//
// One element can hold several occurrences — a run of shorthands, "-it", names two flags — and is
// visited once for each, so that a rule subscribing to more than one of them, or to "/runArgs/*",
// sees every flag the argv gives rather than whichever comes first. Collapsing them would drop a
// flag silently, which is the failure this addressing exists to prevent. The elements are
// deliberately not visited by index as well: that reaches the same occurrence by a second path and
// buys nothing, where two occurrences in one element are two distinct things to report on.
func (w *walker) runArgsFlags(arr *hujson.Array, pointer string, segs []string) {
	for _, arg := range RunArgs(arr) {
		node := &Node{
			Pointer: pointer + "/" + strconv.Itoa(arg.Index),
			Value:   &arr.Elements[arg.Index],
			Arg:     &arg,
		}
		w.dispatch(node, append(segs, "--"+arg.Flag))
	}
}

// dispatch calls visit for every rule with a pattern matching segs, at most once per rule. A pattern
// addressing a flag occurrence (see isFlagPattern) matches only a node runArgsFlags produced: a
// "runArgs" that is not an argv is still walked as the object or array it is, and a member of it can
// be named like a flag without being one.
func (w *walker) dispatch(node *Node, segs []string) {
	var called []*Rule
	for _, p := range w.patterns {
		if p.flag && node.Arg == nil {
			continue
		}
		if !matches(p.segments, segs) {
			continue
		}
		if slices.Contains(called, p.rule) {
			continue
		}
		called = append(called, p.rule)
		w.visit(p.rule, node)
	}
}
