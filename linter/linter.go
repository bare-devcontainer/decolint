// Package linter implements the decolint engine: it parses configuration files as HuJSON (JSONC),
// runs lint rules against the syntax tree, and filters findings suppressed by ignore comments.
// Locating the configuration files a devcontainer directory contains is the discovery package's
// responsibility.
package linter

import (
	"fmt"
	"sort"
)

// Issue is a rule finding resolved to a file position.
type Issue struct {
	Path     string   `json:"path"`
	Line     int      `json:"line"` // 1-based
	Col      int      `json:"col"`  // 1-based, in bytes
	RuleID   string   `json:"ruleId"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
}

func (i Issue) String() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s (%s)", i.Path, i.Line, i.Col, i.Severity, i.Message, i.RuleID)
}

// Linter runs a set of rules against devcontainer configuration files.
type Linter struct {
	patterns map[FileType][]pattern
	// severities holds the effective severity of each rule, keyed by rule ID, as specified when the
	// rule was registered via RegisterRule.
	severities map[string]Severity
}

// New returns an empty Linter. Use RegisterRule to add rules to it.
func New() *Linter {
	return &Linter{patterns: map[FileType][]pattern{}, severities: map[string]Severity{}}
}

// RegisterRule adds r to the linter, to run at the given severity.
func (l *Linter) RegisterRule(r *Rule, severity Severity) {
	l.severities[r.ID] = severity
	if severity == SeverityOff {
		return
	}
	for _, t := range r.FileTypes {
		l.patterns[t] = append(l.patterns[t], compilePatterns(r)...)
	}
}

// HasRules reports whether any registered rule applies to files of type t. Callers can use it to
// skip per-file preparation work (e.g. Feature fetches) for files no rule will inspect.
func (l *Linter) HasRules(t FileType) bool {
	return len(l.patterns[t]) > 0
}

// LintDocument applies the linter's rules to doc, a configuration file of the given type, and
// returns the findings sorted by position, then by rule ID and message. path is used only for
// reporting, and dir describes the directory containing the file; both are handed to the rules as
// [Context.Path] and [Context.Dir], and dir may be left zero for an in-memory document. It reads the
// document as given; any mutation of its tree (see Document.Tree) must happen before calling it.
//
// Issues that match in every field are reported once, so a rule that reaches the same problem by
// more than one route does not report it twice.
func (l *Linter) LintDocument(path string, fileType FileType, doc *Document, dir Dir) []Issue {
	patterns := l.patterns[fileType]
	if len(patterns) == 0 {
		return nil
	}
	rctx := &Context{Path: path, Type: fileType, Root: doc.tree, Dir: dir}
	var issues []Issue
	seen := map[Issue]struct{}{}
	walk(doc.tree, "", nil, patterns, func(r *Rule, node *Node) {
		id := r.ID
		severity := l.severities[id]
		for _, f := range safeCheck(r, rctx, node) {
			line, col := doc.pos.lineCol(f.Offset)
			if doc.ignores.ignores(line, id) {
				continue
			}
			issue := Issue{
				Path:     path,
				Line:     line,
				Col:      col,
				RuleID:   id,
				Message:  f.Message,
				Severity: severity,
			}
			if _, dup := seen[issue]; dup {
				continue
			}
			seen[issue] = struct{}{}
			issues = append(issues, issue)
		}
	})
	// Deduplication leaves position, rule ID and message a unique key, so this order is total: the
	// same document always yields the same sequence.
	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.Message < b.Message
	})
	return issues
}

// safeCheck calls r.Check and recovers from any panic, so that a defect in one rule (e.g. a nil
// dereference on an unexpected value shape) is reported as that rule's finding instead of aborting
// the rest of the lint run.
func safeCheck(r *Rule, rctx *Context, node *Node) (findings []Finding) {
	defer func() {
		if rec := recover(); rec != nil {
			findings = []Finding{{
				Message: fmt.Sprintf("rule panicked: %v", rec),
				Offset:  node.Value.StartOffset,
			}}
		}
	}()
	return r.Check(rctx, node)
}
