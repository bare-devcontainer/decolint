package rules

import (
	"fmt"
	"slices"

	"github.com/bare-devcontainer/decolint/dockerargs"
	"github.com/bare-devcontainer/decolint/linter"
)

// hostNamespace describes one of the "runArgs" flags that put the container in one of the host's
// namespaces instead of its own.
type hostNamespace struct {
	// namespace names what the flag shares with the host.
	namespace string
	// target reduces the flag's value to the namespace it names. It is nil for a flag whose value is
	// the namespace itself, which every one but the network flags is.
	target func(value string) string
}

// hostNamespaces holds those flags, keyed by the canonical long name the engine hands a rule (see
// [dockerargs.Arg]). "--net" is a flag docker/cli registers in its own right rather than a shorthand
// of "--network", so it is its own entry.
//
// The CIS Docker Benchmark has a control per namespace for all of these but the cgroup namespace,
// which postdates the benchmark's controls rather than being held to be safe to share.
var hostNamespaces = map[string]hostNamespace{
	"network":  {"network", dockerargs.NetworkTarget},
	"net":      {"network", dockerargs.NetworkTarget},
	"pid":      {"process", nil},
	"ipc":      {"IPC", nil},
	"uts":      {"UTS", nil},
	"userns":   {"user", nil},
	"cgroupns": {"cgroup", nil},
}

// hostNamespacePaths are the rule's Paths: one per flag in [hostNamespaces], so that the two cannot
// drift apart.
var hostNamespacePaths = func() []string {
	paths := make([]string, 0, len(hostNamespaces))
	for flag := range hostNamespaces {
		paths = append(paths, "/runArgs/--"+flag)
	}
	slices.Sort(paths)
	return paths
}()

// NoHostNamespace reports a devcontainer.json whose "runArgs" put the container in one of the host's
// namespaces, e.g. "--network=host" or "--pid=host". Each such entry removes the isolation that
// namespace provides.
var NoHostNamespace = &linter.Rule{
	ID:          "no-host-namespace",
	Description: `disallow sharing one of the host's namespaces with the container via a "--network=host", "--pid=host", "--ipc=host", "--uts=host", "--userns=host", or "--cgroupns=host" entry in "runArgs"`,
	LongDescription: `Namespaces are what make a container a container: the process table, network stack, and IPC objects it
sees are its own. A "host" value in "runArgs" hands one of them back, and the container stops being
isolated in that dimension. With "--pid=host" every process on the machine is visible from inside the
container, and a root process there can signal or trace it; with "--network=host" the container reaches
every service bound to the host's loopback interface, including the ones that are only reachable there
because they trust anything local. Put the container on a user-defined Docker network, or forward the
port you need, instead of joining the host's namespace.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#image-specific`,
		`https://github.com/docker/docker-bench-security`,
		`https://man7.org/linux/man-pages/man7/namespaces.7.html`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     hostNamespacePaths,
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "runArgs": ["--network=host"]
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "runArgs": ["--network=devnet"]
}
`},
			},
		},
		Note: `The good example puts the container on a user-defined Docker network, so it can
reach other containers on it by name while keeping a network namespace of its own.`,
	},
	Check: checkNoHostNamespace,
}

func checkNoHostNamespace(_ *linter.Context, node *linter.Node) []linter.Finding {
	ns := hostNamespaces[node.Arg.Flag]
	target := node.Arg.Value
	if ns.target != nil {
		target = ns.target(target)
	}
	if target != dockerargs.NetworkHost {
		return nil
	}
	// An entry a repeat of the same option supersedes is reported too. Which repeat is the live one
	// is a property of the order alone — Docker keeps the first "--network" value and the last of
	// every other one here — so an entry that is dead where it stands is live again the moment the
	// order changes. The message therefore says what the entry asks for rather than what the
	// container it is part of ends up with.
	return []linter.Finding{{
		Message: fmt.Sprintf(`"runArgs" contains "--%s=%s", which asks for the host's %s namespace instead of the container's own`,
			node.Arg.Flag, node.Arg.Value, ns.namespace),
		Offset: node.Value.StartOffset,
	}}
}
