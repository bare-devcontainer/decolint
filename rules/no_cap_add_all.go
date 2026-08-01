package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoCapAddAll reports a devcontainer.json or devcontainer-feature.json that grants every Linux
// capability to the container, either via an "ALL" entry in the "capAdd" property or, in a
// devcontainer.json, a "--cap-add=ALL" entry in "runArgs". Granting all capabilities gives the
// container far more privilege than most workloads need, which is a significant security risk.
var NoCapAddAll = &linter.Rule{
	ID:          "no-cap-add-all",
	Description: `disallow granting all Linux capabilities via an "ALL" entry in the "capAdd" property, or a "--cap-add=ALL" entry in a devcontainer.json's "runArgs"`,
	LongDescription: `Linux capabilities split root's powers into units a container can be granted individually, and the runtime
withholds the dangerous ones by default. "ALL" hands them all over, including capabilities such as
"SYS_ADMIN" and "SYS_MODULE" that let a process reconfigure the host kernel and escape the container.
"capAdd" exists to name the one or two a workload actually needs, e.g. "SYS_PTRACE" for a debugger.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#general-properties`,
		`https://docs.docker.com/engine/security/#linux-kernel-capabilities`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer, linter.Feature},
	Paths:     []string{"/capAdd/*", "/runArgs/--cap-add"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "capAdd": ["ALL"]
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
	Check: checkNoCapAddAll,
}

func checkNoCapAddAll(_ *linter.Context, node *linter.Node) []linter.Finding {
	if node.Arg != nil {
		if !isAllCapability(node.Arg.Value) {
			return nil
		}
		return []linter.Finding{{
			Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`,
			Offset:  node.Value.StartOffset,
		}}
	}

	if underRunArgs(node) {
		return nil
	}
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' || !isAllCapability(lit.String()) {
		return nil
	}
	return []linter.Finding{{
		Message: `"capAdd" contains "ALL", granting every Linux capability to the container`,
		Offset:  node.Value.StartOffset,
	}}
}
