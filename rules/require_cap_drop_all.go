package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// RequireCapDropAll reports a devcontainer.json that does not drop all Linux capabilities via a
// "--cap-drop=ALL" entry in "runArgs". Dropping every capability and adding back only what's needed
// (e.g. via "capAdd") follows the principle of least privilege. It is off by default because most
// configs don't set it and enabling it by default would be noisy.
var RequireCapDropAll = &linter.Rule{
	ID:          "require-cap-drop-all",
	Description: `require a "--cap-drop=ALL" entry in a devcontainer.json's "runArgs", dropping every Linux capability`,
	LongDescription: `Container runtimes grant a default set of capabilities that a dev container almost never uses: raw network
access, changing file ownership, or binding privileged ports. Dropping all of them and adding back only
what the workload needs ("capAdd") means a process that is compromised inherits no privilege the project
never asked for.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties`,
		`https://docs.docker.com/engine/security/#linux-kernel-capabilities`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu"
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "runArgs": ["--cap-drop=ALL"],
  "capAdd": ["CHOWN", "SETUID", "SETGID"]
}
`},
			},
		},
		Note: "`" + `runArgs` + "`" + ` is the only place this can be expressed: devcontainer.json has a
` + "`" + `capAdd` + "`" + ` property but no ` + "`" + `capDrop` + "`" + ` one, so dropping capabilities means passing
the flag to the container runtime. Add back through ` + "`" + `capAdd` + "`" + ` whatever the
workload actually needs.`,
	},
	Check: checkRequireCapDropAll,
}

func checkRequireCapDropAll(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	if arr, ok := arrayMember(obj, "runArgs"); ok {
		if runArgsFindFlagValue(arr, "--cap-drop", func(s string) bool { return s == "ALL" }) != nil {
			return nil
		}
	}

	return []linter.Finding{{
		Message: `"ALL" is not set via "runArgs", leaving the container with its default Linux capabilities`,
		Offset:  node.Value.StartOffset,
	}}
}
