package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

// TestNoDangerousCapAdd_EveryCapability pins the rule's inventory: every capability it reports,
// and the effect each one is reported with, which is user-facing text. The list is written out here
// rather than read from the rule, so that dropping an entry or rewording an effect fails rather
// than quietly reporting one capability fewer. TestNoDangerousCapAdd covers the forms a capability
// may be written in; this covers which capabilities there are.
func TestNoDangerousCapAdd_EveryCapability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		capability, effect string
	}{
		{"AUDIT_CONTROL", "allows reconfiguring the kernel's audit subsystem, which is not namespaced"},
		{"BPF", "allows loading BPF programs into the host kernel"},
		{"DAC_READ_SEARCH", "bypasses file read permission checks and allows opening files by handle, outside the container's filesystem"},
		{"MAC_ADMIN", "allows changing the host's mandatory access control policy"},
		{"MAC_OVERRIDE", "bypasses the host's mandatory access control policy"},
		{"PERFMON", "grants access to the kernel's performance monitoring interfaces, which observe the whole host"},
		{"SYSLOG", "allows reading the host kernel's log, which discloses kernel addresses"},
		{"SYS_ADMIN", "grants a broad range of administrative operations, including mounting filesystems"},
		{"SYS_BOOT", "allows rebooting the host"},
		{"SYS_MODULE", "allows loading modules into the host kernel"},
		{"SYS_RAWIO", "allows raw access to the host's I/O ports and memory devices"},
		{"SYS_TIME", "allows setting the clock, which the container shares with the host"},
	}
	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			t.Parallel()
			src := `{"capAdd": ["` + tt.capability + `"]}`
			assertIssues(t, rules.NoDangerousCapAdd, linter.SeverityWarn, src, []linter.Issue{
				{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-dangerous-cap-add",
					Message: `"capAdd" contains "` + tt.capability + `", which ` + tt.effect},
			})
		})
	}
}

func TestNoDangerousCapAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no capAdd property", `{"name": "test"}`, nil},
		// SYS_PTRACE and NET_ADMIN are confined to the container's own process and network
		// namespaces, so this rule leaves them to no-host-namespace.
		{"capAdd with SYS_PTRACE is allowed", `{"capAdd": ["SYS_PTRACE"]}`, nil},
		{"capAdd with NET_ADMIN is allowed", `{"capAdd": ["NET_ADMIN"]}`, nil},
		{"capAdd with ALL is left to no-cap-add-all", `{"capAdd": ["ALL"]}`, nil},
		{"capAdd with SYS_ADMIN", `{"capAdd": ["SYS_ADMIN"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-dangerous-cap-add",
				Message: `"capAdd" contains "SYS_ADMIN", which grants a broad range of administrative operations, including mounting filesystems`},
		}},
		{"capAdd with the CAP_ prefix in lower case", `{"capAdd": ["cap_sys_module"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-dangerous-cap-add",
				Message: `"capAdd" contains "cap_sys_module", which allows loading modules into the host kernel`},
		}},
		{"every offending entry is reported", `{"capAdd": ["SYS_PTRACE", "SYS_RAWIO", "SYSLOG"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 27, RuleID: "no-dangerous-cap-add",
				Message: `"capAdd" contains "SYS_RAWIO", which allows raw access to the host's I/O ports and memory devices`},
			{Path: "devcontainer.json", Line: 1, Col: 40, RuleID: "no-dangerous-cap-add",
				Message: `"capAdd" contains "SYSLOG", which allows reading the host kernel's log, which discloses kernel addresses`},
		}},
		{"no runArgs", `{"runArgs": ["--init"]}`, nil},
		{"runArgs with cap-add=SYS_PTRACE", `{"runArgs": ["--cap-add=SYS_PTRACE"]}`, nil},
		{"runArgs with cap-drop is not cap-add", `{"runArgs": ["--cap-drop=SYS_ADMIN"]}`, nil},
		{"runArgs with cap-add=SYS_ADMIN", `{"runArgs": ["--cap-add=SYS_ADMIN"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 14, RuleID: "no-dangerous-cap-add",
				Message: `"runArgs" contains "--cap-add=SYS_ADMIN", which grants a broad range of administrative operations, including mounting filesystems`},
		}},
		{"runArgs with cap-add DAC_READ_SEARCH two tokens", `{"runArgs": ["--cap-add", "DAC_READ_SEARCH"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 27, RuleID: "no-dangerous-cap-add",
				Message: `"runArgs" contains "--cap-add=DAC_READ_SEARCH", which bypasses file read permission checks and allows opening files by handle, outside the container's filesystem`},
		}},
		{"capAdd entry is not a string", `{"capAdd": [123]}`, nil},
		{"runArgs is not an array", `{"runArgs": "--cap-add=SYS_ADMIN"}`, nil},
		// "-e" and "--name" take a value, so the entry after each is that value and names no flag
		// (see dockerargs.Flags). In the second case "SYS_ADMIN" is left as the image name, so no
		// capability is added there either.
		{"runArgs with cap-add in another flag's value position", `{"runArgs": ["-e", "--cap-add=SYS_ADMIN"]}`, nil},
		{"runArgs with cap-add after another flag", `{"runArgs": ["--name", "--cap-add", "SYS_ADMIN"]}`, nil},
		// A bare entry would be the image name to "docker run", which parses no flag after it — but
		// it also displaces the image the devcontainer CLI appends, along with the flags it derives
		// from the configuration's own security properties. Reading on reports what is written
		// rather than going quiet on a file that is broken.
		{"runArgs with cap-add after the image name", `{"runArgs": ["myimage", "--cap-add=SYS_ADMIN"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 25, RuleID: "no-dangerous-cap-add",
				Message: `"runArgs" contains "--cap-add=SYS_ADMIN", which grants a broad range of administrative operations, including mounting filesystems`},
		}},
		{"runArgs with cap-add after a terminator", `{"runArgs": ["--", "--cap-add=SYS_ADMIN"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 20, RuleID: "no-dangerous-cap-add", Message: `"runArgs" contains "--cap-add=SYS_ADMIN", which grants a broad range of administrative operations, including mounting filesystems`},
		}},
		{"both capAdd and runArgs", `{"capAdd": ["SYS_BOOT"], "runArgs": ["--cap-add=SYS_TIME"]}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 13, RuleID: "no-dangerous-cap-add",
				Message: `"capAdd" contains "SYS_BOOT", which allows rebooting the host`},
			{Path: "devcontainer.json", Line: 1, Col: 38, RuleID: "no-dangerous-cap-add",
				Message: `"runArgs" contains "--cap-add=SYS_TIME", which allows setting the clock, which the container shares with the host`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoDangerousCapAdd, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}

func TestNoDangerousCapAdd_Feature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no capAdd property", `{"id": "test", "version": "1.0.0", "name": "test"}`, nil},
		{"capAdd with SYS_PTRACE is allowed", `{"id": "test", "capAdd": ["SYS_PTRACE"]}`, nil},
		{"capAdd with SYS_MODULE", `{"id": "test", "capAdd": ["SYS_MODULE"]}`, []linter.Issue{
			{Path: "devcontainer-feature.json", Line: 1, Col: 27, RuleID: "no-dangerous-cap-add",
				Message: `"capAdd" contains "SYS_MODULE", which allows loading modules into the host kernel`},
		}},
		// "runArgs" has no meaning in a Feature, so it's not flagged there.
		{"runArgs with cap-add=SYS_ADMIN is ignored", `{"id": "test", "runArgs": ["--cap-add=SYS_ADMIN"]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.NoDangerousCapAdd, linter.SeverityWarn, "devcontainer-feature.json", linter.Feature, tt.src, tt.want)
		})
	}
}
