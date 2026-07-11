package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestCodespacesNoHostPortFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no forwardPorts or portsAttributes", `{"name": "test"}`, nil},
		{"integer port", `{"forwardPorts": [3000]}`, nil},
		{"bare port string", `{"forwardPorts": ["3000"]}`, nil},
		{"host:port entry", `{"forwardPorts": ["db:5432"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 19, RuleID: "codespaces-no-host-port-format",
				Message: `"forwardPorts" entry "db:5432" uses "host:port" format; Codespaces only supports a bare port number`},
		}},
		{"mixed valid and host:port entries", `{"forwardPorts": [3000, "db:5432"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 25, RuleID: "codespaces-no-host-port-format",
				Message: `"forwardPorts" entry "db:5432" uses "host:port" format; Codespaces only supports a bare port number`},
		}},
		{"bare port key", `{"portsAttributes": {"3000": {}}}`, nil},
		{"regex-like key with non-numeric suffix", `{"portsAttributes": {"auto:.*": {}}}`, nil},
		{"leading colon has no host", `{"forwardPorts": [":8080"]}`, nil},
		{"host:port key", `{"portsAttributes": {"db:5432": {}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 33, RuleID: "codespaces-no-host-port-format",
				Message: `"portsAttributes" key "db:5432" uses "host:port" format; Codespaces only supports a bare port number`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.CodespacesNoHostPortFormat{}, linter.Error, tt.src, tt.want)
		})
	}
}
