package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoSeccompOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no securityOpt or runArgs", `{"name": "test"}`, nil},

		// "securityOpt"
		{"securityOpt non-string entry", `{"securityOpt": [123]}`, nil},
		{"securityOpt with unrelated option", `{"securityOpt": ["apparmor=unconfined"]}`, nil},
		{"securityOpt seccomp unconfined", `{"securityOpt": ["seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 18, RuleID: "no-seccomp-override",
				Message: `"securityOpt" overrides the default seccomp profile`},
		}},
		{"securityOpt with custom seccomp profile", `{"securityOpt": ["seccomp=/path/to/profile.json"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 18, RuleID: "no-seccomp-override",
				Message: `"securityOpt" overrides the default seccomp profile`},
		}},

		// "runArgs"
		{"runArgs without security-opt", `{"runArgs": ["--init", "--cap-add=SYS_PTRACE"]}`, nil},
		{"runArgs with unrelated security-opt", `{"runArgs": ["--security-opt", "apparmor=unconfined"]}`, nil},
		{"runArgs seccomp unconfined two tokens", `{"runArgs": ["--security-opt", "seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 32, RuleID: "no-seccomp-override",
				Message: `"runArgs" overrides the default seccomp profile via "--security-opt"`},
		}},
		{"runArgs seccomp unconfined combined", `{"runArgs": ["--security-opt=seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-seccomp-override",
				Message: `"runArgs" overrides the default seccomp profile via "--security-opt"`},
		}},
		{"runArgs with custom seccomp profile", `{"runArgs": ["--security-opt", "seccomp=/path/to/profile.json"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 32, RuleID: "no-seccomp-override",
				Message: `"runArgs" overrides the default seccomp profile via "--security-opt"`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoSeccompOverride, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}

func TestNoSeccompOverride_Feature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no securityOpt", `{"id": "test", "version": "1.0.0", "name": "test"}`, nil},
		{"securityOpt with custom seccomp profile", `{"id": "test", "securityOpt": ["seccomp=/path/to/profile.json"]}`, []linter.Issue{
			{Path: "devcontainer-feature.json", Line: 1, Col: 32, RuleID: "no-seccomp-override",
				Message: `"securityOpt" overrides the default seccomp profile`},
		}},
		// "runArgs" has no meaning in a Feature, so it's not flagged there.
		{"runArgs with security-opt is ignored", `{"id": "test", "runArgs": ["--security-opt=seccomp=unconfined"]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.NoSeccompOverride, linter.SeverityWarn, "devcontainer-feature.json", linter.Feature, tt.src, tt.want)
		})
	}
}
