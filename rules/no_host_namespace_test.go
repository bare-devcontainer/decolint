package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoHostNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no runArgs property", `{"name": "test"}`, nil},
		{"runArgs without a namespace flag", `{"runArgs": ["--init"]}`, nil},
		{"network on a user-defined network", `{"runArgs": ["--network=devnet"]}`, nil},
		{"a host value for another flag", `{"runArgs": ["--add-host=host"]}`, nil},
		{"network=host", `{"runArgs": ["--network=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=host", which asks for the host's network namespace instead of the container's own`},
		}},
		{"net=host uses the alias", `{"runArgs": ["--net=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--net=host", which asks for the host's network namespace instead of the container's own`},
		}},
		{"pid host two tokens", `{"runArgs": ["--pid", "host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 23, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--pid=host", which asks for the host's process namespace instead of the container's own`},
		}},
		{"ipc=host", `{"runArgs": ["--ipc=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--ipc=host", which asks for the host's IPC namespace instead of the container's own`},
		}},
		{"uts=host", `{"runArgs": ["--uts=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--uts=host", which asks for the host's UTS namespace instead of the container's own`},
		}},
		{"userns=host", `{"runArgs": ["--userns=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--userns=host", which asks for the host's user namespace instead of the container's own`},
		}},
		{"cgroupns=host", `{"runArgs": ["--cgroupns=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--cgroupns=host", which asks for the host's cgroup namespace instead of the container's own`},
		}},
		{"ipc container reference is not the host", `{"runArgs": ["--ipc=container:other"]}`, nil},
		// Docker reads a "--network" value carrying a field value as a field list, in which "name"
		// holds the network, and lower-cases each field.
		{"network field list naming the host", `{"runArgs": ["--network=name=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=name=host", which asks for the host's network namespace instead of the container's own`},
		}},
		{"network field list is matched case-insensitively", `{"runArgs": ["--net=NAME=HOST"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--net=NAME=HOST", which asks for the host's network namespace instead of the container's own`},
		}},
		{"network field list with the host among other fields", `{"runArgs": ["--network=alias=web,name=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=alias=web,name=host", which asks for the host's network namespace instead of the container's own`},
		}},
		{"network field list naming another network", `{"runArgs": ["--network=name=devnet,alias=web"]}`, nil},
		// Docker assigns the network from every "name" field it reads, so a repeated one leaves the
		// last.
		{"network field list naming the host last", `{"runArgs": ["--network=name=devnet,name=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=name=devnet,name=host", which asks for the host's network namespace instead of the container's own`},
		}},
		{"network field list naming the host but not last", `{"runArgs": ["--network=name=host,name=devnet"]}`, nil},
		// Docker trims the space around a field's key and value, so where it falls does not matter.
		{"network field list with space around the fields", `{"runArgs": ["--network=alias=web, name = host "]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=alias=web, name = host ", which asks for the host's network namespace instead of the container's own`},
		}},
		// Docker rejects a field list holding a field it cannot read, so nothing runs; the list still
		// says which network was asked for, and the rule says so rather than going quiet on it.
		{"network field list with a field carrying no value", `{"runArgs": ["--network=name=host,web"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=name=host,web", which asks for the host's network namespace instead of the container's own`},
		}},
		{"network field list with an unknown field key", `{"runArgs": ["--network=name=host,foo=bar"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=name=host,foo=bar", which asks for the host's network namespace instead of the container's own`},
		}},
		{"network field list naming no network", `{"runArgs": ["--network=alias=host"]}`, nil},
		// A "runArgs" that is an object is no argv: its members are traversed as an object's, so one
		// named like a flag reaches this rule's path carrying no flag occurrence.
		{"runArgs is an object keyed like a flag", `{"runArgs": {"--network": "host"}}`, nil},
		{"runArgs is an object with a non-string member", `{"runArgs": {"--pid": {"a": 1}}}`, nil},
		// Only "--network" takes a field list; every other namespace flag names its value directly,
		// so Docker reads this one as a network named "name=host" rather than as the host.
		{"a field list is not accepted for another namespace", `{"runArgs": ["--pid=name=host"]}`, nil},
		{"a host value is matched case-sensitively", `{"runArgs": ["--pid=Host"]}`, nil},
		{"every offending entry is reported", `{"runArgs": ["--network=host", "--pid=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=host", which asks for the host's network namespace instead of the container's own`},
			{Path: "devcontainer.json", Line: 1, Col: 32, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--pid=host", which asks for the host's process namespace instead of the container's own`},
		}},
		// "runArgs" holds strings, so an entry that is not one is undefined rather than specified
		// to end the parse; reading on keeps a flag behind it from going unreported.
		{"non-string entry before the flag", `{"runArgs": [123, "--pid=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 19, RuleID: "no-host-namespace", Message: `"runArgs" contains "--pid=host", which asks for the host's process namespace instead of the container's own`},
		}},
		{"runArgs is not an array", `{"runArgs": "--pid=host"}`, nil},
		// A bare flag reads as a boolean set to true, which is not a namespace, so only a flag with
		// a value can name one.
		{"a flag with no value left to take", `{"runArgs": ["--pid"]}`, nil},
		{"a flag whose value is not a string", `{"runArgs": ["--pid", 123, "--pid=host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 28, RuleID: "no-host-namespace", Message: `"runArgs" contains "--pid=host", which asks for the host's process namespace instead of the container's own`},
		}},
		// Which repeat of one of these options Docker keeps differs per option: "--network" keeps
		// the first value it is given and the rest keep the last. Either way the superseded entry is
		// dead only where it stands, so it is reported too.
		{"network repeated, the host first", `{"runArgs": ["--network", "host", "--net", "devnet"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 27, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--network=host", which asks for the host's network namespace instead of the container's own`},
		}},
		{"network repeated, the host second", `{"runArgs": ["--network", "devnet", "--net", "host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 46, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--net=host", which asks for the host's network namespace instead of the container's own`},
		}},
		{"pid repeated, the host last", `{"runArgs": ["--pid", "container:x", "--pid", "host"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 47, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--pid=host", which asks for the host's process namespace instead of the container's own`},
		}},
		{"pid repeated, the host first", `{"runArgs": ["--pid", "host", "--pid", "container:x"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 23, RuleID: "no-host-namespace",
				Message: `"runArgs" contains "--pid=host", which asks for the host's process namespace instead of the container's own`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoHostNamespace, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}
