package rules_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/rules"
)

// Every built-in rule documents itself: a short description of what it checks, a rationale, and the
// references that justify it. These tests are what keeps a new rule from landing undocumented.

func TestBuiltin_Descriptions(t *testing.T) {
	t.Parallel()

	for _, reg := range rules.Builtin() {
		if strings.TrimSpace(reg.Rule.Description) == "" {
			t.Errorf("rule %s has no Description", reg.Rule.ID)
		}
		if strings.TrimSpace(reg.Rule.LongDescription) == "" {
			t.Errorf("rule %s has no LongDescription", reg.Rule.ID)
		}
		if got := reg.Rule.LongDescription; got != strings.TrimSpace(got) {
			t.Errorf("rule %s LongDescription has leading or trailing whitespace", reg.Rule.ID)
		}
	}
}

func TestBuiltin_References(t *testing.T) {
	t.Parallel()

	for _, reg := range rules.Builtin() {
		if len(reg.Rule.References) == 0 {
			t.Errorf("rule %s has no References", reg.Rule.ID)
		}
		for _, ref := range reg.Rule.References {
			// A reference is rendered as a link wherever it is shown, so it has to be a URL a reader
			// can follow on its own, not a bare path or a prose citation.
			u, err := url.Parse(ref)
			if err != nil {
				t.Errorf("rule %s reference %q does not parse: %v", reg.Rule.ID, ref, err)
				continue
			}
			if u.Scheme != "https" || u.Host == "" {
				t.Errorf("rule %s reference %q is not an absolute https URL", reg.Rule.ID, ref)
			}
		}
		if i := duplicateIndex(reg.Rule.References); i >= 0 {
			t.Errorf("rule %s lists reference %q twice", reg.Rule.ID, reg.Rule.References[i])
		}
	}
}

// duplicateIndex returns the index of the first element of refs that appears earlier in it, or -1
// if every element is unique.
func duplicateIndex(refs []string) int {
	for i, ref := range refs {
		if slices.Contains(refs[:i], ref) {
			return i
		}
	}
	return -1
}
