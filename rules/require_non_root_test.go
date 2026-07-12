package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestRequireNonRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"neither remoteUser nor containerUser", `{"name": "test"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-non-root",
				Message: `neither "remoteUser" nor "containerUser" is set, so the container defaults to running as root`},
		}},
		{"root is an array, not an object", `[]`, nil},
		{"remoteUser non-string falls back to containerUser", `{"remoteUser": 0, "containerUser": "vscode"}`, nil},
		{"remoteUser and containerUser both non-string", `{"remoteUser": 0, "containerUser": 0}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-non-root",
				Message: `neither "remoteUser" nor "containerUser" is set, so the container defaults to running as root`},
		}},

		// "remoteUser"
		{"remoteUser non-root", `{"remoteUser": "vscode"}`, nil},
		{"remoteUser root", `{"remoteUser": "root"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-non-root",
				Message: `"remoteUser" is set to "root", running lifecycle scripts and any remote editor/IDE session as root`},
		}},
		{"remoteUser uid 0", `{"remoteUser": "0"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-non-root",
				Message: `"remoteUser" is set to "root", running lifecycle scripts and any remote editor/IDE session as root`},
		}},
		{"remoteUser root with group", `{"remoteUser": "root:root"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-non-root",
				Message: `"remoteUser" is set to "root", running lifecycle scripts and any remote editor/IDE session as root`},
		}},
		{"remoteUser root overrides non-root containerUser", `{"remoteUser": "root", "containerUser": "vscode"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-non-root",
				Message: `"remoteUser" is set to "root", running lifecycle scripts and any remote editor/IDE session as root`},
		}},
		{"remoteUser non-root overrides root containerUser", `{"remoteUser": "vscode", "containerUser": "root"}`, nil},

		// "containerUser" fallback, since "remoteUser" defaults to it.
		{"containerUser non-root", `{"containerUser": "vscode"}`, nil},
		{"containerUser root", `{"containerUser": "root"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-non-root",
				Message: `"remoteUser" is not set and "containerUser" is set to "root", running lifecycle scripts and any remote editor/IDE session as root`},
		}},
		{"containerUser uid 0", `{"containerUser": "0"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-non-root",
				Message: `"remoteUser" is not set and "containerUser" is set to "root", running lifecycle scripts and any remote editor/IDE session as root`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.RequireNonRoot, linter.Warn, tt.src, tt.want)
		})
	}
}
