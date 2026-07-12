package linter

import (
	"slices"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"
)

// pattern is one compiled entry of a rule's Paths.
type pattern struct {
	rule     *Rule
	segments []string // unescaped segments; "*" matches any segment
}

// compilePatterns compiles the path patterns of a rule.
func compilePatterns(r *Rule) []pattern {
	var out []pattern
	for _, p := range r.Paths {
		out = append(out, pattern{rule: r, segments: splitPointer(p)})
	}
	return out
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

// walk traverses the syntax tree depth-first exactly once and calls visit for every (rule, value)
// pair where one of the rule's patterns matches the value's path. A rule is visited at most once
// per value. pointer and segs describe the location of v; they must be "" and nil for the document
// root.
func walk(v *hujson.Value, pointer string, segs []string, patterns []pattern, visit func(*Rule, *Node)) {
	node := &Node{Pointer: pointer, Value: v}
	var called []*Rule
	for _, p := range patterns {
		if !matches(p.segments, segs) {
			continue
		}
		if slices.Contains(called, p.rule) {
			continue
		}
		called = append(called, p.rule)
		visit(p.rule, node)
	}

	// append(segs, seg) below may share segs's backing array across sibling calls, so a later
	// sibling can overwrite an element a previous sibling appended. This is safe only because
	// traversal is sequential and no walk call retains segs past its own return (matches reads it
	// synchronously via the visit callback). Parallelizing this traversal or having Node retain segs
	// would require copying it first.
	switch t := v.Value.(type) {
	case *hujson.Object:
		for i := range t.Members {
			m := &t.Members[i]
			name, ok := m.Name.Value.(hujson.Literal)
			if !ok {
				continue
			}
			seg := name.String()
			walk(&m.Value, pointer+"/"+escapeSegment(seg), append(segs, seg), patterns, visit)
		}
	case *hujson.Array:
		for i := range t.Elements {
			seg := strconv.Itoa(i)
			walk(&t.Elements[i], pointer+"/"+seg, append(segs, seg), patterns, visit)
		}
	}
}
