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
		{"  Extra   spaces  ", "extra-spaces"},
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
	body := "See [here](#local) and [there](#remote), plus [unknown](#nowhere)."
	got := rewriteAnchors(body, "getting-started", slugPage)
	want := "See [here](#local) and [there](reference.md#remote), plus [unknown](#nowhere)."
	if got != want {
		t.Errorf("rewriteAnchors() = %q, want %q", got, want)
	}
}

func TestRewriteAnchors_SkipsFencedCode(t *testing.T) {
	t.Parallel()

	slugPage := map[string]string{"remote": "reference"}
	body := "text [a](#remote)\n\n```\n[b](#remote)\n```\n"
	got := rewriteAnchors(body, "getting-started", slugPage)
	want := "text [a](reference.md#remote)\n\n```\n[b](#remote)\n```\n"
	if got != want {
		t.Errorf("rewriteAnchors() = %q, want %q", got, want)
	}
}
