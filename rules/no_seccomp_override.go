package rules

import (
	"github.com/bare-devcontainer/decolint/dockerargs"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoSeccompOverride reports a devcontainer.json or devcontainer-feature.json that overrides the
// container runtime's default seccomp profile, either via the "securityOpt" property or, in a
// devcontainer.json, a "--security-opt seccomp=..." entry in "runArgs". Unlike
// [NoSeccompUnconfined], which only flags disabling seccomp entirely, this rule flags any override,
// including a custom profile, since it replaces the runtime's vetted default. It is off by default
// because many projects legitimately ship a custom profile.
var NoSeccompOverride = &linter.Rule{
	ID:          "no-seccomp-override",
	Description: `disallow overriding the container runtime's default seccomp profile via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt seccomp=..." entry in a devcontainer.json's "runArgs"`,
	LongDescription: `The runtime's default seccomp profile blocks the syscalls containers do not need, several of which have
featured in container escapes. Pointing "seccomp" at a profile of your own replaces that default
wholesale, and a hand-written profile is rarely reviewed as carefully or updated as the kernel gains new
syscalls. Keep the default unless the workload provably needs more, and review the replacement if it
does.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#general-properties`,
		`https://docs.docker.com/engine/security/seccomp/`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer, linter.Feature},
	Paths:     []string{"/securityOpt/*", "/runArgs/--security-opt"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "securityOpt": ["seccomp=./seccomp.json"]
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "capAdd": ["SYS_PTRACE"]
}
`},
			},
		},
		Note: `Leaving ` + "`" + `securityOpt` + "`" + ` unset keeps the runtime's default seccomp profile,
which already allows what a development container normally does.`,
	},
	Check: checkNoSeccompOverride,
}

func checkNoSeccompOverride(_ *linter.Context, node *linter.Node) []linter.Finding {
	if node.Arg != nil {
		if !securityOptOverridesSeccomp(node.Arg.Value) {
			return nil
		}
		return []linter.Finding{{
			Message: `"runArgs" overrides the default seccomp profile via "--security-opt"`,
			Offset:  node.Value.StartOffset,
		}}
	}

	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' || !securityOptOverridesSeccomp(lit.String()) {
		return nil
	}
	return []linter.Finding{{
		Message: `"securityOpt" overrides the default seccomp profile`,
		Offset:  node.Value.StartOffset,
	}}
}

// securityOptOverridesSeccomp reports whether s, a single "securityOpt" entry, points seccomp at
// anything other than the runtime's own default profile. Naming that default explicitly leaves the
// container exactly where it started, so it is not an override.
func securityOptOverridesSeccomp(s string) bool {
	profile, ok := securityOptSeccompProfile(s)
	return ok && profile != dockerargs.SeccompProfileDefault
}
