package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
)

// NoBindMount reports "mounts" entries that use the "bind" mount type. The Dev Container
// spec allows bind mounts, but GitHub Codespaces silently ignores them, except for a mount whose
// source is the Docker socket, so other bind mounts have no effect there.
var NoBindMount = &linter.Rule{
	ID:          "no-bind-mount",
	Description: `disallow "bind" type entries in "mounts", which GitHub Codespaces silently ignores except for the Docker socket`,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Platforms:   []linter.Platform{linter.PlatformCodespaces},
	Paths:       []string{"/mounts/*"},
	Check:       checkNoBindMount,
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
