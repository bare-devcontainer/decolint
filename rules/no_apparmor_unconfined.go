package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/dockerargs"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoApparmorUnconfined reports a devcontainer.json or devcontainer-feature.json that disables
// AppArmor confinement, either via the "securityOpt" property or, in a devcontainer.json, a
// "--security-opt apparmor=unconfined" entry in "runArgs". It is the AppArmor counterpart of
// [NoSeccompUnconfined]: both remove a mandatory confinement layer the runtime applies by default.
var NoApparmorUnconfined = &linter.Rule{
	ID:          "no-apparmor-unconfined",
	Description: `disallow disabling AppArmor confinement via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt apparmor=unconfined" entry in a devcontainer.json's "runArgs"`,
	LongDescription: `A container runtime applies its own AppArmor profile ("docker-default" for Docker) to every container on
a host that has AppArmor enabled, restricting what the container may do to the host's filesystem,
capabilities, and network. "apparmor=unconfined" removes that profile outright, so the only thing left
between a process in the container and the host is the discretionary access control the container's user
is already subject to. The setting is usually copied from instructions for running nested containers or a
debugger, both of which have narrower settings that work.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#general-properties`,
		`https://docs.docker.com/engine/security/apparmor/`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer, linter.Feature},
	Paths:     []string{"/securityOpt/*", "/runArgs/--security-opt"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "securityOpt": ["apparmor=unconfined"]
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
	},
	Check: checkNoApparmorUnconfined,
}

func checkNoApparmorUnconfined(_ *linter.Context, node *linter.Node) []linter.Finding {
	if node.Arg != nil {
		if !securityOptDisablesAppArmor(node.Arg.Value) {
			return nil
		}
		return []linter.Finding{{
			Message: fmt.Sprintf(`"runArgs" contains "--security-opt %s", disabling the container's AppArmor confinement`, node.Arg.Value),
			Offset:  node.Value.StartOffset,
		}}
	}

	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' || !securityOptDisablesAppArmor(lit.String()) {
		return nil
	}
	return []linter.Finding{{
		Message: fmt.Sprintf(`"securityOpt" contains %q, disabling the container's AppArmor confinement`, lit.String()),
		Offset:  node.Value.StartOffset,
	}}
}

// securityOptDisablesAppArmor reports whether s, a single "securityOpt" entry, removes the
// container's AppArmor profile.
func securityOptDisablesAppArmor(s string) bool {
	opt, ok := dockerargs.ParseSecurityOpt(s)
	return ok && opt.Key == "apparmor" && opt.Value == dockerargs.AppArmorProfileUnconfined
}
