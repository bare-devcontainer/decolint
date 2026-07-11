package rules

import "github.com/bare-devcontainer/decolint/linter"

// NoAppPort reports the legacy "appPort" property. It only supports statically publishing ports at
// container-creation time; "forwardPorts" is the modern replacement and forwards ports dynamically
// without requiring the container to be recreated.
type NoAppPort struct{}

// ID implements [linter.Rule].
func (NoAppPort) ID() string { return "no-app-port" }

// Description implements [linter.Rule].
func (NoAppPort) Description() string {
	return `disallow the legacy "appPort" property in favor of "forwardPorts"`
}

// FileTypes implements [linter.Rule].
func (NoAppPort) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (NoAppPort) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (NoAppPort) Paths() []string { return []string{"/appPort"} }

// Check implements [linter.Rule].
func (r NoAppPort) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	return []linter.Finding{{
		RuleID:  r.ID(),
		Message: `"appPort" is a legacy property; use "forwardPorts" instead to forward ports dynamically`,
		Offset:  node.Value.StartOffset,
	}}
}
