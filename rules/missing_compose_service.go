package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingComposeService reports a devcontainer.json that sets "dockerComposeFile" without also
// setting "service", leaving the tool no way to know which compose service to attach to.
type MissingComposeService struct{}

// ID implements [linter.Rule].
func (MissingComposeService) ID() string { return "missing-compose-service" }

// Description implements [linter.Rule].
func (MissingComposeService) Description() string {
	return `disallow a devcontainer.json that sets "dockerComposeFile" without "service"`
}

// FileTypes implements [linter.Rule].
func (MissingComposeService) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (MissingComposeService) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (MissingComposeService) Paths() []string { return []string{""} }

// Check implements [linter.Rule].
func (r MissingComposeService) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok || !hasMember(obj, "dockerComposeFile") || hasMember(obj, "service") {
		return nil
	}
	return []linter.Finding{{
		RuleID:  r.ID(),
		Message: `devcontainer.json sets "dockerComposeFile" but is missing "service"`,
		Offset:  node.Value.StartOffset,
	}}
}
