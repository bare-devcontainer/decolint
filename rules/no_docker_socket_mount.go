package rules

import (
	"strings"

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
	Category:    linter.CategorySecurity,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{"/mounts/*", "/runArgs/*"},
	Check:       checkNoDockerSocketMount,
}

func checkNoDockerSocketMount(_ *linter.Context, node *linter.Node) []linter.Finding {
	if strings.HasPrefix(node.Pointer, "/mounts/") {
		return checkDockerSocketMount(node)
	}
	return checkDockerSocketRunArg(node)
}

func checkDockerSocketMount(node *linter.Node) []linter.Finding {
	_, source, ok := parseMount(node.Value)
	if !ok || source != dockerSocketPath {
		return nil
	}
	return []linter.Finding{{
		Message: `"mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host`,
		Offset:  node.Value.StartOffset,
	}}
}

func checkDockerSocketRunArg(node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' || !runArgMountsDockerSocket(lit.String()) {
		return nil
	}
	return []linter.Finding{{
		Message: `"runArgs" bind-mounts the Docker socket, which grants the container root-equivalent control over the host`,
		Offset:  node.Value.StartOffset,
	}}
}

// runArgMountsDockerSocket reports whether s, a single "runArgs" entry, bind-mounts the Docker
// socket. It recognizes a "--mount"-style "key=value,..." entry with a matching "source", and a
// "-v"/"--volume" entry, with or without an "=" before the value, whose host path is the Docker
// socket.
func runArgMountsDockerSocket(s string) bool {
	if strings.Contains(s, ",") {
		if _, source := parseMountString(strings.TrimPrefix(s, "--mount=")); source == dockerSocketPath {
			return true
		}
	}
	for _, prefix := range []string{"--volume=", "-v="} {
		s = strings.TrimPrefix(s, prefix)
	}
	return s == dockerSocketPath || strings.HasPrefix(s, dockerSocketPath+":")
}
