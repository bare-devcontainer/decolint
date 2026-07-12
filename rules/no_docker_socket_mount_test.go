package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoDockerSocketMount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no mounts or runArgs", `{"name": "test"}`, nil},

		// "mounts"
		{"string unrelated bind mount", `{"mounts": ["source=/host,target=/data,type=bind"]}`, nil},
		{"string docker socket bind mount", `{"mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-docker-socket-mount",
				Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"string docker socket mount with renamed target", `{"mounts": ["source=/var/run/docker.sock,target=/var/run/docker-host.sock,type=bind"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-docker-socket-mount",
				Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"object docker socket bind mount", `{"mounts": [{"source": "/var/run/docker.sock", "target": "/var/run/docker.sock", "type": "bind"}]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-docker-socket-mount",
				Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"unrelated source named docker.sock", `{"mounts": ["source=/host/docker.sock,target=/var/run/docker.sock,type=bind"]}`, nil},
		{"non-string literal mount entry", `{"mounts": [123]}`, nil},
		{"non-literal, non-object mount entry", `{"mounts": [["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]]}`, nil},

		// "runArgs"
		{"runArgs without docker socket", `{"runArgs": ["--init", "-v", "/data:/data"]}`, nil},
		{"runArgs -v two tokens", `{"runArgs": ["-v", "/var/run/docker.sock:/var/run/docker.sock"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 20, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs -v two tokens with mode", `{"runArgs": ["-v", "/var/run/docker.sock:/var/run/docker.sock:ro"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 20, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs --volume combined", `{"runArgs": ["--volume=/var/run/docker.sock:/var/run/docker.sock"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs -v combined", `{"runArgs": ["-v=/var/run/docker.sock:/var/run/docker.sock"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs --mount two tokens", `{"runArgs": ["--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 25, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs --mount combined", `{"runArgs": ["--mount=type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs unrelated volume", `{"runArgs": ["-v", "/host/docker.sock:/var/run/docker.sock"]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoDockerSocketMount, linter.Warn, tt.src, tt.want)
		})
	}
}
