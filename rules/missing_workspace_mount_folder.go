package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingWorkspaceMountFolder reports a devcontainer.json that uses "image" or "build" and sets
// only one of "workspaceMount" or "workspaceFolder", leaving the tool unable to tell where the
// overridden mount lands inside the container.
type MissingWorkspaceMountFolder struct{}

// ID implements [linter.Rule].
func (MissingWorkspaceMountFolder) ID() string { return "missing-workspace-mount-folder" }

// Description implements [linter.Rule].
func (MissingWorkspaceMountFolder) Description() string {
	return `disallow a devcontainer.json using "image" or "build" that sets only one of "workspaceMount" or "workspaceFolder"`
}

// FileTypes implements [linter.Rule].
func (MissingWorkspaceMountFolder) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (MissingWorkspaceMountFolder) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (MissingWorkspaceMountFolder) Paths() []string { return []string{""} }

// Check implements [linter.Rule].
func (r MissingWorkspaceMountFolder) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok || (!hasMember(obj, "image") && !hasMember(obj, "build")) {
		return nil
	}

	hasMount := hasMember(obj, "workspaceMount")
	hasFolder := hasMember(obj, "workspaceFolder")
	if hasMount == hasFolder {
		return nil
	}

	present, missing := "workspaceMount", "workspaceFolder"
	if hasFolder {
		present, missing = "workspaceFolder", "workspaceMount"
	}
	return []linter.Finding{{
		RuleID:  r.ID(),
		Message: fmt.Sprintf("devcontainer.json sets %q but is missing %q", present, missing),
		Offset:  node.Value.StartOffset,
	}}
}
