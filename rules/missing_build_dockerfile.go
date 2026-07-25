package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingBuildDockerfile reports a devcontainer.json "build" object that does not set "dockerfile",
// leaving no way to know which Dockerfile to build.
var MissingBuildDockerfile = &linter.Rule{
	ID:          "missing-build-dockerfile",
	Description: `disallow a devcontainer.json "build" object that is missing "dockerfile"`,
	Category:    linter.CategoryCorrectness,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{"/build"},
	Check:       checkMissingBuildDockerfile,
}

func checkMissingBuildDockerfile(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok || hasMember(obj, "dockerfile") {
		return nil
	}
	return []linter.Finding{{
		Message: `"build" is missing "dockerfile"`,
		Offset:  node.Value.StartOffset,
	}}
}
