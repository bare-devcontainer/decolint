package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingContainerDef reports a devcontainer.json that defines none of "image", "build", or
// "dockerComposeFile", leaving no way to build a container.
type MissingContainerDef struct{}

// ID implements [linter.Rule].
func (MissingContainerDef) ID() string { return "missing-container-def" }

// Description implements [linter.Rule].
func (MissingContainerDef) Description() string {
	return `disallow a devcontainer.json that defines none of "image", "build", or "dockerComposeFile"`
}

// FileTypes implements [linter.Rule].
func (MissingContainerDef) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (MissingContainerDef) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (MissingContainerDef) Paths() []string { return []string{""} }

// Check implements [linter.Rule].
func (r MissingContainerDef) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	for _, name := range []string{"image", "build", "dockerComposeFile"} {
		if hasMember(obj, name) {
			return nil
		}
	}
	return []linter.Finding{{
		RuleID:  r.ID(),
		Message: `devcontainer.json must define one of "image", "build", or "dockerComposeFile"`,
		Offset:  node.Value.StartOffset,
	}}
}
