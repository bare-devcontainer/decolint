package main

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

// writeRulePages renders one Markdown page per built-in rule into dir/rules, named after the rule's
// ID. It is the site's only source for the rule reference: nothing under docs/content/rules other
// than _index.md is committed, so a rule's documentation cannot drift from rules/*.go.
func writeRulePages(dir string) error {
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", rulesDir, err)
	}
	for _, reg := range rules.Builtin() {
		page := renderRulePage(reg.Rule)
		path := filepath.Join(rulesDir, reg.Rule.ID+".md")
		if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// renderRulePage renders one rule's documentation page: front matter matching what
// docs/layouts/page.html reads, the rationale, a Bad and a Good example, and its references.
func renderRulePage(r *linter.Rule) string {
	var b strings.Builder

	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "title: %s\n", r.ID)
	fmt.Fprintf(&b, "category: %s\n", r.Category)
	fmt.Fprintf(&b, "platforms: [%s]\n", strings.Join(platformNames(r.Platforms), ", "))
	fmt.Fprintf(&b, "file_types: [%s]\n", strings.Join(fileTypeNames(r.FileTypes), ", "))
	fmt.Fprintf(&b, "description: %s\n", yamlSingleQuoted(r.Description))
	fmt.Fprintf(&b, "---\n\n")

	fmt.Fprintf(&b, "## Why\n\n%s\n\n", r.LongDescription)

	fmt.Fprintf(&b, "## Bad\n\n%s\n", strings.TrimRight(renderSnippet(r.Example.Bad), "\n"))
	fmt.Fprintf(&b, "\n## Good\n\n%s\n", strings.TrimRight(renderSnippet(r.Example.Good), "\n"))
	if r.Example.Note != "" {
		fmt.Fprintf(&b, "\n%s\n", r.Example.Note)
	}

	fmt.Fprintf(&b, "\n## References\n\n")
	for _, ref := range r.References {
		fmt.Fprintf(&b, "- <%s>\n", ref)
	}

	return b.String()
}

// renderSnippet renders a [linter.Snippet] as one fenced code block per file, each preceded by a
// "### `path`" heading when the snippet has more than one file, or when a non-default Mode is the
// only thing distinguishing the file from its counterpart in the other half of the example (e.g. an
// install.sh whose content is identical in Bad and Good).
func renderSnippet(s linter.Snippet) string {
	var b strings.Builder
	for _, f := range s.Files {
		switch {
		case f.Mode != 0:
			fmt.Fprintf(&b, "### `%s` (mode %04o)\n\n", f.Path, f.Mode)
		case len(s.Files) > 1:
			fmt.Fprintf(&b, "### `%s`\n\n", f.Path)
		}
		fmt.Fprintf(&b, "```%s\n%s```\n\n", codeLang(f.Path), f.Content)
	}
	return b.String()
}

// codeLang returns the fenced-block language for a file named path, guessed from its extension.
func codeLang(path string) string {
	switch {
	case strings.HasSuffix(path, ".json"):
		// The examples use // comments for context, so they need JSONC's lexer, not plain JSON's.
		return "jsonc"
	case strings.HasSuffix(path, ".sh"):
		return "bash"
	default:
		return "text"
	}
}

func platformNames(platforms []linter.Platform) []string {
	names := make([]string, len(platforms))
	for i, p := range platforms {
		names[i] = p.String()
	}
	return names
}

func fileTypeNames(fileTypes []linter.FileType) []string {
	names := make([]string, len(fileTypes))
	for i, ft := range fileTypes {
		names[i] = string(ft)
	}
	return names
}

// yamlSingleQuoted renders s as a single-quoted YAML scalar: the one quoting style that needs no
// escaping for the double quotes a rule's Description is full of ("image", "build", ...); a literal
// single quote doubles, per YAML's own rule for this style.
func yamlSingleQuoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// rulesTableMarkers delimit the generated rules table in README.md, so updateReadmeRulesTable can
// find and replace exactly that table and nothing else.
const (
	rulesTableStart = "<!-- decolint:rules-table -->"
	rulesTableEnd   = "<!-- /decolint:rules-table -->"
)

// updateReadmeRulesTable rewrites the table between the rulesTableStart/rulesTableEnd markers in
// the README at path to the current built-in rules, sorted by category (in the same order the
// category list above the table uses) then ID. It is an error if the markers are missing.
func updateReadmeRulesTable(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	start := strings.Index(string(src), rulesTableStart)
	end := strings.Index(string(src), rulesTableEnd)
	if start < 0 || end < 0 || end < start {
		return fmt.Errorf("%s: rules table markers %q/%q not found", path, rulesTableStart, rulesTableEnd)
	}
	end += len(rulesTableEnd)

	table := rulesTableStart + "\n" + renderRulesTable() + rulesTableEnd
	out := string(src[:start]) + table + string(src[end:])
	if out == string(src) {
		return nil
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

// renderRulesTable renders every built-in rule as a Markdown table row, each ID linking to its
// documentation page, sorted by category then ID.
func renderRulesTable() string {
	regs := slices.Clone(rules.Builtin())
	slices.SortFunc(regs, func(a, b rules.Registration) int {
		if c := cmp.Compare(a.Rule.Category, b.Rule.Category); c != 0 {
			return c
		}
		return cmp.Compare(a.Rule.ID, b.Rule.ID)
	})

	var b strings.Builder
	b.WriteString("| ID | Category | Platform | Description |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, reg := range regs {
		r := reg.Rule
		platform := "(all)"
		if names := platformNames(r.Platforms); len(names) > 0 {
			quoted := make([]string, len(names))
			for i, n := range names {
				quoted[i] = "`" + n + "`"
			}
			platform = strings.Join(quoted, ", ")
		}
		fmt.Fprintf(&b, "| [`%s`](%s) | `%s` | %s | %s |\n", r.ID, rules.DocsURL(r.ID), r.Category, platform, r.Description)
	}
	return b.String()
}
