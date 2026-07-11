package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestCodespacesNoBindMount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no mounts", `{"name": "test"}`, nil},
		{"string volume mount", `{"mounts": ["source=vol,target=/data,type=volume"]}`, nil},
		{"string mount with no type", `{"mounts": ["source=vol,target=/data"]}`, nil},
		{"string bind mount", `{"mounts": ["source=/host,target=/data,type=bind"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "codespaces-no-bind-mount",
				Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`},
		}},
		{"string bind mount with spaces", `{"mounts": ["source=/host, target=/data, type=bind"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "codespaces-no-bind-mount",
				Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`},
		}},
		{"object volume mount", `{"mounts": [{"source": "vol", "target": "/data", "type": "volume"}]}`, nil},
		{"object mount with no type", `{"mounts": [{"source": "vol", "target": "/data"}]}`, nil},
		{"object bind mount", `{"mounts": [{"source": "/host", "target": "/data", "type": "bind"}]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "codespaces-no-bind-mount",
				Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`},
		}},
		{"mixed valid and bind mounts", `{"mounts": ["source=vol,target=/data,type=volume", "source=/host,target=/etc,type=bind"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 52, RuleID: "codespaces-no-bind-mount",
				Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`},
		}},
		{"string docker socket bind mount", `{"mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]}`, nil},
		{"string docker socket bind mount with docker-host.sock target", `{"mounts": ["source=/var/run/docker.sock,target=/var/run/docker-host.sock,type=bind"]}`, nil},
		{"object docker socket bind mount", `{"mounts": [{"source": "/var/run/docker.sock", "target": "/var/run/docker.sock", "type": "bind"}]}`, nil},
		{"bind mount with unrelated source named docker.sock", `{"mounts": ["source=/host/docker.sock,target=/var/run/docker.sock,type=bind"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "codespaces-no-bind-mount",
				Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`},
		}},
		{"non-string literal mount entry", `{"mounts": [123]}`, nil},
		{"non-literal, non-object mount entry", `{"mounts": [["source=/host,target=/data,type=bind"]]}`, nil},
		{"string bind mount with a valueless part", `{"mounts": ["source=/host,target=/data,type=bind,extra"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "codespaces-no-bind-mount",
				Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`},
		}},
		{"object bind mount with non-string source", `{"mounts": [{"type": "bind", "source": 123, "target": "/data"}]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "codespaces-no-bind-mount",
				Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.CodespacesNoBindMount{}, linter.Warn, tt.src, tt.want)
		})
	}
}
