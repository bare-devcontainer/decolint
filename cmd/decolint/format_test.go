package main

import (
	"testing"

	"github.com/bare-devcontainer/decolint/format"
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
			if got != tt.want {
				t.Errorf("parseFormat(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
