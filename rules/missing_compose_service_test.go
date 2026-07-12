package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestMissingComposeService(t *testing.T) {
	t.Parallel()

	const msg = `devcontainer.json sets "dockerComposeFile" but is missing "service"`

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"dockerComposeFile and service defined", `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`, nil},
		{"dockerComposeFile without service", `{"dockerComposeFile": "docker-compose.yml"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "missing-compose-service", Message: msg},
		}},
		{"service without dockerComposeFile", `{"service": "app"}`, nil},
		{"neither defined", `{"image": "ubuntu:22.04"}`, nil},
		{"empty object", `{}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.MissingComposeService, linter.SeverityError, tt.src, tt.want)
		})
	}
}
