package rules

import (
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoSeccompOverride reports a devcontainer.json or devcontainer-feature.json that overrides the
// container runtime's default seccomp profile, either via the "securityOpt" property or, in a
// devcontainer.json, a "--security-opt seccomp=..." entry in "runArgs". Unlike
// [NoSeccompUnconfined], which only flags disabling seccomp entirely, this rule flags any override,
// including a custom profile, since it replaces the runtime's vetted default. It is off by default
// because many projects legitimately ship a custom profile.
type NoSeccompOverride struct{}

// ID implements [linter.Rule].
func (NoSeccompOverride) ID() string { return "no-seccomp-override" }

// Description implements [linter.Rule].
func (NoSeccompOverride) Description() string {
	return `disallow overriding the container runtime's default seccomp profile via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt seccomp=..." entry in a devcontainer.json's "runArgs"`
}

// FileTypes implements [linter.Rule].
func (NoSeccompOverride) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer, linter.Feature}
}

// Platforms implements [linter.Rule].
func (NoSeccompOverride) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (NoSeccompOverride) Paths() []string {
	return []string{"/securityOpt/*", "/runArgs/*"}
}

// Check implements [linter.Rule].
func (NoSeccompOverride) Check(ctx *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}

	if strings.HasPrefix(node.Pointer, "/securityOpt/") {
		if !strings.HasPrefix(lit.String(), "seccomp=") {
			return nil
		}
		return []linter.Finding{{
			Message: `"securityOpt" overrides the default seccomp profile`,
			Offset:  node.Value.StartOffset,
		}}
	}

	if !runArgsApplicable(ctx) || !runArgOverridesSeccomp(lit.String()) {
		return nil
	}
	return []linter.Finding{{
		Message: `"runArgs" overrides the default seccomp profile via "--security-opt"`,
		Offset:  node.Value.StartOffset,
	}}
}

// runArgOverridesSeccomp reports whether s, a single "runArgs" entry, overrides the default seccomp
// profile. It recognizes a combined "--security-opt=seccomp=..." entry as well as the bare
// "seccomp=..." value that follows a separate "--security-opt" entry.
func runArgOverridesSeccomp(s string) bool {
	return strings.HasPrefix(strings.TrimPrefix(s, "--security-opt="), "seccomp=")
}
