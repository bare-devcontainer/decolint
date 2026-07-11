package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoSeccompUnconfined(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no securityOpt or runArgs", `{"name": "test"}`, nil},

		// "securityOpt"
		{"securityOpt with custom seccomp profile", `{"securityOpt": ["seccomp=/path/to/profile.json"]}`, nil},
		{"securityOpt with unrelated option", `{"securityOpt": ["apparmor=unconfined"]}`, nil},
		{"securityOpt seccomp unconfined", `{"securityOpt": ["seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 18, RuleID: "no-seccomp-unconfined",
				Message: `"securityOpt" contains "seccomp=unconfined", disabling the container's syscall filtering`},
		}},

		// "runArgs"
		{"runArgs without security-opt", `{"runArgs": ["--init", "--cap-add=SYS_PTRACE"]}`, nil},
		{"runArgs with custom seccomp profile", `{"runArgs": ["--security-opt", "seccomp=/path/to/profile.json"]}`, nil},
		{"runArgs with unrelated security-opt", `{"runArgs": ["--security-opt", "apparmor=unconfined"]}`, nil},
		{"runArgs seccomp unconfined two tokens", `{"runArgs": ["--security-opt", "seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 32, RuleID: "no-seccomp-unconfined",
				Message: `"runArgs" contains "--security-opt seccomp=unconfined", disabling the container's syscall filtering`},
		}},
		{"runArgs seccomp unconfined combined", `{"runArgs": ["--security-opt=seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-seccomp-unconfined",
				Message: `"runArgs" contains "--security-opt seccomp=unconfined", disabling the container's syscall filtering`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoSeccompUnconfined{}, linter.Warn, tt.src, tt.want)
		})
	}
}

func TestNoSeccompUnconfinedFeature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no securityOpt", `{"id": "test", "version": "1.0.0", "name": "test"}`, nil},
		{"securityOpt with custom seccomp profile", `{"id": "test", "securityOpt": ["seccomp=/path/to/profile.json"]}`, nil},
		{"securityOpt seccomp unconfined", `{"id": "test", "securityOpt": ["seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer-feature.json", Line: 1, Col: 32, RuleID: "no-seccomp-unconfined",
				Message: `"securityOpt" contains "seccomp=unconfined", disabling the container's syscall filtering`},
		}},
		// "runArgs" has no meaning in a Feature, so it's not flagged there.
		{"runArgs with security-opt is ignored", `{"id": "test", "runArgs": ["--security-opt=seccomp=unconfined"]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.NoSeccompUnconfined{}, linter.Warn, "devcontainer-feature.json", linter.Feature, tt.src, tt.want)
		})
	}
}
