package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoCapAddAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no capAdd property", `{"name": "test"}`, nil},
		{"capAdd without ALL", `{"capAdd": ["SYS_PTRACE"]}`, nil},
		{"capAdd with ALL", `{"capAdd": ["SYS_PTRACE", "ALL"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 27, RuleID: "no-cap-add-all",
				Message: `"capAdd" contains "ALL", granting every Linux capability to the container`},
		}},
		{"capAdd with lower-case all", `{"capAdd": ["all"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-cap-add-all",
				Message: `"capAdd" contains "ALL", granting every Linux capability to the container`},
		}},
		// "ALL" takes no "CAP_" prefix, so a prefixed one is an ordinary capability name.
		{"capAdd with CAP_ALL", `{"capAdd": ["CAP_ALL"]}`, nil},
		{"no runArgs", `{"runArgs": ["--init"]}`, nil},
		{"runArgs without cap-add=ALL", `{"runArgs": ["--init", "--cap-add=SYS_PTRACE"]}`, nil},
		{"runArgs with cap-add=ALL", `{"runArgs": ["--init", "--cap-add=ALL"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 24, RuleID: "no-cap-add-all",
				Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`},
		}},
		{"runArgs with cap-add ALL two tokens", `{"runArgs": ["--init", "--cap-add", "ALL"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 37, RuleID: "no-cap-add-all",
				Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`},
		}},
		{"runArgs with cap-drop ALL is not cap-add", `{"runArgs": ["--cap-drop", "ALL"]}`, nil},
		{"runArgs with lower-case all", `{"runArgs": ["--cap-add=all"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-cap-add-all",
				Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`},
		}},
		{"runArgs with cap-add consumed as another flag's value", `{"runArgs": ["--label", "--cap-add=ALL"]}`, nil},
		// Every entry granting "ALL" is reported, so suppressing one does not hide the rest.
		{"runArgs with two cap-add=ALL entries", `{"runArgs": ["--cap-add=ALL", "--cap-add=all"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-cap-add-all",
				Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`},
			{Path: "devcontainer.json", Line: 1, Col: 31, RuleID: "no-cap-add-all",
				Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`},
		}},
		{"runArgs with non-string entry before cap-add=ALL", `{"runArgs": [123, "--cap-add=ALL"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 19, RuleID: "no-cap-add-all",
				Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`},
		}},
		{"both capAdd and runArgs flag", `{"capAdd": ["ALL"], "runArgs": ["--cap-add=ALL"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-cap-add-all",
				Message: `"capAdd" contains "ALL", granting every Linux capability to the container`},
			{Path: "devcontainer.json", Line: 1, Col: 33, RuleID: "no-cap-add-all",
				Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoCapAddAll, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}

func TestNoCapAddAll_Feature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no capAdd property", `{"id": "test", "version": "1.0.0", "name": "test"}`, nil},
		{"capAdd without ALL", `{"id": "test", "capAdd": ["SYS_PTRACE"]}`, nil},
		{"capAdd with ALL", `{"id": "test", "capAdd": ["ALL"]}`, []linter.Issue{
			{Path: "devcontainer-feature.json", Line: 1, Col: 27, RuleID: "no-cap-add-all",
				Message: `"capAdd" contains "ALL", granting every Linux capability to the container`},
		}},
		{"capAdd with lower-case all", `{"id": "test", "capAdd": ["all"]}`, []linter.Issue{
			{Path: "devcontainer-feature.json", Line: 1, Col: 27, RuleID: "no-cap-add-all",
				Message: `"capAdd" contains "ALL", granting every Linux capability to the container`},
		}},
		// "runArgs" has no meaning in a Feature, so it's not flagged there.
		{"runArgs with cap-add=ALL is ignored", `{"id": "test", "runArgs": ["--cap-add=ALL"]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.NoCapAddAll, linter.SeverityWarn, "devcontainer-feature.json", linter.Feature, tt.src, tt.want)
		})
	}
}
