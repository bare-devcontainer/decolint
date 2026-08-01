package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
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
	Paths:     []string{"/mounts/*", "/runArgs/--mount", "/runArgs/--volume"},
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
	if node.Arg != nil {
		return checkDockerSocketRunArg(node)
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

// checkDockerSocketRunArg reports the host path node's "runArgs" flag mounts, if it is the Docker
// socket. The value syntaxes of the two flags that can mount one are unrelated, so a value is read
// only as the flag introducing it.
func checkDockerSocketRunArg(node *linter.Node) []linter.Finding {
	var source string
	switch node.Arg.Flag {
	case "mount":
		_, source = parseMountString(node.Arg.Value)
	case "volume":
		source = volumeSpecSource(node.Arg.Value)
	}
	if !isDockerSocketSource(source) {
		return nil
	}
	return []linter.Finding{{
		Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`,
		Offset:  node.Value.StartOffset,
	}}
}
