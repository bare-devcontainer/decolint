package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureReadme is a minimal README.md with the page markers splitReadme requires, a cross-page
// anchor link in each direction, and the rules table markers, so a single fixture exercises the
// marker split, anchor rewriting and marker stripping together. It also has content outside any
// page marker (the title, badges, the "---", "## Contributing") that must never reach a page.
const fixtureReadme = `# example

[![CI](https://example.invalid/badge.svg)](https://example.invalid)

<!-- decolint:page=_index -->
Intro paragraph. See [Config file](#config-file).

## Why decolint

- A landing page bullet.
<!-- decolint:end-page -->

<!-- decolint:page=getting-started -->
## Try it

Getting started body. Self link: [Try it](#try-it).

## Linting a Feature or Template

Last guide section.
<!-- decolint:end-page -->

---

# Reference

<!-- decolint:page=reference -->
## What decolint lints

Reference body.

## Config file

<!-- decolint:rules-table -->
| ID |
| --- |
| ` + "`x`" + ` |
<!-- /decolint:rules-table -->
<!-- decolint:end-page -->

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

// TestSplitReadme_DuplicateHeadingSlug guards against nondeterminism: slugPage used to be built by
// ranging the bodies map, so two pages sharing a heading slug resolved to whichever page Go's map
// iteration visited last — differently from run to run — instead of failing.
func TestSplitReadme_DuplicateHeadingSlug(t *testing.T) {
	t.Parallel()

	dup := strings.Replace(fixtureReadme, "## What decolint lints", "## Why decolint", 1)
	_, err := splitReadme(dup)
	if err == nil {
		t.Fatal("splitReadme with a heading slug shared by two pages: got nil error, want one")
	}
}

// TestSplitReadme_LinkToUnmarkedContent guards against a dead link publishing silently: "Contributing"
// is real content in README.md, but outside every page marker, so a link to it from within a marked
// page has nowhere to resolve to once split onto a real site page.
func TestSplitReadme_LinkToUnmarkedContent(t *testing.T) {
	t.Parallel()

	src := strings.Replace(fixtureReadme, "Last guide section.", "Last guide section. See [Contributing](#contributing).", 1)
	_, err := splitReadme(src)
	if err == nil {
		t.Fatal("splitReadme with a link to unmarked content: got nil error, want one")
	}
}

func TestSplitReadme_MarkerErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "a page is never marked",
			src:  "<!-- decolint:page=_index -->\nbody\n<!-- decolint:end-page -->\n",
		},
		{
			name: "an unknown page name",
			src:  "<!-- decolint:page=bogus -->\nbody\n<!-- decolint:end-page -->\n",
		},
		{
			name: "end-page with no page marker open",
			src:  "<!-- decolint:end-page -->\n",
		},
		{
			name: "a page marker starts before the previous one ends",
			src:  "<!-- decolint:page=_index -->\n<!-- decolint:page=reference -->\nbody\n<!-- decolint:end-page -->\n<!-- decolint:end-page -->\n",
		},
		{
			name: "a page is marked twice",
			src:  "<!-- decolint:page=_index -->\na\n<!-- decolint:end-page -->\n<!-- decolint:page=_index -->\nb\n<!-- decolint:end-page -->\n",
		},
		{
			name: "a page marker is never closed",
			src:  "<!-- decolint:page=_index -->\nbody\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := splitReadme(tt.src)
			if err == nil {
				t.Fatal("splitReadme: got nil error, want one")
			}
		})
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

func TestWriteReadmePages_MissingReadme(t *testing.T) {
	t.Parallel()

	readmePath := filepath.Join(t.TempDir(), "absent.md")
	if err := writeReadmePages(readmePath, t.TempDir()); err == nil {
		t.Fatal("writeReadmePages with a missing README: got nil error, want one")
	}
}
