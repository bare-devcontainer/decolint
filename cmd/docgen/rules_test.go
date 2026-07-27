package main

import (
	"io/fs"
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

// TestSnippet_ModeCaptionIsPermissionBitsOnly checks that the "(mode ####)" caption reports only
// the POSIX permission bits, not the type bits fs.FileMode also carries (e.g. fs.ModeDir), which
// would otherwise inflate the printed octal value beyond four digits.
func TestSnippet_ModeCaptionIsPermissionBitsOnly(t *testing.T) {
	t.Parallel()

	got := snippet(linter.Snippet{
		Files: []linter.ExampleFile{{Path: "some-dir", Content: "x\n", Mode: fs.ModeDir | 0o755}},
	})
	if want := "`some-dir` (mode 0755)"; got.Files[0].Heading != want {
		t.Errorf("snippet() heading = %q, want %q", got.Files[0].Heading, want)
	}
}

// TestSnippet_ContentWithoutTrailingNewline guards against an unclosed fence: the closing "```" has
// to start its own line (a markdown renderer does not recognize one run straight into the preceding
// content as a close, and treats everything after as still inside the code block), so content
// missing a trailing newline needs one added before it.
func TestSnippet_ContentWithoutTrailingNewline(t *testing.T) {
	t.Parallel()

	data := snippet(linter.Snippet{
		Files: []linter.ExampleFile{{Path: "devcontainer.json", Content: "{}"}},
	})
	got := mustRender("snippet", data)
	want := "```jsonc\n{}\n```"
	if got != want {
		t.Errorf("mustRender(\"snippet\") = %q, want %q", got, want)
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

// TestWriteRulePages_UnwritableDir checks that a rules directory that cannot be created is
// reported: the site would otherwise be published with its whole rule reference silently missing.
func TestWriteRulePages_UnwritableDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// A plain file where the rules directory has to go: MkdirAll fails on it whatever the process
	// runs as, unlike a permission bit root ignores.
	if err := os.WriteFile(filepath.Join(dir, "rules"), nil, 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	if err := writeRulePages(dir); err == nil {
		t.Error("writeRulePages into a dir whose rules path is a file: got nil error, want one")
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

func TestUpdateReadmeRulesTable_MissingFile(t *testing.T) {
	t.Parallel()

	if err := updateReadmeRulesTable(filepath.Join(t.TempDir(), "absent.md")); err == nil {
		t.Fatal("updateReadmeRulesTable on a missing file: got nil error, want one")
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
