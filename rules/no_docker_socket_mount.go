package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoDockerSocketMount reports a devcontainer.json that bind-mounts the host's Docker daemon socket
// into the container, either via a "mounts" entry or a "-v"/"--volume"/"--mount" entry in
// "runArgs". Anything with access to the socket can control the host's Docker daemon, which is
// effectively root-equivalent access to the host.
var NoDockerSocketMount = &linter.Rule{
	ID:          "no-docker-socket-mount",
	Description: `disallow bind-mounting the host's Docker socket via a devcontainer.json's "mounts" or "runArgs", which grants the container root-equivalent control over the host`,
	LongDescription: `The Docker socket is the daemon's full API, and the daemon runs as root on the host. Anything that can
reach the socket can start a container that mounts the host's filesystem, so mounting it into the dev
container hands root-equivalent control of the host to every process inside — including code the
project's own build fetches. When the container genuinely needs Docker, a Docker-in-Docker Feature or a
rootless daemon keeps that access inside the container.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#general-properties`,
		`https://docs.docker.com/engine/security/#docker-daemon-attack-surface`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{"/mounts/*", "/runArgs"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "mounts": [
    {
      "source": "/var/run/docker.sock",
      "target": "/var/run/docker.sock",
      "type": "bind"
    }
  ]
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/docker-in-docker:2.13.0": {}
  }
}
`},
			},
		},
	},
	Check: checkNoDockerSocketMount,
}

func checkNoDockerSocketMount(_ *linter.Context, node *linter.Node) []linter.Finding {
	if node.Pointer == "/runArgs" {
		return checkDockerSocketRunArgs(node)
	}
	return checkDockerSocketMount(node)
}

func checkDockerSocketMount(node *linter.Node) []linter.Finding {
	_, source, ok := parseMount(node.Value)
	if !ok || !isDockerSocketSource(source) {
		return nil
	}
	return []linter.Finding{{
		Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`,
		Offset:  node.Value.StartOffset,
	}}
}

// dockerSocketRunArgFlags are the "runArgs" flags that can mount a host path, each paired with the
// reader for its own value syntax. The two syntaxes are unrelated, so a value must be read only as
// the flag introducing it, which is why this rule inspects the whole "runArgs" array rather than its
// entries one by one.
var dockerSocketRunArgFlags = []struct {
	flag   string
	source func(string) string
}{
	{"--mount", func(s string) string { _, source := parseMountString(s); return source }},
	{"--volume", volumeSpecSource},
	{"-v", volumeSpecSource},
}

func checkDockerSocketRunArgs(node *linter.Node) []linter.Finding {
	arr, ok := node.Value.Value.(*hujson.Array)
	if !ok {
		return nil
	}
	var findings []linter.Finding
	for _, f := range dockerSocketRunArgFlags {
		for value, s := range runArgsFlagValues(arr, f.flag) {
			if !isDockerSocketSource(f.source(s)) {
				continue
			}
			findings = append(findings, linter.Finding{
				Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`,
				Offset:  value.StartOffset,
			})
		}
	}
	return findings
}
