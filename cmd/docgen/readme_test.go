package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureReadme is a minimal README.md with the section structure splitReadme requires: the
// headings it splits on, a cross-page anchor link in each direction, and the rules table markers,
// so a single fixture exercises the heading split, anchor rewriting and marker stripping together.
const fixtureReadme = `# example

[![CI](https://example.invalid/badge.svg)](https://example.invalid)

Intro paragraph. See [Config file](#config-file).

## Why decolint

- A landing page bullet.

## Try it

Getting started body. Self link: [Try it](#try-it).

## Linting a Feature or Template

Last guide section.

---

# Reference

## What decolint lints

Reference body.

## Config file

<!-- decolint:rules-table -->
| ID |
| --- |
| ` + "`x`" + ` |
<!-- /decolint:rules-table -->

## Contributing

Not part of the site.
`

func TestSplitReadme(t *testing.T) {
	t.Parallel()

	pages, err := splitReadme(fixtureReadme)
	if err != nil {
		t.Fatalf("splitReadme: %v", err)
	}
	for _, name := range []string{"_index", "getting-started", "reference"} {
		if _, ok := pages[name]; !ok {
			t.Errorf("splitReadme result has no %q page", name)
		}
	}

	landing := pages["_index"]
	if !strings.Contains(landing, "title: decolint") {
		t.Errorf("_index front matter missing title, got:\n%s", landing)
	}
	if !strings.Contains(landing, "Intro paragraph.") || !strings.Contains(landing, "landing page bullet") {
		t.Errorf("_index body missing expected content, got:\n%s", landing)
	}
	if strings.Contains(landing, "## Try it") {
		t.Errorf("_index leaked getting-started content, got:\n%s", landing)
	}
	// "Config file" lives on reference, so the landing page's link to it must be rewritten.
	if !strings.Contains(landing, "(reference.md#config-file)") {
		t.Errorf("_index anchor to #config-file was not rewritten to reference.md, got:\n%s", landing)
	}

	gs := pages["getting-started"]
	if !strings.Contains(gs, "title: Getting started") || !strings.Contains(gs, "toc: true") {
		t.Errorf("getting-started front matter missing title/toc, got:\n%s", gs)
	}
	if strings.Contains(gs, "Reference body") || strings.Contains(gs, "Not part of the site") {
		t.Errorf("getting-started leaked reference/contributing content, got:\n%s", gs)
	}
	// "Try it" is on the same page, so this link must stay untouched.
	if !strings.Contains(gs, "[Try it](#try-it)") {
		t.Errorf("getting-started same-page anchor was rewritten, got:\n%s", gs)
	}
	gsBody := strings.SplitN(gs, "\n---\n\n", 2)[1] // past the front matter fence
	if strings.Contains(gsBody, "---") {
		t.Errorf("getting-started retained the horizontal rule before # Reference, got body:\n%s", gsBody)
	}

	ref := pages["reference"]
	if !strings.Contains(ref, "title: Reference") || !strings.Contains(ref, "toc: true") {
		t.Errorf("reference front matter missing title/toc, got:\n%s", ref)
	}
	if strings.Contains(ref, "Not part of the site") {
		t.Errorf("reference leaked Contributing content, got:\n%s", ref)
	}
	if strings.Contains(ref, rulesTableStart) || strings.Contains(ref, rulesTableEnd) {
		t.Errorf("reference retained the rules table markers, got:\n%s", ref)
	}
	if !strings.Contains(ref, "| `x` |") {
		t.Errorf("reference lost the table content between the markers, got:\n%s", ref)
	}
}

func TestSplitReadme_MissingHeading(t *testing.T) {
	t.Parallel()

	_, err := splitReadme("# example\n\n## Try it\n\nno other headings\n")
	if err == nil {
		t.Fatal("splitReadme with missing headings: got nil error, want one")
	}
}

// TestSplitReadme_MissingTitle guards against a panic: with no "# " title above the sections,
// skipTitleAndBadges used to scan past the end of lines, and splitReadme sliced with that
// out-of-bounds index.
func TestSplitReadme_MissingTitle(t *testing.T) {
	t.Parallel()

	untitled := strings.Replace(fixtureReadme, "# example\n", "", 1)
	_, err := splitReadme(untitled)
	if err == nil {
		t.Fatal("splitReadme with no title: got nil error, want one")
	}
}

func TestWriteReadmePages(t *testing.T) {
	t.Parallel()

	readmePath := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(readmePath, []byte(fixtureReadme), 0o644); err != nil {
		t.Fatalf("write fixture README: %v", err)
	}
	dir := t.TempDir()

	if err := writeReadmePages(readmePath, dir); err != nil {
		t.Fatalf("writeReadmePages: %v", err)
	}
	for _, name := range []string{"_index.md", "getting-started.md", "reference.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("writeReadmePages did not create %s: %v", name, err)
		}
	}
}
