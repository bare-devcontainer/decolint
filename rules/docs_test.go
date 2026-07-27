package rules

import (
	"strings"
	"testing"
)

// The published addresses are spelled out here rather than built from docsBaseURL: a test that
// derives them the same way the code does would agree with any change, including one that moves
// every rule page out from under the address findings already point readers at.

func TestDocsURL(t *testing.T) {
	t.Parallel()

	const want = "https://bare-devcontainer.github.io/decolint/rules/no-image-latest/"
	if got := DocsURL("no-image-latest"); got != want {
		t.Errorf("DocsURL(%q) = %q, want %q", "no-image-latest", got, want)
	}
}

func TestDocsCategoryURL(t *testing.T) {
	t.Parallel()

	const want = "https://bare-devcontainer.github.io/decolint/rules/#security"
	if got := DocsCategoryURL("security"); got != want {
		t.Errorf("DocsCategoryURL(%q) = %q, want %q", "security", got, want)
	}
}

// TestDocsURL_EveryBuiltinRule checks that every rule ID reaches its page as a path segment of its
// own. cmd/docgen names each generated page after the ID, so an ID carrying a separator or a
// fragment would publish somewhere the address findings carry does not resolve to.
func TestDocsURL_EveryBuiltinRule(t *testing.T) {
	t.Parallel()

	for _, reg := range Builtin() {
		id := reg.Rule.ID
		if got := DocsURL(id); strings.Count(got, "/") != strings.Count(docsBaseURL, "/")+1 || strings.ContainsAny(id, "#?") {
			t.Errorf("DocsURL(%q) = %q, want the ID as a single path segment", id, got)
		}
	}
}
