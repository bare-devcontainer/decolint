package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoSeccompUnconfined reports a devcontainer.json or devcontainer-feature.json that disables
// seccomp confinement, either via the "securityOpt" property or, in a devcontainer.json, a
// "--security-opt seccomp=unconfined" entry in "runArgs". Running unconfined removes a key layer of
// kernel-level syscall filtering that isolates the container from the host.
var NoSeccompUnconfined = &linter.Rule{
	ID:          "no-seccomp-unconfined",
	Description: `disallow disabling seccomp confinement via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt seccomp=unconfined" entry in a devcontainer.json's "runArgs"`,
	LongDescription: `"seccomp=unconfined" turns off syscall filtering entirely, exposing the whole kernel API — including the
calls the default profile blocks precisely because they have been used to break out of containers. The
setting is most often copied from debugger instructions, where granting the "SYS_PTRACE" capability is
enough on current runtimes.`,
	References: []string{
		"https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties",
		"https://docs.docker.com/engine/security/seccomp/",
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer, linter.Feature},
	Paths:     []string{"/securityOpt/*", "/runArgs"},
	Check:     checkNoSeccompUnconfined,
}

func checkNoSeccompUnconfined(ctx *linter.Context, node *linter.Node) []linter.Finding {
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
