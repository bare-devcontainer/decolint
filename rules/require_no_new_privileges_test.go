package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestRequireNoNewPrivileges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no securityOpt or runArgs", `{"name": "test"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},

		// "securityOpt"
		{"securityOpt bare", `{"securityOpt": ["no-new-privileges"]}`, nil},
		{"securityOpt equals true", `{"securityOpt": ["no-new-privileges=true"]}`, nil},
		{"securityOpt colon true", `{"securityOpt": ["no-new-privileges:true"]}`, nil},
		{"securityOpt equals false", `{"securityOpt": ["no-new-privileges=false"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},
		{"securityOpt with unrelated option", `{"securityOpt": ["seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},
		{"securityOpt not an array", `{"securityOpt": "no-new-privileges"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},

		// "runArgs"
		{"runArgs without security-opt", `{"runArgs": ["--init", "--cap-add=SYS_PTRACE"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},
		{"runArgs two tokens", `{"runArgs": ["--security-opt", "no-new-privileges"]}`, nil},
		{"runArgs combined", `{"runArgs": ["--security-opt=no-new-privileges=true"]}`, nil},
		{"runArgs combined false", `{"runArgs": ["--security-opt=no-new-privileges=false"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},
		{"runArgs not an array", `{"runArgs": "--security-opt=no-new-privileges"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},

		// document root
		{"root is an array, not an object", `[]`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.RequireNoNewPrivileges, linter.Warn, tt.src, tt.want)
		})
	}
}
