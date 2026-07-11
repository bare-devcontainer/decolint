package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoPrivilegedContainer reports a devcontainer.json or devcontainer-feature.json that runs the
// container in privileged mode, either via the "privileged" property or, in a devcontainer.json, a
// "--privileged" entry in "runArgs". Privileged mode disables the container's isolation from the
// host, which is a significant security risk.
type NoPrivilegedContainer struct{}

// ID implements [linter.Rule].
func (NoPrivilegedContainer) ID() string { return "no-privileged-container" }

// Description implements [linter.Rule].
func (NoPrivilegedContainer) Description() string {
	return `disallow running the container in privileged mode via the "privileged" property or a "--privileged" entry in "runArgs"`
}

// FileTypes implements [linter.Rule].
func (NoPrivilegedContainer) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer, linter.Feature}
}

// Platforms implements [linter.Rule].
func (NoPrivilegedContainer) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (NoPrivilegedContainer) Paths() []string {
	return []string{"/privileged", "/runArgs/*"}
}

// Check implements [linter.Rule].
func (r NoPrivilegedContainer) Check(ctx *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok {
		return nil
	}

	switch node.Pointer {
	case "/privileged":
		if lit.Kind() != 't' {
			return nil
		}
		return []linter.Finding{{
			RuleID:  r.ID(),
			Message: `"privileged" is set to true, disabling the container's isolation from the host`,
			Offset:  node.Value.StartOffset,
		}}
	default:
		if !runArgsApplicable(ctx) || lit.Kind() != '"' || lit.String() != "--privileged" {
			return nil
		}
		return []linter.Finding{{
			RuleID:  r.ID(),
			Message: `"runArgs" contains "--privileged", disabling the container's isolation from the host`,
			Offset:  node.Value.StartOffset,
		}}
	}
}
