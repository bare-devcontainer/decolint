package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
)

// CodespacesNoBindMount reports "mounts" entries that use the "bind" mount type. The Dev Container
// spec allows bind mounts, but GitHub Codespaces silently ignores them, except for a mount whose
// source is the Docker socket, so other bind mounts have no effect there.
type CodespacesNoBindMount struct{}

// ID implements [linter.Rule].
func (CodespacesNoBindMount) ID() string { return "codespaces-no-bind-mount" }

// Description implements [linter.Rule].
func (CodespacesNoBindMount) Description() string {
	return `disallow "bind" type entries in "mounts", which GitHub Codespaces silently ignores except for the Docker socket`
}

// FileTypes implements [linter.Rule].
func (CodespacesNoBindMount) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (CodespacesNoBindMount) Platforms() []linter.Platform {
	return []linter.Platform{linter.PlatformCodespaces}
}

// Paths implements [linter.Rule].
func (CodespacesNoBindMount) Paths() []string { return []string{"/mounts/*"} }

// Check implements [linter.Rule].
func (CodespacesNoBindMount) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	mountType, source, ok := parseMount(node.Value)
	if !ok || mountType != "bind" || source == dockerSocketPath {
		return nil
	}
	return []linter.Finding{{
		Message: `"mounts" entry uses the "bind" type, which GitHub Codespaces silently ignores`,
		Offset:  node.Value.StartOffset,
	}}
}
