package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/dockerargs"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// dangerousCapabilities maps each Linux capability that lets a process act on the host rather than
// on the container to what it allows. The keys are as [dockerargs.Capability] returns a name, so a
// capability written in any of its spellings is found here. None of them is in a container runtime's default set, so one
// appearing in "capAdd" was granted deliberately.
//
// The list is deliberately narrower than "every capability a container does not need": adding one
// means arguing that granting it reaches past the container, which for these means reaching a
// kernel subsystem that is not namespaced. A capability the kernel confines to the container's own
// namespaces belongs elsewhere however privileged it sounds — "SYS_PTRACE" (process namespace) and
// "NET_ADMIN" (network namespace) are the standing examples, and sharing the host's namespaces is
// what [NoHostNamespace] reports.
//
// "ALL" is not here: granting every capability at once is what [NoCapAddAll] reports.
var dangerousCapabilities = map[string]string{
	"CAP_AUDIT_CONTROL":   "allows reconfiguring the kernel's audit subsystem, which is not namespaced",
	"CAP_BPF":             "allows loading BPF programs into the host kernel",
	"CAP_DAC_READ_SEARCH": "bypasses file read permission checks and allows opening files by handle, outside the container's filesystem",
	"CAP_MAC_ADMIN":       "allows changing the host's mandatory access control policy",
	"CAP_MAC_OVERRIDE":    "bypasses the host's mandatory access control policy",
	"CAP_PERFMON":         "grants access to the kernel's performance monitoring interfaces, which observe the whole host",
	"CAP_SYSLOG":          "allows reading the host kernel's log, which discloses kernel addresses",
	"CAP_SYS_ADMIN":       "grants a broad range of administrative operations, including mounting filesystems",
	"CAP_SYS_BOOT":        "allows rebooting the host",
	"CAP_SYS_MODULE":      "allows loading modules into the host kernel",
	"CAP_SYS_RAWIO":       "allows raw access to the host's I/O ports and memory devices",
	"CAP_SYS_TIME":        "allows setting the clock, which the container shares with the host",
}

// NoDangerousCapAdd reports a devcontainer.json or devcontainer-feature.json that grants a Linux
// capability which lets a process reach past the container, either via the "capAdd" property or, in
// a devcontainer.json, a "--cap-add" entry in "runArgs". Unlike [NoCapAddAll], which only flags
// granting every capability at once, this rule flags the individual capabilities in
// dangerousCapabilities.
var NoDangerousCapAdd = &linter.Rule{
	ID:          "no-dangerous-cap-add",
	Description: `disallow granting a Linux capability that lets a process act on the host, e.g. "SYS_ADMIN" or "SYS_MODULE", via the "capAdd" property or a "--cap-add" entry in a devcontainer.json's "runArgs"`,
	LongDescription: `Container runtimes withhold the capabilities that let a process act on the host rather than on the
container, and "capAdd" adds them back one at a time. Each capability this rule reports is on its own
enough to reach past the container — loading a module into the host kernel, opening a file by handle
outside the mounted filesystem, rebooting the machine — and none of them is granted by default, so one
that appears here was asked for. Grant only what the workload actually fails without.

A capability the kernel confines to the container's own namespaces is not reported, however privileged
it sounds: "SYS_PTRACE" reaches no further than the container's process namespace, and "NET_ADMIN" no
further than its network namespace. What makes those dangerous is sharing the host's namespaces, which
is a separate rule.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#general-properties`,
		`https://docs.docker.com/engine/security/#linux-kernel-capabilities`,
		`https://man7.org/linux/man-pages/man7/capabilities.7.html`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer, linter.Feature},
	Paths:     []string{"/capAdd/*", "/runArgs/--cap-add"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "capAdd": ["SYS_ADMIN"]
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
	Check: checkNoDangerousCapAdd,
}

func checkNoDangerousCapAdd(_ *linter.Context, node *linter.Node) []linter.Finding {
	if node.Arg != nil {
		effect, dangerous := dangerousCapabilities[dockerargs.Capability(node.Arg.Value)]
		if !dangerous {
			return nil
		}
		return []linter.Finding{{
			Message: fmt.Sprintf(`"runArgs" contains "--cap-add=%s", which %s`, node.Arg.Value, effect),
			Offset:  node.Value.StartOffset,
		}}
	}

	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	effect, dangerous := dangerousCapabilities[dockerargs.Capability(lit.String())]
	if !dangerous {
		return nil
	}
	return []linter.Finding{{
		Message: fmt.Sprintf(`"capAdd" contains %q, which %s`, lit.String(), effect),
		Offset:  node.Value.StartOffset,
	}}
}
