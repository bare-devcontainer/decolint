package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestMissingContainerDef(t *testing.T) {
	t.Parallel()

	const msg = `devcontainer.json must define one of "image", "build", or "dockerComposeFile"`

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"image defined", `{"image": "ubuntu:22.04"}`, nil},
		{"build defined", `{"build": {"dockerfile": "Dockerfile"}}`, nil},
		{"dockerComposeFile defined", `{"dockerComposeFile": "docker-compose.yml", "service": "app", "workspaceFolder": "/workspace"}`, nil},
		{"none defined", `{"name": "test"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "missing-container-def", Message: msg},
		}},
		{"empty object", `{}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "missing-container-def", Message: msg},
		}},
		{"root is an array, not an object", `[]`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.MissingContainerDef, linter.SeverityError, tt.src, tt.want)
		})
	}
}
