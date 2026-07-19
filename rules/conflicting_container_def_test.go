package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestConflictingContainerDef(t *testing.T) {
	t.Parallel()

	const msg = `devcontainer.json must define only one of "image", "build", or "dockerComposeFile"`

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"image only", `{"image": "ubuntu:22.04"}`, nil},
		{"build only", `{"build": {"dockerfile": "Dockerfile"}}`, nil},
		{"dockerComposeFile only", `{"dockerComposeFile": "docker-compose.yml"}`, nil},
		{"none defined", `{"name": "test"}`, nil},
		{"empty object", `{}`, nil},
		{"image and build", `{"image": "x", "build": {"dockerfile": "d"}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 16, RuleID: "conflicting-container-def", Message: msg},
		}},
		{"image and dockerComposeFile", `{"image": "x", "dockerComposeFile": "c"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 16, RuleID: "conflicting-container-def", Message: msg},
		}},
		{"build and dockerComposeFile", `{"build": {"dockerfile": "d"}, "dockerComposeFile": "c"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 32, RuleID: "conflicting-container-def", Message: msg},
		}},
		{"all three defined", `{"image": "x", "build": {"dockerfile": "d"}, "dockerComposeFile": "c"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 16, RuleID: "conflicting-container-def", Message: msg},
			{Path: "devcontainer.json", Line: 1, Col: 46, RuleID: "conflicting-container-def", Message: msg},
		}},
		{"root is an array, not an object", `[]`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.ConflictingContainerDef, linter.SeverityError, tt.src, tt.want)
		})
	}
}
