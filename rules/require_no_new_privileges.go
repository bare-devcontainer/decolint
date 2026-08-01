package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// RequireNoNewPrivileges reports a devcontainer.json that does not set "no-new-privileges", either
// via the "securityOpt" property or a "--security-opt no-new-privileges..." entry in "runArgs".
// Without it, processes in the container can gain additional privileges through setuid/setgid
// binaries. It is off by default because most configs don't set it and enabling it by default would
// be noisy.
var RequireNoNewPrivileges = &linter.Rule{
	ID:          "require-no-new-privileges",
	Description: `require "no-new-privileges" to be set via a devcontainer.json's "securityOpt" property, or a "--security-opt no-new-privileges..." entry in "runArgs"`,
	LongDescription: `Without this option a process in the container can still gain privileges it was not started with, by
executing a setuid binary — which undercuts the point of running as a non-root user. Setting it raises the
kernel's "no_new_privs" bit, which every child process inherits and none can clear, so the container's
privileges can only ever shrink.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#general-properties`,
		`https://docs.kernel.org/userspace-api/no_new_privs.html`,
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
  "securityOpt": ["no-new-privileges"]
}
`},
			},
		},
	},
	Check: checkRequireNoNewPrivileges,
}

func checkRequireNoNewPrivileges(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	if stringArrayContains(obj, "securityOpt", securityOptIsNoNewPrivileges) {
		return nil
	}
	for arr := range arrayMembers(obj, "runArgs") {
		if runArgsFindFlagValue(arr, "--security-opt", securityOptIsNoNewPrivileges) != nil {
			return nil
		}
	}

	return []linter.Finding{{
		Message: `"no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges`,
		Offset:  node.Value.StartOffset,
	}}
}

// securityOptIsNoNewPrivileges reports whether s, a single "securityOpt" entry, sets
// "no-new-privileges". Docker treats the bare keyword as well as an explicit "=true" or ":true"
// value as enabling it; "=false" or ":false" leaves it disabled.
func securityOptIsNoNewPrivileges(s string) bool {
	switch s {
	case "no-new-privileges", "no-new-privileges=true", "no-new-privileges:true":
		return true
	}
	return false
}
