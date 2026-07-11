package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// RequireCapDropAll reports a devcontainer.json that does not drop all Linux capabilities, either
// via an "ALL" entry in the "capDrop" property or a "--cap-drop=ALL" entry in "runArgs". Dropping
// every capability and adding back only what's needed (e.g. via "capAdd") follows the principle of
// least privilege. It is off by default because most configs don't set it and enabling it by
// default would be noisy.
type RequireCapDropAll struct{}

// ID implements [linter.Rule].
func (RequireCapDropAll) ID() string { return "require-cap-drop-all" }

// Description implements [linter.Rule].
func (RequireCapDropAll) Description() string {
	return `require an "ALL" entry in a devcontainer.json's "capDrop" property, or a "--cap-drop=ALL" entry in "runArgs", dropping every Linux capability`
}

// FileTypes implements [linter.Rule].
func (RequireCapDropAll) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (RequireCapDropAll) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (RequireCapDropAll) Paths() []string { return []string{""} }

// Check implements [linter.Rule].
func (r RequireCapDropAll) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	if stringArrayContains(obj, "capDrop", func(s string) bool { return s == "ALL" }) {
		return nil
	}
	if arr, ok := arrayMember(obj, "runArgs"); ok {
		if runArgsFindFlagValue(arr, "--cap-drop", func(s string) bool { return s == "ALL" }) != nil {
			return nil
		}
	}

	return []linter.Finding{{
		RuleID:  r.ID(),
		Message: `"ALL" is not set via "capDrop" or "runArgs", leaving the container with its default Linux capabilities`,
		Offset:  node.Value.StartOffset,
	}}
}
