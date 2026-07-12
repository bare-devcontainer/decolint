package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestIDDirMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		fileType linter.FileType
		src      string
		want     []linter.Issue
	}{
		{"no id property", "my-feature/devcontainer-feature.json", linter.Feature, `{"name": "test"}`, nil},
		{"feature matching id", "my-feature/devcontainer-feature.json", linter.Feature, `{"id": "my-feature"}`, nil},
		{"feature mismatched id", "my-feature/devcontainer-feature.json", linter.Feature, `{"id": "other"}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 8, RuleID: "id-dir-mismatch", Message: `id "other" does not match containing directory "my-feature"`},
		}},
		{"feature nested under a parent directory", "src/my-feature/devcontainer-feature.json", linter.Feature, `{"id": "my-feature"}`, nil},
		{"feature nested, id matches parent instead of own directory", "src/my-feature/devcontainer-feature.json", linter.Feature, `{"id": "src"}`, []linter.Issue{
			{Path: "src/my-feature/devcontainer-feature.json", Line: 1, Col: 8, RuleID: "id-dir-mismatch", Message: `id "src" does not match containing directory "my-feature"`},
		}},
		{"template matching id", "my-template/devcontainer-template.json", linter.Template, `{"id": "my-template"}`, nil},
		{"template mismatched id", "my-template/devcontainer-template.json", linter.Template, `{"id": "other"}`, []linter.Issue{
			{Path: "my-template/devcontainer-template.json", Line: 1, Col: 8, RuleID: "id-dir-mismatch", Message: `id "other" does not match containing directory "my-template"`},
		}},
		{"non-string id", "my-feature/devcontainer-feature.json", linter.Feature, `{"id": 42}`, nil},
		{"position points at the value, not the key", "my-feature/devcontainer-feature.json", linter.Feature, `{
  // the feature's id
  "id": "other"
}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 3, Col: 9, RuleID: "id-dir-mismatch", Message: `id "other" does not match containing directory "my-feature"`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.IDDirMismatch, linter.SeverityError, tt.path, tt.fileType, tt.src, tt.want)
		})
	}
}
