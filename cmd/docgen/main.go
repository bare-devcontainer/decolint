// Command docgen generates the documentation site's content and the README's rules table from
// rules/*.go and README.md, so neither has to be hand-kept in sync with the other. It is not part of
// the decolint binary; "make docs" runs it before Hugo builds the site (see the Makefile).
package main

import (
	"fmt"
	"os"
)

// generatedContentDir is where the generated site pages are written. It is mounted alongside
// docs/content in hugo.toml and is never committed (see .gitignore); docs/content/rules/_index.md
// is the only rules page that stays hand-written.
const generatedContentDir = "docs/.generated/content"

func main() {
	if err := run("README.md", generatedContentDir); err != nil {
		fmt.Fprintln(os.Stderr, "docgen:", err)
		os.Exit(1)
	}
}

// run regenerates everything docgen owns: the rule pages and the README-derived pages into
// contentDir, and the rules table in the README at readmePath, in place.
func run(readmePath, contentDir string) error {
	if err := os.RemoveAll(contentDir); err != nil {
		return fmt.Errorf("clean %s: %w", contentDir, err)
	}
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", contentDir, err)
	}
	if err := writeRulePages(contentDir); err != nil {
		return fmt.Errorf("generate rule pages: %w", err)
	}
	// Update the README's own table before splitting it into pages below, so a stale table (e.g.
	// right after a rule is added or removed) doesn't get carried into the generated reference page.
	if err := updateReadmeRulesTable(readmePath); err != nil {
		return fmt.Errorf("update rules table in %s: %w", readmePath, err)
	}
	if err := writeReadmePages(readmePath, contentDir); err != nil {
		return fmt.Errorf("generate pages from %s: %w", readmePath, err)
	}
	return nil
}
