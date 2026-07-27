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

// rulePageData is what templates/rule.md.tmpl renders: one rule, with every list already formatted
// as the string the page shows.
type rulePageData struct {
	ID              string
	Category        string
	Platforms       string
	FileTypes       string
	Description     string
	LongDescription string
	Bad             snippetData
	Good            snippetData
	Note            string
	References      []string
}

// renderRulePage renders one rule's documentation page: front matter matching what
// docs/layouts/page.html reads, the rationale, a Bad and a Good example, and its references.
func renderRulePage(r *linter.Rule) string {
	data := rulePageData{
		ID:              r.ID,
		Category:        r.Category.String(),
		Platforms:       strings.Join(platformNames(r.Platforms), ", "),
		FileTypes:       strings.Join(fileTypeNames(r.FileTypes), ", "),
		Description:     r.Description,
		LongDescription: r.LongDescription,
		Bad:             snippet(r.Example.Bad),
		Good:            snippet(r.Example.Good),
		Note:            r.Example.Note,
		References:      r.References,
	}
	return mustRender("rule.md.tmpl", data)
}

// snippetData is what the "snippet" template renders: one example's files, each a fenced code block
// optionally introduced by a heading.
type snippetData struct {
	Files []snippetFile
}

// snippetFile is one file of an example: the text of its "### " heading (empty for none), the
// fenced block's language, and its content, guaranteed to end in a newline.
type snippetFile struct {
	Heading string
	Lang    string
	Content string
}

// snippet builds the render data for a [linter.Snippet]. A file gets a heading when the snippet has
// more than one file, or when a non-default Mode is the only thing distinguishing the file from its
// counterpart in the other half of the example (e.g. an install.sh whose content is identical in Bad
// and Good).
func snippet(s linter.Snippet) snippetData {
	files := make([]snippetFile, len(s.Files))
	for i, f := range s.Files {
		heading := ""
		switch {
		case f.Mode != 0:
			heading = fmt.Sprintf("`%s` (mode %04o)", f.Path, f.Mode.Perm())
		case len(s.Files) > 1:
			heading = fmt.Sprintf("`%s`", f.Path)
		}
		content := f.Content
		if !strings.HasSuffix(content, "\n") {
			// The closing fence must start its own line; without this, content missing a trailing
			// newline would run straight into it, leaving the fence unclosed and swallowing the
			// rest of the page.
			content += "\n"
		}
		files[i] = snippetFile{Heading: heading, Lang: codeLang(f.Path), Content: content}
	}
	return snippetData{Files: files}
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
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// tableRow is one row of the README rules table, as templates/rules-table.md.tmpl renders it.
type tableRow struct {
	ID          string
	URL         string
	Category    string
	Platform    string
	Description string
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

	rows := make([]tableRow, len(regs))
	for i, reg := range regs {
		r := reg.Rule
		platform := "(all)"
		if names := platformNames(r.Platforms); len(names) > 0 {
			quoted := make([]string, len(names))
			for j, n := range names {
				quoted[j] = "`" + n + "`"
			}
			platform = strings.Join(quoted, ", ")
		}
		rows[i] = tableRow{
			ID:          r.ID,
			URL:         rules.DocsURL(r.ID),
			Category:    r.Category.String(),
			Platform:    platform,
			Description: r.Description,
		}
	}
	return mustRender("rules-table.md.tmpl", rows)
}
