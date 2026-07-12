package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestRequireCapDropAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no capDrop or runArgs", `{"name": "test"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-cap-drop-all",
				Message: `"ALL" is not set via "capDrop" or "runArgs", leaving the container with its default Linux capabilities`},
		}},

		// "capDrop"
		{"capDrop with ALL", `{"capDrop": ["ALL"]}`, nil},
		{"capDrop without ALL", `{"capDrop": ["SYS_PTRACE"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-cap-drop-all",
				Message: `"ALL" is not set via "capDrop" or "runArgs", leaving the container with its default Linux capabilities`},
		}},
		{"capDrop not an array", `{"capDrop": "ALL"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-cap-drop-all",
				Message: `"ALL" is not set via "capDrop" or "runArgs", leaving the container with its default Linux capabilities`},
		}},

		// "runArgs"
		{"runArgs with cap-drop=ALL", `{"runArgs": ["--cap-drop=ALL"]}`, nil},
		{"runArgs with cap-drop ALL two tokens", `{"runArgs": ["--cap-drop", "ALL"]}`, nil},
		{"runArgs without cap-drop=ALL", `{"runArgs": ["--init", "--cap-add=SYS_PTRACE"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-cap-drop-all",
				Message: `"ALL" is not set via "capDrop" or "runArgs", leaving the container with its default Linux capabilities`},
		}},
		{"runArgs with cap-add ALL is not cap-drop", `{"runArgs": ["--cap-add", "ALL"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-cap-drop-all",
				Message: `"ALL" is not set via "capDrop" or "runArgs", leaving the container with its default Linux capabilities`},
		}},
		{"runArgs not an array", `{"runArgs": "--cap-drop=ALL"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-cap-drop-all",
				Message: `"ALL" is not set via "capDrop" or "runArgs", leaving the container with its default Linux capabilities`},
		}},

		// document root
		{"root is an array, not an object", `[]`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.RequireCapDropAll, linter.Warn, tt.src, tt.want)
		})
	}
}
