package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoPrivilegedContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no privileged property", `{"name": "test"}`, nil},
		{"privileged false", `{"privileged": false}`, nil},
		{"privileged non-boolean", `{"privileged": {}}`, nil},
		{"privileged true", `{"privileged": true}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 16, RuleID: "no-privileged-container",
				Message: `"privileged" is set to true, disabling the container's isolation from the host`},
		}},
		{"no runArgs", `{"runArgs": ["--init"]}`, nil},
		{"runArgs without privileged", `{"runArgs": ["--init", "--cap-add=SYS_PTRACE"]}`, nil},
		{"runArgs with non-literal entry", `{"runArgs": [["--privileged"]]}`, nil},
		{"runArgs with privileged", `{"runArgs": ["--init", "--privileged"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 24, RuleID: "no-privileged-container",
				Message: `"runArgs" contains "--privileged", disabling the container's isolation from the host`},
		}},
		{"runArgs with privileged set to true", `{"runArgs": ["--privileged=true"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-privileged-container",
				Message: `"runArgs" contains "--privileged", disabling the container's isolation from the host`},
		}},
		{"runArgs with privileged set to false", `{"runArgs": ["--privileged=false"]}`, nil},
		{"runArgs with privileged consumed as another flag's value", `{"runArgs": ["--label", "--privileged"]}`, nil},
		// An object "runArgs" is no command line, so a member named like a flag is not that flag.
		{"runArgs object with a privileged member", `{"runArgs": {"--privileged": true}}`, nil},
		{"both privileged and runArgs flag", `{"privileged": true, "runArgs": ["--privileged"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 16, RuleID: "no-privileged-container",
				Message: `"privileged" is set to true, disabling the container's isolation from the host`},
			{Path: "devcontainer.json", Line: 1, Col: 34, RuleID: "no-privileged-container",
				Message: `"runArgs" contains "--privileged", disabling the container's isolation from the host`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoPrivilegedContainer, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}

func TestNoPrivilegedContainer_Feature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no privileged property", `{"id": "test", "version": "1.0.0", "name": "test"}`, nil},
		{"privileged false", `{"id": "test", "privileged": false}`, nil},
		{"privileged true", `{"id": "test", "privileged": true}`, []linter.Issue{
			{Path: "devcontainer-feature.json", Line: 1, Col: 30, RuleID: "no-privileged-container",
				Message: `"privileged" is set to true, disabling the container's isolation from the host`},
		}},
		// "runArgs" has no meaning in a Feature, so it's not flagged there, whatever it holds.
		{"runArgs with privileged is ignored", `{"id": "test", "runArgs": ["--privileged"]}`, nil},
		{"runArgs object with a privileged member is ignored", `{"id": "test", "runArgs": {"--privileged": true}}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.NoPrivilegedContainer, linter.SeverityWarn, "devcontainer-feature.json", linter.Feature, tt.src, tt.want)
		})
	}
}
