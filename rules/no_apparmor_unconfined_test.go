package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoApparmorUnconfined(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no securityOpt property", `{"name": "test"}`, nil},
		{"securityOpt without apparmor", `{"securityOpt": ["no-new-privileges"]}`, nil},
		{"securityOpt with a custom apparmor profile", `{"securityOpt": ["apparmor=my-profile"]}`, nil},
		{"securityOpt with apparmor=unconfined", `{"securityOpt": ["apparmor=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 18, RuleID: "no-apparmor-unconfined",
				Message: `"securityOpt" contains "apparmor=unconfined", disabling the container's AppArmor confinement`},
		}},
		{"securityOpt with apparmor:unconfined", `{"securityOpt": ["apparmor:unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 18, RuleID: "no-apparmor-unconfined",
				Message: `"securityOpt" contains "apparmor:unconfined", disabling the container's AppArmor confinement`},
		}},
		{"seccomp=unconfined is a different confinement", `{"securityOpt": ["seccomp=unconfined"]}`, nil},
		{"no runArgs", `{"runArgs": ["--init"]}`, nil},
		{"runArgs without apparmor", `{"runArgs": ["--security-opt", "seccomp=unconfined"]}`, nil},
		{"runArgs with security-opt apparmor=unconfined", `{"runArgs": ["--security-opt=apparmor=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-apparmor-unconfined",
				Message: `"runArgs" contains "--security-opt apparmor=unconfined", disabling the container's AppArmor confinement`},
		}},
		{"runArgs with security-opt apparmor=unconfined two tokens", `{"runArgs": ["--security-opt", "apparmor=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 32, RuleID: "no-apparmor-unconfined",
				Message: `"runArgs" contains "--security-opt apparmor=unconfined", disabling the container's AppArmor confinement`},
		}},
		{"every offending entry is reported", `{"runArgs": ["--security-opt=apparmor=unconfined", "--security-opt=apparmor:unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-apparmor-unconfined",
				Message: `"runArgs" contains "--security-opt apparmor=unconfined", disabling the container's AppArmor confinement`},
			{Path: "devcontainer.json", Line: 1, Col: 52, RuleID: "no-apparmor-unconfined",
				Message: `"runArgs" contains "--security-opt apparmor:unconfined", disabling the container's AppArmor confinement`},
		}},
		{"both securityOpt and runArgs", `{"securityOpt": ["apparmor=unconfined"], "runArgs": ["--security-opt=apparmor=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 18, RuleID: "no-apparmor-unconfined",
				Message: `"securityOpt" contains "apparmor=unconfined", disabling the container's AppArmor confinement`},
			{Path: "devcontainer.json", Line: 1, Col: 54, RuleID: "no-apparmor-unconfined",
				Message: `"runArgs" contains "--security-opt apparmor=unconfined", disabling the container's AppArmor confinement`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoApparmorUnconfined, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}

func TestNoApparmorUnconfined_Feature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no securityOpt property", `{"id": "test", "version": "1.0.0", "name": "test"}`, nil},
		{"securityOpt with apparmor=unconfined", `{"id": "test", "securityOpt": ["apparmor=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer-feature.json", Line: 1, Col: 32, RuleID: "no-apparmor-unconfined",
				Message: `"securityOpt" contains "apparmor=unconfined", disabling the container's AppArmor confinement`},
		}},
		// "runArgs" has no meaning in a Feature, so it's not flagged there.
		{"runArgs with apparmor=unconfined is ignored", `{"id": "test", "runArgs": ["--security-opt=apparmor=unconfined"]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.NoApparmorUnconfined, linter.SeverityWarn, "devcontainer-feature.json", linter.Feature, tt.src, tt.want)
		})
	}
}
