package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata")

// TestMustRender_UnknownTemplate checks that a name no template in templates/ defines panics,
// rather than quietly rendering an empty document that would be published as a blank page.
func TestMustRender_UnknownTemplate(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("mustRender with an unknown template name: did not panic, want a panic")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "no-such.md.tmpl") {
			t.Errorf("mustRender panicked with %v, want a message naming the template", r)
		}
	}()

	mustRender("no-such.md.tmpl", nil)
}

// TestRenderRulePage_Golden pins a whole rule page — headings, blank lines, fence placement — which
// the field-by-field assertions in rules_test.go cannot see. Rerun with "-update" after an intended
// change to a template and review the resulting diff.
func TestRenderRulePage_Golden(t *testing.T) {
	t.Parallel()

	r := &linter.Rule{
		ID:              "golden-rule",
		Description:     `disallow "privileged"`,
		LongDescription: "Why it matters.",
		References:      []string{"https://example.invalid/a", "https://example.invalid/b"},
		Category:        linter.CategorySecurity,
		FileTypes:       []linter.FileType{linter.Feature},
		Platforms:       []linter.Platform{linter.PlatformCodespaces},
		Example: linter.Example{
			Bad: linter.Snippet{
				Files: []linter.ExampleFile{
					{Path: "devcontainer-feature.json", Content: "{\n  \"id\": \"node\"\n}\n"},
					{Path: "install.sh", Content: "#!/bin/sh\n", Mode: 0o644},
				},
			},
			Good: linter.Snippet{
				Files: []linter.ExampleFile{{Path: "devcontainer-feature.json", Content: "{}\n"}},
			},
			Note: "A closing note.",
		},
	}

	got := renderRulePage(r)

	golden := filepath.Join("testdata", "rule-page.golden.md")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("update %s: %v", golden, err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read %s: %v", golden, err)
	}
	if got != string(want) {
		t.Errorf("renderRulePage() does not match %s:\ngot:\n%s\nwant:\n%s", golden, got, want)
	}
}
