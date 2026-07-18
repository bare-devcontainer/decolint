package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// lintSource parses src and applies l's registered rules to it as a file at the given path and of
// the given type, failing the test on any parse error. It is the in-memory entry the rule tests use
// in place of a real directory.
func lintSource(t *testing.T, l *linter.Linter, path string, fileType linter.FileType, src string) []linter.Issue {
	t.Helper()
	doc, err := linter.ParseDocument([]byte(src))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	return l.LintDocument(path, fileType, doc)
}

// assertIssues lints src as a devcontainer.json with r as the only active rule, registered at
// severity, exercising the same path-matching and traversal logic the linter uses in production,
// and fails the test if the resulting issues don't match want. Issues are compared irrespective of
// order.
func assertIssues(t *testing.T, r *linter.Rule, severity linter.Severity, src string, want []linter.Issue) {
	t.Helper()
	assertIssuesAt(t, r, severity, "devcontainer.json", linter.Devcontainer, src, want)
}

// assertIssuesAt is like assertIssues but lets the caller control the path and file type src is
// linted as, e.g. to exercise rules that depend on the containing directory or apply only to
// Features or Templates.
func assertIssuesAt(t *testing.T, r *linter.Rule, severity linter.Severity, path string, fileType linter.FileType, src string, want []linter.Issue) {
	t.Helper()

	l := linter.New()
	l.RegisterRule(r, severity)
	got := lintSource(t, l, path, fileType, src)

	// want is written without Severity since it's the same for every case in a table-driven test: the
	// severity r was registered at.
	for i, w := range want {
		w.Severity = severity
		want[i] = w
	}

	sortIssues := cmpopts.SortSlices(func(a, b linter.Issue) bool {
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
	if diff := cmp.Diff(want, got, sortIssues); diff != "" {
		t.Errorf("issues mismatch (-want +got):\n%s", diff)
	}
}
