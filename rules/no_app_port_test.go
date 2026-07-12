package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoAppPort(t *testing.T) {
	t.Parallel()

	const msg = `"appPort" is a legacy property; use "forwardPorts" instead to forward ports dynamically`

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no appPort property", `{"name": "test"}`, nil},
		{"integer port", `{"appPort": 8080}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-app-port", Message: msg},
		}},
		{"string port mapping", `{"appPort": "8080:8080"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-app-port", Message: msg},
		}},
		{"array of ports", `{"appPort": [8080, "8081:8081"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-app-port", Message: msg},
		}},
		{"position points at the value, not the key", `{
  // published ports
  "appPort": 8080
}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 3, Col: 14, RuleID: "no-app-port", Message: msg},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoAppPort, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}
