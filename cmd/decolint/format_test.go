package main

import (
	"testing"

	"github.com/bare-devcontainer/decolint/format"
	"github.com/google/go-cmp/cmp"
)

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
