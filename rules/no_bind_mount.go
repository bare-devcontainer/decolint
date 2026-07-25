package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
)

// NoBindMount reports "mounts" entries that use the "bind" mount type. The Dev Container
// spec allows bind mounts, but GitHub Codespaces silently ignores them, except for a mount whose
// source is the Docker socket.
var NoBindMount = &linter.Rule{
	ID:          "no-bind-mount",
	Description: `disallow "bind" type entries in "mounts", which GitHub Codespaces silently ignores except for the Docker socket`,
	LongDescription: `A codespace runs on a machine in the cloud, where the host path a bind mount points at does not exist, so
Codespaces documents that it ignores "bind" mounts apart from the Docker socket. The mount is dropped
without an error and the container starts missing the data it expects. Volume mounts are honored, so use
"type=volume" for anything that only has to persist across rebuilds.`,
	References: []string{
		"https://github.com/devcontainers/spec/blob/main/docs/specs/supporting-tools.md#github-codespaces",
		"https://containers.dev/implementors/spec/#mounts",
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Platforms: []linter.Platform{linter.PlatformCodespaces},
	Paths:     []string{"/mounts/*"},
	Check:     checkNoBindMount,
}

func checkNoBindMount(_ *linter.Context, node *linter.Node) []linter.Finding {
	mountType, source, ok := parseMount(node.Value)
	if !ok || mountType != "bind" || source == dockerSocketPath {
		return nil
	}
	return []linter.Finding{{
		Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`,
		Offset:  node.Value.StartOffset,
	}}
}
