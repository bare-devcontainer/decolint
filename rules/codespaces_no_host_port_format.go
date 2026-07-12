package rules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// CodespacesNoHostPortFormat reports "forwardPorts" entries and "portsAttributes" keys written in
// "host:port" format. The Dev Container spec allows that format, but GitHub Codespaces only
// supports a bare port number in either property.
type CodespacesNoHostPortFormat struct{}

// ID implements [linter.Rule].
func (CodespacesNoHostPortFormat) ID() string { return "codespaces-no-host-port-format" }

// Description implements [linter.Rule].
func (CodespacesNoHostPortFormat) Description() string {
	return `disallow "host:port" entries in "forwardPorts" and "portsAttributes", which GitHub Codespaces does not support`
}

// FileTypes implements [linter.Rule].
func (CodespacesNoHostPortFormat) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (CodespacesNoHostPortFormat) Platforms() []linter.Platform {
	return []linter.Platform{linter.PlatformCodespaces}
}

// Paths implements [linter.Rule].
func (CodespacesNoHostPortFormat) Paths() []string {
	return []string{"/forwardPorts/*", "/portsAttributes/*"}
}

// Check implements [linter.Rule].
func (CodespacesNoHostPortFormat) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
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
