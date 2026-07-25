package main

import (
	"testing"

	"github.com/bare-devcontainer/decolint/cmd/decolint/format"
	"github.com/bare-devcontainer/decolint/rules"
	"github.com/google/go-cmp/cmp"
)

// TestSARIFRules checks that the adapter carries every documented field of a built-in rule into the
// SARIF catalog, so an alert can be traced back to what the rule checks and why.
func TestSARIFRules(t *testing.T) {
	t.Parallel()

	var reg rules.Registration
	for _, r := range rules.Builtin() {
		if r.Rule.ID == "no-image-latest" {
			reg = r
		}
	}
	if reg.Rule == nil {
		t.Fatal("built-in rule no-image-latest not found")
	}

	var got format.SARIFRule
	for _, r := range sarifRules() {
		if r.ID == reg.Rule.ID {
			got = r
		}
	}

	want := format.SARIFRule{
		ID:          reg.Rule.ID,
		Description: reg.Rule.Description,
		Category:    reg.Rule.Category.String(),
		HelpURI:     rules.DocsURL(reg.Rule.ID),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sarifRules() entry mismatch (-want +got):\n%s", diff)
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    Format
		wantErr bool
	}{
		{"empty", "", format.TextFormat{}, false},
		{"text", "text", format.TextFormat{}, false},
		{"json", "json", format.JSONFormat{}, false},
		{"github", "github", format.GitHubFormat{}, false},
		{"sarif", "sarif", format.SARIFFormat{Version: version, Rules: sarifRules()}, false},
		{"unknown", "bogus", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseFormat(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFormat(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// cmp.Diff rather than ==: SARIFFormat is not comparable, so an interface comparison
			// would panic.
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseFormat(%q) mismatch (-want +got):\n%s", tt.in, diff)
			}
		})
	}
}
