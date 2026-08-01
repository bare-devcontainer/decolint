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
		{"runArgs security-opt consumed as another flag's value", `{"runArgs": ["--label", "--security-opt=no-new-privileges"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},
		{"runArgs combined false", `{"runArgs": ["--security-opt=no-new-privileges=false"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},
		{"runArgs not an array", `{"runArgs": "--security-opt=no-new-privileges"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},

		// duplicate members: a JSON parser keeps only the last copy, so every copy is read
		{"securityOpt duplicated, set by the last", `{"securityOpt": [], "securityOpt": ["no-new-privileges"]}`, nil},
		{"securityOpt duplicated, set by the first", `{"securityOpt": ["no-new-privileges"], "securityOpt": []}`, nil},
		{"securityOpt duplicated, one copy not an array", `{"securityOpt": "no-new-privileges", "securityOpt": ["no-new-privileges"]}`, nil},
		{"securityOpt duplicated, set by neither", `{"securityOpt": [], "securityOpt": ["seccomp=unconfined"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},
		{"runArgs duplicated, set by the last", `{"runArgs": ["--init"], "runArgs": ["--security-opt=no-new-privileges"]}`, nil},
		{"runArgs duplicated, set by the first", `{"runArgs": ["--security-opt=no-new-privileges"], "runArgs": ["--init"]}`, nil},
		{"runArgs duplicated, one copy not an array", `{"runArgs": "--security-opt=no-new-privileges", "runArgs": ["--security-opt", "no-new-privileges"]}`, nil},
		{"runArgs duplicated, set by neither", `{"runArgs": ["--init"], "runArgs": ["--cap-drop=ALL"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "require-no-new-privileges",
				Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`},
		}},

		// document root
		{"root is an array, not an object", `[]`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.RequireNoNewPrivileges, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}
