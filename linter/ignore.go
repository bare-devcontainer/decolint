package linter

import (
	"strings"

	"github.com/tailscale/hujson"
)

// Ignore directives are comments recognized inside devcontainer.json:
//
//	// decolint-ignore-line <rule-id>[, <rule-id>...]
//	// decolint-ignore-next-line <rule-id>[, <rule-id>...]
//	// decolint-ignore-file [<rule-id>, ...]
//
// A "decolint-ignore-line" directive suppresses matching findings on the same line. A
// "decolint-ignore-next-line" directive suppresses matching findings on the line immediately below.
// Omitting the rule IDs suppresses all rules. A "decolint-ignore-file" directive suppresses matching
// findings in the whole file.
const (
	lineDirective     = "decolint-ignore-line"
	nextLineDirective = "decolint-ignore-next-line"
	fileDirective     = "decolint-ignore-file"
)

// comment is a single comment in the source, without its "//" or "/* */" markers.
type comment struct {
	offset int // byte offset of the start of the comment marker
	text   string
}

// ignoreIndex holds the ignore directives found in one file, keyed by the target line they apply to
// (already resolved from the directive's own line, e.g. a next-line directive on line N is stored
// under N+1).
type ignoreIndex struct {
	fileAll   bool
	fileRules map[string]bool
	lineAll   map[int]bool            // line -> all rules ignored
	lineRules map[int]map[string]bool // line -> rule IDs ignored
}

// addLine registers rules (or, if empty, all rules) as ignored on line.
func (ix *ignoreIndex) addLine(line int, rules []string) {
	if len(rules) == 0 {
		ix.lineAll[line] = true
	}
	for _, r := range rules {
		if ix.lineRules[line] == nil {
			ix.lineRules[line] = map[string]bool{}
		}
		ix.lineRules[line][r] = true
	}
}

// ignores reports whether a finding for ruleID on the given line is suppressed by a directive.
func (ix *ignoreIndex) ignores(line int, ruleID string) bool {
	if ix.fileAll || ix.fileRules[ruleID] {
		return true
	}
	return ix.lineAll[line] || ix.lineRules[line][ruleID]
}

// buildIgnoreIndex extracts ignore directives from all comments in the syntax tree.
func buildIgnoreIndex(root *hujson.Value, pos *positions) *ignoreIndex {
	ix := &ignoreIndex{
		fileRules: map[string]bool{},
		lineAll:   map[int]bool{},
		lineRules: map[int]map[string]bool{},
	}
	for _, c := range collectComments(root) {
		directive, rest, ok := splitDirective(c.text)
		if !ok {
			continue
		}
		rules := splitRuleIDs(rest)
		switch directive {
		case fileDirective:
			if len(rules) == 0 {
				ix.fileAll = true
			}
			for _, r := range rules {
				ix.fileRules[r] = true
			}
		case lineDirective:
			line, _ := pos.lineCol(c.offset)
			ix.addLine(line, rules)
		case nextLineDirective:
			line, _ := pos.lineCol(c.offset)
			ix.addLine(line+1, rules)
		}
	}
	return ix
}

// splitDirective checks whether a comment is an ignore directive and, if so, returns the directive
// keyword and the remainder of the comment.
func splitDirective(text string) (directive, rest string, ok bool) {
	text = strings.TrimSpace(text)
	for _, d := range [...]string{fileDirective, nextLineDirective, lineDirective} {
		if r, found := strings.CutPrefix(text, d); found {
			// Require the keyword to end at a word boundary so that e.g. "decolint-ignore-lined" is not a
			// directive.
			if r == "" || r[0] == ' ' || r[0] == '\t' {
				return d, r, true
			}
		}
	}
	return "", "", false
}

// splitRuleIDs parses a rule ID list separated by commas and/or spaces.
func splitRuleIDs(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
}

// collectComments walks the syntax tree and returns every comment with its byte offset in the
// original source.
func collectComments(root *hujson.Value) []comment {
	var comments []comment
	add := func(extra hujson.Extra, base int) {
		comments = append(comments, scanExtra(extra, base)...)
	}
	for v := range root.All() {
		if len(v.BeforeExtra) > 0 {
			// BeforeExtra immediately precedes the value.
			add(v.BeforeExtra, v.StartOffset-len(v.BeforeExtra))
		}
		if len(v.AfterExtra) > 0 {
			// AfterExtra immediately follows the value.
			add(v.AfterExtra, v.EndOffset)
		}
		// The extra before the closing brace/bracket of a composite is not attached to any child value.
		switch t := v.Value.(type) {
		case *hujson.Object:
			if len(t.AfterExtra) > 0 {
				add(t.AfterExtra, v.EndOffset-len("}")-len(t.AfterExtra))
			}
		case *hujson.Array:
			if len(t.AfterExtra) > 0 {
				add(t.AfterExtra, v.EndOffset-len("]")-len(t.AfterExtra))
			}
		}
	}
	return comments
}

// scanExtra splits a whitespace-and-comments run into individual comments. base is the byte offset
// of extra in the original source.
func scanExtra(extra hujson.Extra, base int) []comment {
	var comments []comment
	for i := 0; i < len(extra)-1; i++ {
		if extra[i] != '/' {
			continue
		}
		switch extra[i+1] {
		case '/':
			end := len(extra)
			if j := strings.IndexByte(string(extra[i:]), '\n'); j >= 0 {
				end = i + j
			}
			comments = append(comments, comment{offset: base + i, text: string(extra[i+2 : end])})
			i = end
		case '*':
			end := len(extra)
			if j := strings.Index(string(extra[i+2:]), "*/"); j >= 0 {
				end = i + 2 + j
			}
			comments = append(comments, comment{offset: base + i, text: string(extra[i+2 : end])})
			i = end + 1
		}
	}
	return comments
}
