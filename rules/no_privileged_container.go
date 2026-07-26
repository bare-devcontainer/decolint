package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoPrivilegedContainer reports a devcontainer.json or devcontainer-feature.json that runs the
// container in privileged mode, either via the "privileged" property or, in a devcontainer.json, a
// "--privileged" entry in "runArgs". Privileged mode disables the container's isolation from the
// host, which is a significant security risk.
var NoPrivilegedContainer = &linter.Rule{
	ID:          "no-privileged-container",
	Description: `disallow running the container in privileged mode via the "privileged" property or a "--privileged" entry in "runArgs"`,
	LongDescription: `A privileged container gets every Linux capability, unconfined seccomp and LSM profiles, and access to all
host devices. That removes essentially every boundary between the container and the host, so any code
running in it — including a compromised dependency pulled in by the project's own build — can take over
the machine. Docker-in-Docker is the usual reason it is set; a Feature that provides it, or the specific
capabilities and devices the workload needs, is a far narrower grant.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties`,
		`https://docs.docker.com/engine/security/#docker-daemon-attack-surface`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer, linter.Feature},
	Paths:     []string{"/privileged", "/runArgs/*"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "privileged": true
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
		Note: `The good example grants only the capability a debugger needs. Reach for the
narrowest grant that works: ` + "`" + `capAdd` + "`" + ` for a capability, ` + "`" + `--device` + "`" + ` in ` + "`" + `runArgs` + "`" + `
for a device, and the docker-in-docker Feature rather than privileged mode for
nested containers.`,
	},
	Check: checkNoPrivilegedContainer,
}

func checkNoPrivilegedContainer(ctx *linter.Context, node *linter.Node) []linter.Finding {
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
			Message: `"privileged" is set to true, disabling the container's isolation from the host`,
			Offset:  node.Value.StartOffset,
		}}
	default:
		if !runArgsApplicable(ctx) || lit.Kind() != '"' || lit.String() != "--privileged" {
			return nil
		}
		return []linter.Finding{{
			Message: `"runArgs" contains "--privileged", disabling the container's isolation from the host`,
			Offset:  node.Value.StartOffset,
		}}
	}
}
