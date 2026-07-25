package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingContainerDef reports a devcontainer.json that defines none of "image", "build", or
// "dockerComposeFile", leaving no way to build a container.
var MissingContainerDef = &linter.Rule{
	ID:          "missing-container-def",
	Description: `disallow a devcontainer.json that defines none of "image", "build", or "dockerComposeFile"`,
	Category:    linter.CategoryCorrectness,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{""},
	Check:       checkMissingContainerDef,
}

func checkMissingContainerDef(_ *linter.Context, node *linter.Node) []linter.Finding {
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
		Message: `devcontainer.json must define one of "image", "build", or "dockerComposeFile"`,
		Offset:  node.Value.StartOffset,
	}}
}
