package main

import "testing"

func TestSlugify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		heading string
		want    string
	}{
		{"Why decolint", "why-decolint"},
		{"1. Run it", "1-run-it"},
		{"4. Lint what actually runs", "4-lint-what-actually-runs"},
		{"Prebuilt binary (recommended)", "prebuilt-binary-recommended"},
		{"Config file", "config-file"},
		// Verified against Hugo's own heading-id output, not guessed: an underscore is kept, a
		// leading or trailing hyphen is kept, and repeated spaces are not collapsed into one hyphen.
		{"Local_env setting", "local_env-setting"},
		{"-format", "-format"},
		{"trailing-", "trailing-"},
		{"foo__bar", "foo__bar"},
		{"Extra   spaces", "extra---spaces"},
	}
	for _, tt := range tests {
		if got := slugify(tt.heading); got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.heading, got, tt.want)
		}
	}
}

func TestScanHeadings_SkipsFencedCode(t *testing.T) {
	t.Parallel()

	body := "## Real heading\n\n```console\n# not a heading\n## also not one\n```\n\n## Another\n"
	got := scanHeadings(body)
	if len(got) != 2 {
		t.Fatalf("scanHeadings found %d heading(s), want 2: %+v", len(got), got)
	}
	if got[0].Text != "Real heading" || got[1].Text != "Another" {
		t.Errorf("scanHeadings = %+v", got)
	}
}

func TestRewriteAnchors(t *testing.T) {
	t.Parallel()

	slugPage := map[string]string{
		"local":  "getting-started",
		"remote": "reference",
	}
	body := "See [here](#local) and [there](#remote)."
	got, err := rewriteAnchors(body, "getting-started", slugPage)
	if err != nil {
		t.Fatalf("rewriteAnchors: %v", err)
	}
	want := "See [here](#local) and [there](reference.md#remote)."
	if got != want {
		t.Errorf("rewriteAnchors() = %q, want %q", got, want)
	}
}

// TestRewriteAnchors_UnderscoreSlug guards anchorLink's character class: a slug containing an
// underscore (e.g. from a heading like "Local_env setting") has to be recognized as a same-document
// link before it can be rewritten, the same as any other slug.
func TestRewriteAnchors_UnderscoreSlug(t *testing.T) {
	t.Parallel()

	slugPage := map[string]string{"local_env-setting": "reference"}
	body := "See [here](#local_env-setting)."
	got, err := rewriteAnchors(body, "getting-started", slugPage)
	if err != nil {
		t.Fatalf("rewriteAnchors: %v", err)
	}
	want := "See [here](reference.md#local_env-setting)."
	if got != want {
		t.Errorf("rewriteAnchors() = %q, want %q", got, want)
	}
}

func TestRewriteAnchors_SkipsFencedCode(t *testing.T) {
	t.Parallel()

	slugPage := map[string]string{"remote": "reference"}
	body := "text [a](#remote)\n\n```\n[b](#remote)\n```\n"
	got, err := rewriteAnchors(body, "getting-started", slugPage)
	if err != nil {
		t.Fatalf("rewriteAnchors: %v", err)
	}
	want := "text [a](reference.md#remote)\n\n```\n[b](#remote)\n```\n"
	if got != want {
		t.Errorf("rewriteAnchors() = %q, want %q", got, want)
	}
}

// TestRewriteAnchors_UnknownSlug guards against a dead link publishing silently: a "(#slug)" link to
// a heading that exists nowhere among the marked pages (e.g. "#contributing", which is real content
// in README.md but outside every page marker) has to fail the build, not pass through unrewritten.
func TestRewriteAnchors_UnknownSlug(t *testing.T) {
	t.Parallel()

	slugPage := map[string]string{"remote": "reference"}
	body := "See [there](#remote) and [nowhere](#contributing)."
	_, err := rewriteAnchors(body, "getting-started", slugPage)
	if err == nil {
		t.Fatal("rewriteAnchors with a link to an unknown slug: got nil error, want one")
	}
}
