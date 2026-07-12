package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestMissingRequiredProps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		fileType linter.FileType
		src      string
		want     []linter.Issue
	}{
		{"feature with all required properties", "my-feature/devcontainer-feature.json", linter.Feature, `{"id": "my-feature", "version": "1.0.0", "name": "My linter.Feature"}`, nil},
		{"template with all required properties", "my-template/devcontainer-template.json", linter.Template, `{"id": "my-template", "version": "1.0.0", "name": "My linter.Template"}`, nil},
		{"feature missing id", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": "1.0.0", "name": "My linter.Feature"}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 1, RuleID: "missing-required-props", Message: `required property "id" is missing`},
		}},
		{"feature missing version", "my-feature/devcontainer-feature.json", linter.Feature, `{"id": "my-feature", "name": "My linter.Feature"}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 1, RuleID: "missing-required-props", Message: `required property "version" is missing`},
		}},
		{"feature missing name", "my-feature/devcontainer-feature.json", linter.Feature, `{"id": "my-feature", "version": "1.0.0"}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 1, RuleID: "missing-required-props", Message: `required property "name" is missing`},
		}},
		{"feature missing all required properties", "my-feature/devcontainer-feature.json", linter.Feature, `{}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 1, RuleID: "missing-required-props", Message: `required property "id" is missing`},
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 1, RuleID: "missing-required-props", Message: `required property "version" is missing`},
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 1, RuleID: "missing-required-props", Message: `required property "name" is missing`},
		}},
		{"template missing id and version", "my-template/devcontainer-template.json", linter.Template, `{"name": "My linter.Template"}`, []linter.Issue{
			{Path: "my-template/devcontainer-template.json", Line: 1, Col: 1, RuleID: "missing-required-props", Message: `required property "id" is missing`},
			{Path: "my-template/devcontainer-template.json", Line: 1, Col: 1, RuleID: "missing-required-props", Message: `required property "version" is missing`},
		}},
		{"root is an array, not an object", "my-feature/devcontainer-feature.json", linter.Feature, `[]`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.MissingRequiredProps, linter.SeverityError, tt.path, tt.fileType, tt.src, tt.want)
		})
	}
}
