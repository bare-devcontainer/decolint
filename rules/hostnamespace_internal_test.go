package rules

import (
	"testing"

	"github.com/bare-devcontainer/decolint/dockerargs"
)

// TestHostNamespaces_KeysAreRunFlags guards the lookup in checkNoHostNamespace, which is by the
// canonical long name the engine hands a rule. A key naming no "docker run" flag would leave its
// entry unreachable, and the rule reading a zero hostNamespace for every occurrence.
func TestHostNamespaces_KeysAreRunFlags(t *testing.T) {
	t.Parallel()

	known := map[string]bool{}
	for _, f := range dockerargs.RunFlags {
		known[f.Name] = true
	}
	for flag := range hostNamespaces {
		if !known[flag] {
			t.Errorf("hostNamespaces is keyed by %q, which names no \"docker run\" flag", flag)
		}
	}
}
