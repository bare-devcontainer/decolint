package rules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoHostPortFormat reports "forwardPorts" entries and "portsAttributes" keys written in
// "host:port" format. The Dev Container spec allows that format, but GitHub Codespaces only
// supports a bare port number in either property.
var NoHostPortFormat = &linter.Rule{
	ID:          "no-host-port-format",
	Description: `disallow "host:port" entries in "forwardPorts" and "portsAttributes", which GitHub Codespaces does not support`,
	LongDescription: `The "host:port" form forwards a port from another container in a Docker Compose project (e.g. "db:5432")
rather than from the primary one. Codespaces documents that it does not support that variation of either
property, so the entry is ignored there and the port is not forwarded. A bare port number, which refers
to the primary container, works everywhere.`,
	References: []string{
		"https://github.com/devcontainers/spec/blob/main/docs/specs/supporting-tools.md#github-codespaces",
		"https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties",
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Platforms: []linter.Platform{linter.PlatformCodespaces},
	Paths:     []string{"/forwardPorts/*", "/portsAttributes/*"},
	Check:     checkNoHostPortFormat,
}

func checkNoHostPortFormat(_ *linter.Context, node *linter.Node) []linter.Finding {
	if key, ok := strings.CutPrefix(node.Pointer, "/portsAttributes/"); ok {
		if !isHostPort(key) {
			return nil
		}
		return []linter.Finding{{
			Message: fmt.Sprintf(`"portsAttributes" key %q uses "host:port" format; Codespaces only supports a bare port number`, key),
			Offset:  node.Value.StartOffset,
		}}
	}

	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	port := lit.String()
	if !isHostPort(port) {
		return nil
	}
	return []linter.Finding{{
		Message: fmt.Sprintf(`"forwardPorts" entry %q uses "host:port" format; Codespaces only supports a bare port number`, port),
		Offset:  node.Value.StartOffset,
	}}
}

// isHostPort reports whether s is a "host:port" pair: a non-empty host, a colon, and a port that is
// entirely digits.
func isHostPort(s string) bool {
	i := strings.LastIndex(s, ":")
	if i <= 0 || i == len(s)-1 {
		return false
	}
	if _, err := strconv.Atoi(s[i+1:]); err != nil {
		return false
	}
	return true
}
