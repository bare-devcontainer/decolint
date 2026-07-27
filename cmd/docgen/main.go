// Command docgen generates the documentation site's rule pages and landing page, and the README's
// category summary, from rules/*.go and README.md, so none of them has to be hand-kept in sync with
// the others. It is not part of the decolint binary; "make docs" runs it before Hugo builds the site
// (see the Makefile).
package main

import (
	"fmt"
	"os"
)

// generatedContentDir is where the generated site pages are written. It is mounted alongside
// docs/content in hugo.toml and is never committed (see .gitignore); the pages that live in
// docs/content — Getting started, Reference, and docs/content/rules/_index.md — are hand-written.
const generatedContentDir = "docs/.generated/content"

func main() {
	if err := run("README.md", generatedContentDir); err != nil {
		fmt.Fprintln(os.Stderr, "docgen:", err)
		os.Exit(1)
	}
}

// run regenerates everything docgen owns: the rule pages and the landing page into contentDir, and
// the category summary in the README at readmePath, in place.
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
	if err := updateReadmeCategories(readmePath); err != nil {
		return fmt.Errorf("update category summary in %s: %w", readmePath, err)
	}
	if err := writeReadmePages(readmePath, contentDir); err != nil {
		return fmt.Errorf("generate pages from %s: %w", readmePath, err)
	}
	return nil
}
