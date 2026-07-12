package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestInvalidSemver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		fileType linter.FileType
		src      string
		want     []linter.Issue
	}{
		{"no version property", "my-feature/devcontainer-feature.json", linter.Feature, `{"name": "test"}`, nil},
		{"valid version", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": "1.0.0"}`, nil},
		{"valid version with pre-release", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": "1.0.0-alpha.1"}`, nil},
		{"valid version with build metadata", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": "1.0.0+build.5"}`, nil},
		{"missing patch version", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": "1.0"}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 13, RuleID: "invalid-semver", Message: `version "1.0" is not a valid semantic version (see https://semver.org/)`},
		}},
		{"leading v prefix", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": "v1.0.0"}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 13, RuleID: "invalid-semver", Message: `version "v1.0.0" is not a valid semantic version (see https://semver.org/)`},
		}},
		{"leading zero in numeric identifier", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": "01.0.0"}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 13, RuleID: "invalid-semver", Message: `version "01.0.0" is not a valid semantic version (see https://semver.org/)`},
		}},
		{"non-semver string", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": "latest"}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 1, Col: 13, RuleID: "invalid-semver", Message: `version "latest" is not a valid semantic version (see https://semver.org/)`},
		}},
		{"non-string version", "my-feature/devcontainer-feature.json", linter.Feature, `{"version": 1}`, nil},
		{"valid template version", "my-template/devcontainer-template.json", linter.Template, `{"version": "2.1.3"}`, nil},
		{"invalid template version", "my-template/devcontainer-template.json", linter.Template, `{"version": "2.1"}`, []linter.Issue{
			{Path: "my-template/devcontainer-template.json", Line: 1, Col: 13, RuleID: "invalid-semver", Message: `version "2.1" is not a valid semantic version (see https://semver.org/)`},
		}},
		{"position points at the value, not the key", "my-feature/devcontainer-feature.json", linter.Feature, `{
  // the feature's version
  "version": "1.0"
}`, []linter.Issue{
			{Path: "my-feature/devcontainer-feature.json", Line: 3, Col: 14, RuleID: "invalid-semver", Message: `version "1.0" is not a valid semantic version (see https://semver.org/)`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.InvalidSemver, linter.SeverityError, tt.path, tt.fileType, tt.src, tt.want)
		})
	}
}
