package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingBuildDockerfile reports a devcontainer.json "build" object that does not set "dockerfile",
// leaving no way to know which Dockerfile to build.
type MissingBuildDockerfile struct{}

// ID implements [linter.Rule].
func (MissingBuildDockerfile) ID() string { return "missing-build-dockerfile" }

// Description implements [linter.Rule].
func (MissingBuildDockerfile) Description() string {
	return `disallow a devcontainer.json "build" object that is missing "dockerfile"`
}

// FileTypes implements [linter.Rule].
func (MissingBuildDockerfile) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (MissingBuildDockerfile) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (MissingBuildDockerfile) Paths() []string { return []string{"/build"} }

// Check implements [linter.Rule].
func (MissingBuildDockerfile) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok || hasMember(obj, "dockerfile") {
		return nil
	}
	return []linter.Finding{{
		Message: `"build" is missing "dockerfile"`,
		Offset:  node.Value.StartOffset,
	}}
}
