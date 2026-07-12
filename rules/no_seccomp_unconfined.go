package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoSeccompUnconfined reports a devcontainer.json or devcontainer-feature.json that disables
// seccomp confinement, either via the "securityOpt" property or, in a devcontainer.json, a
// "--security-opt seccomp=unconfined" entry in "runArgs". Running unconfined removes a key layer of
// kernel-level syscall filtering that isolates the container from the host.
type NoSeccompUnconfined struct{}

// ID implements [linter.Rule].
func (NoSeccompUnconfined) ID() string { return "no-seccomp-unconfined" }

// Description implements [linter.Rule].
func (NoSeccompUnconfined) Description() string {
	return `disallow disabling seccomp confinement via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt seccomp=unconfined" entry in a devcontainer.json's "runArgs"`
}

// FileTypes implements [linter.Rule].
func (NoSeccompUnconfined) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer, linter.Feature}
}

// Platforms implements [linter.Rule].
func (NoSeccompUnconfined) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (NoSeccompUnconfined) Paths() []string {
	return []string{"/securityOpt/*", "/runArgs"}
}

// Check implements [linter.Rule].
func (NoSeccompUnconfined) Check(ctx *linter.Context, node *linter.Node) []linter.Finding {
	if node.Pointer == "/runArgs" {
		arr, ok := node.Value.Value.(*hujson.Array)
		if !ok || !runArgsApplicable(ctx) {
			return nil
		}
		v := runArgsFindFlagValue(arr, "--security-opt", func(s string) bool { return s == "seccomp=unconfined" })
		if v == nil {
			return nil
		}
		return []linter.Finding{{
			Message: `"runArgs" contains "--security-opt seccomp=unconfined", disabling the container's syscall filtering`,
			Offset:  v.StartOffset,
		}}
	}

	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' || lit.String() != "seccomp=unconfined" {
		return nil
	}
	return []linter.Finding{{
		Message: `"securityOpt" contains "seccomp=unconfined", disabling the container's syscall filtering`,
		Offset:  node.Value.StartOffset,
	}}
}
