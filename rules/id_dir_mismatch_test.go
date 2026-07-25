package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestIDDirMismatch(t *testing.T) {
	t.Parallel()

	// The reported path and the directory's name are supplied separately, as the linter supplies
	// them: the path is only how the file is named in output (see [linter.Context.Path]), so the
	// cases below vary the two independently.
	const featurePath = "my-feature/devcontainer-feature.json"
	const templatePath = "my-template/devcontainer-template.json"

	tests := []struct {
		name     string
		path     string
		dir      string
		fileType linter.FileType
		src      string
		want     []linter.Issue
	}{
		{"no id property", featurePath, "my-feature", linter.Feature, `{"name": "test"}`, nil},
		{"feature matching id", featurePath, "my-feature", linter.Feature, `{"id": "my-feature"}`, nil},
		{"feature mismatched id", featurePath, "my-feature", linter.Feature, `{"id": "other"}`, []linter.Issue{
			{Path: featurePath, Line: 1, Col: 8, RuleID: "id-dir-mismatch", Message: `id "other" does not match containing directory "my-feature"`},
		}},
		{"feature reported with no directory component", "devcontainer-feature.json", "my-feature", linter.Feature, `{"id": "my-feature"}`, nil},
		{"feature reported with no directory component, mismatched id", "devcontainer-feature.json", "my-feature", linter.Feature, `{"id": "other"}`, []linter.Issue{
			{Path: "devcontainer-feature.json", Line: 1, Col: 8, RuleID: "id-dir-mismatch", Message: `id "other" does not match containing directory "my-feature"`},
		}},
		{"feature nested under a parent directory", "src/my-feature/devcontainer-feature.json", "my-feature", linter.Feature, `{"id": "my-feature"}`, nil},
		{"feature nested, id matches parent instead of own directory", "src/my-feature/devcontainer-feature.json", "my-feature", linter.Feature, `{"id": "src"}`, []linter.Issue{
			{Path: "src/my-feature/devcontainer-feature.json", Line: 1, Col: 8, RuleID: "id-dir-mismatch", Message: `id "src" does not match containing directory "my-feature"`},
		}},
		{"template matching id", templatePath, "my-template", linter.Template, `{"id": "my-template"}`, nil},
		{"template mismatched id", templatePath, "my-template", linter.Template, `{"id": "other"}`, []linter.Issue{
			{Path: templatePath, Line: 1, Col: 8, RuleID: "id-dir-mismatch", Message: `id "other" does not match containing directory "my-template"`},
		}},
		{"unnamed directory", featurePath, "", linter.Feature, `{"id": "other"}`, nil},
		{"non-string id", featurePath, "my-feature", linter.Feature, `{"id": 42}`, nil},
		{"position points at the value, not the key", featurePath, "my-feature", linter.Feature, `{
  // the feature's id
  "id": "other"
}`, []linter.Issue{
			{Path: featurePath, Line: 3, Col: 9, RuleID: "id-dir-mismatch", Message: `id "other" does not match containing directory "my-feature"`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesInDir(t, rules.IDDirMismatch, linter.SeverityError, tt.path, tt.fileType, tt.src, linter.Dir{Name: tt.dir}, tt.want)
		})
	}
}
