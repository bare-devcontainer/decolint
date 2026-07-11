package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestMissingBuildDockerfile(t *testing.T) {
	t.Parallel()

	const msg = `"build" is missing "dockerfile"`

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"dockerfile defined", `{"build": {"dockerfile": "Dockerfile"}}`, nil},
		{"dockerfile missing", `{"build": {"context": "."}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "missing-build-dockerfile", Message: msg},
		}},
		{"build empty", `{"build": {}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "missing-build-dockerfile", Message: msg},
		}},
		{"no build", `{"image": "ubuntu:22.04"}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.MissingBuildDockerfile{}, linter.Error, tt.src, tt.want)
		})
	}
}
