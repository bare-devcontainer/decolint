package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureReadme is a minimal README.md with the page marker splitReadme requires, a same-page anchor
// link, and the category markers, so a single fixture exercises the marker split and the anchor
// rewriting together. It also has content outside the page marker (the title, badges, "## Try it",
// the category table, "## Contributing") that must never reach a page.
const fixtureReadme = `# example

[![CI](https://example.invalid/badge.svg)](https://example.invalid)

<!-- decolint:page=_index -->
Intro paragraph. See [Why decolint](#why-decolint).

## Why decolint

- A landing page bullet.
<!-- decolint:end-page -->

## Try it

README-only content.

<!-- decolint:categories -->
| Category |
| --- |
| ` + "`x`" + ` |
<!-- /decolint:categories -->

## Contributing

Not part of the site.
`

func TestSplitReadme(t *testing.T) {
	t.Parallel()

	pages, err := splitReadme(fixtureReadme)
	if err != nil {
		t.Fatalf("splitReadme: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("splitReadme produced %d page(s), want 1", len(pages))
	}

	landing, ok := pages["_index"]
	if !ok {
		t.Fatal("splitReadme result has no \"_index\" page")
	}
	if !strings.Contains(landing, "title: decolint") {
		t.Errorf("_index front matter missing title, got:\n%s", landing)
	}
	if !strings.Contains(landing, "Intro paragraph.") || !strings.Contains(landing, "landing page bullet") {
		t.Errorf("_index body missing expected content, got:\n%s", landing)
	}
	// "Why decolint" is on the same page, so this link must stay untouched.
	if !strings.Contains(landing, "[Why decolint](#why-decolint)") {
		t.Errorf("_index same-page anchor was rewritten, got:\n%s", landing)
	}
	for _, unmarked := range []string{"## Try it", "Not part of the site", categoriesStart, "| `x` |"} {
		if strings.Contains(landing, unmarked) {
			t.Errorf("_index leaked unmarked content %q, got:\n%s", unmarked, landing)
		}
	}
}

// TestSplitReadme_DuplicateHeadingSlug guards against an ambiguous anchor: two headings sharing a
// slug leave a "(#slug)" link with two places it could resolve to, and slugPage used to keep
// whichever Go's map iteration visited last — differently from run to run — instead of failing.
func TestSplitReadme_DuplicateHeadingSlug(t *testing.T) {
	t.Parallel()

	dup := strings.Replace(fixtureReadme, "- A landing page bullet.", "## Why decolint", 1)
	_, err := splitReadme(dup)
	if err == nil {
		t.Fatal("splitReadme with a heading slug used twice: got nil error, want one")
	}
}

// TestSplitReadme_LinkToUnmarkedContent guards against a dead link publishing silently:
// "Contributing" is real content in README.md, but outside the page marker, so a link to it from
// within the marked page has nowhere to resolve to once split onto a real site page.
func TestSplitReadme_LinkToUnmarkedContent(t *testing.T) {
	t.Parallel()

	src := strings.Replace(fixtureReadme, "- A landing page bullet.", "See [Contributing](#contributing).", 1)
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
			name: "the page is never marked",
			src:  "no markers here\n",
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
			src:  "<!-- decolint:page=_index -->\n<!-- decolint:page=_index -->\nbody\n<!-- decolint:end-page -->\n<!-- decolint:end-page -->\n",
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
	if _, err := os.Stat(filepath.Join(dir, "_index.md")); err != nil {
		t.Errorf("writeReadmePages did not create _index.md: %v", err)
	}
}

func TestWriteReadmePages_MissingReadme(t *testing.T) {
	t.Parallel()

	readmePath := filepath.Join(t.TempDir(), "absent.md")
	if err := writeReadmePages(readmePath, t.TempDir()); err == nil {
		t.Fatal("writeReadmePages with a missing README: got nil error, want one")
	}
}
