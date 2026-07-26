package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestYamlSingleQuoted(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{`disallow "privileged"`, `'disallow "privileged"'`},
		{`it's fine`, `'it''s fine'`},
		{"plain", "'plain'"},
	}
	for _, tt := range tests {
		if got := yamlSingleQuoted(tt.in); got != tt.want {
			t.Errorf("yamlSingleQuoted(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCodeLang(t *testing.T) {
	t.Parallel()

	tests := []struct{ path, want string }{
		{"devcontainer.json", "jsonc"},
		{"devcontainer-feature.json", "jsonc"},
		{"install.sh", "bash"},
		{"README.md", "text"},
	}
	for _, tt := range tests {
		if got := codeLang(tt.path); got != tt.want {
			t.Errorf("codeLang(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestRenderRulePage(t *testing.T) {
	t.Parallel()

	r := &linter.Rule{
		ID:              "my-rule",
		Description:     `disallow "foo"`,
		LongDescription: "Why it matters.",
		References:      []string{"https://example.invalid/a", "https://example.invalid/b"},
		Category:        linter.CategorySecurity,
		FileTypes:       []linter.FileType{linter.Devcontainer},
		Platforms:       []linter.Platform{linter.PlatformCodespaces},
		Example: linter.Example{
			Bad: linter.Snippet{
				Files: []linter.ExampleFile{{Path: "devcontainer.json", Content: "{\n  \"privileged\": true\n}\n"}},
			},
			Good: linter.Snippet{
				Files: []linter.ExampleFile{{Path: "devcontainer.json", Content: "{}\n"}},
			},
			Note: "A closing note.",
		},
	}

	page := renderRulePage(r)

	for _, want := range []string{
		"title: my-rule",
		"category: security",
		"platforms: [codespaces]",
		"file_types: [devcontainer]",
		`description: 'disallow "foo"'`,
		"## Why\n\nWhy it matters.",
		"## Bad",
		`"privileged": true`,
		"## Good",
		"A closing note.",
		"## References",
		"- <https://example.invalid/a>",
		"- <https://example.invalid/b>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("renderRulePage() missing %q, got:\n%s", want, page)
		}
	}
}

func TestRenderRulePage_MultiFileExample(t *testing.T) {
	t.Parallel()

	r := &linter.Rule{
		ID:          "multi-file-rule",
		Description: "d",
		Category:    linter.CategoryCorrectness,
		FileTypes:   []linter.FileType{linter.Template},
		Example: linter.Example{
			Bad: linter.Snippet{
				Files: []linter.ExampleFile{
					{Path: "devcontainer-template.json", Content: "{}\n"},
					{Path: ".devcontainer/devcontainer.json", Content: "{}\n"},
				},
			},
			Good: linter.Snippet{
				Files: []linter.ExampleFile{
					{Path: "devcontainer-template.json", Content: "{}\n"},
					{Path: ".devcontainer/devcontainer.json", Content: "{}\n"},
				},
			},
		},
	}

	page := renderRulePage(r)
	for _, want := range []string{
		"### `devcontainer-template.json`",
		"### `.devcontainer/devcontainer.json`",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("renderRulePage() missing %q for a multi-file example, got:\n%s", want, page)
		}
	}
}

func TestRenderRulePage_ModeCaption(t *testing.T) {
	t.Parallel()

	r := &linter.Rule{
		ID:          "mode-rule",
		Description: "d",
		Category:    linter.CategoryCorrectness,
		FileTypes:   []linter.FileType{linter.Feature},
		Example: linter.Example{
			Bad: linter.Snippet{
				Files: []linter.ExampleFile{{Path: "install.sh", Content: "#!/bin/sh\n", Mode: 0o644}},
			},
			Good: linter.Snippet{
				Files: []linter.ExampleFile{{Path: "install.sh", Content: "#!/bin/sh\n", Mode: 0o755}},
			},
		},
	}

	page := renderRulePage(r)
	if !strings.Contains(page, "(mode 0644)") {
		t.Errorf("renderRulePage() missing the Bad file's mode caption, got:\n%s", page)
	}
	if !strings.Contains(page, "(mode 0755)") {
		t.Errorf("renderRulePage() missing the Good file's mode caption, got:\n%s", page)
	}
}

func TestWriteRulePages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := writeRulePages(dir); err != nil {
		t.Fatalf("writeRulePages: %v", err)
	}

	builtin := rules.Builtin()
	entries, err := os.ReadDir(filepath.Join(dir, "rules"))
	if err != nil {
		t.Fatalf("read rules dir: %v", err)
	}
	if len(entries) != len(builtin) {
		t.Errorf("writeRulePages wrote %d file(s), want %d (one per built-in rule)", len(entries), len(builtin))
	}

	path := filepath.Join(dir, "rules", "no-image-latest.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), "title: no-image-latest") {
		t.Errorf("%s missing expected title, got:\n%s", path, data)
	}
}

func TestRenderRulesTable(t *testing.T) {
	t.Parallel()

	table := renderRulesTable()
	if !strings.HasPrefix(table, "| ID | Category | Platform | Description |\n") {
		t.Fatalf("renderRulesTable() does not start with the header row, got:\n%s", table)
	}

	rows := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if want := len(rules.Builtin()) + 2; len(rows) != want { // +2 for the header and separator rows
		t.Errorf("renderRulesTable() produced %d row(s), want %d", len(rows), want)
	}

	// Sorted by category (Correctness < Security < ... per the linter.Category iota order), then ID:
	// the first data row must be a correctness rule, and the table must link to rules.DocsURL.
	if !strings.Contains(rows[2], "`correctness`") {
		t.Errorf("first data row is not a correctness rule, got: %s", rows[2])
	}
	if !strings.Contains(table, rules.DocsURL("no-image-latest")) {
		t.Errorf("renderRulesTable() does not link to rules.DocsURL, got:\n%s", table)
	}
}

func TestUpdateReadmeRulesTable(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "README.md")
	original := "prose\n\n" + rulesTableStart + "\nstale\n" + rulesTableEnd + "\n\nmore prose\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := updateReadmeRulesTable(path); err != nil {
		t.Fatalf("updateReadmeRulesTable: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if strings.Contains(string(got), "stale") {
		t.Errorf("updateReadmeRulesTable did not replace the stale table, got:\n%s", got)
	}
	if !strings.HasPrefix(string(got), "prose\n\n"+rulesTableStart) || !strings.HasSuffix(string(got), "\n\nmore prose\n") {
		t.Errorf("updateReadmeRulesTable disturbed content outside the markers, got:\n%s", got)
	}

	// Running it again on its own output must be a no-op (the generator has to be idempotent, since
	// CI fails the build if it finds anything left to regenerate).
	if err := updateReadmeRulesTable(path); err != nil {
		t.Fatalf("updateReadmeRulesTable (second run): %v", err)
	}
	got2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got2) != string(got) {
		t.Errorf("updateReadmeRulesTable is not idempotent:\nfirst:\n%s\nsecond:\n%s", got, got2)
	}
}

func TestUpdateReadmeRulesTable_MissingMarkers(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("no markers here\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := updateReadmeRulesTable(path); err == nil {
		t.Fatal("updateReadmeRulesTable with no markers: got nil error, want one")
	}
}
