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
		{"string docker socket mount via src alias", `{"mounts": ["type=bind,src=/var/run/docker.sock,dst=/x"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-docker-socket-mount",
				Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"string docker socket mount with upper-case keys", `{"mounts": ["Type=Bind,SRC=/var/run/docker.sock,DST=/x"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-docker-socket-mount",
				Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"string docker socket mount with a quoted field", `{"mounts": ["type=bind,\"src=/var/run/docker.sock\",dst=/x"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-docker-socket-mount",
				Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"string docker socket mount with leading whitespace", `{"mounts": [" \"src=/var/run/docker.sock\",dst=/x,type=bind"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-docker-socket-mount",
				Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"object docker socket mount with upper-case type", `{"mounts": [{"source": "/var/run/docker.sock", "target": "/x", "type": "BIND"}]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-docker-socket-mount",
				Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"unrelated source named docker.sock", `{"mounts": ["source=/host/docker.sock,target=/var/run/docker.sock,type=bind"]}`, nil},
		{"string mount whose fields are not a CSV record", `{"mounts": ["type=bind,src=\"/var/run/docker.sock\",dst=/x"]}`, nil},
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
		{"runArgs --mount two tokens via src alias", `{"runArgs": ["--mount", "type=bind,src=/var/run/docker.sock,dst=/x"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 25, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs -v host path with a doubled leading slash", `{"runArgs": ["-v", "//var/run/docker.sock:/x"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 20, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs -v host path with a trailing slash", `{"runArgs": ["-v", "/var/run/docker.sock/:/x"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 20, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs every -v is reported", `{"runArgs": ["-v", "/var/run/docker.sock:/a", "-v", "/var/run/docker.sock:/b"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 20, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
			{Path: "devcontainer.json", Line: 1, Col: 53, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs unrelated volume", `{"runArgs": ["-v", "/host/docker.sock:/var/run/docker.sock"]}`, nil},
		// A -v value of a single field is an anonymous volume, and that field is the container path:
		// nothing from the host is bound.
		{"runArgs -v with only a container path", `{"runArgs": ["-v", "/var/run/docker.sock"]}`, nil},
		// A -v value is colon-separated, so a comma in it is part of a field rather than a separator.
		{"runArgs --volume with a comma in the container path", `{"runArgs": ["--volume", "myvol:/data,source=/var/run/docker.sock"]}`, nil},
		{"runArgs not an array", `{"runArgs": "-v /var/run/docker.sock:/x"}`, nil},
		{"runArgs duplicated, mounted by the first", `{"runArgs": ["-v", "/var/run/docker.sock:/x"], "runArgs": ["--init"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 20, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
		{"runArgs duplicated, mounted by the last", `{"runArgs": ["--init"], "runArgs": ["-v", "/var/run/docker.sock:/x"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 43, RuleID: "no-docker-socket-mount",
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoDockerSocketMount, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}
