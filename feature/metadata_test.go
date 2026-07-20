package feature

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseMetadata_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{"invalid hujson", `{`},
		{"root is an array", `[]`},
		{"root is a string", `"feature"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseMetadata([]byte(tt.src)); err == nil {
				t.Errorf("parseMetadata(%q): got nil error, want an error", tt.src)
			}
		})
	}
}

func TestParseMetadata(t *testing.T) {
	t.Parallel()

	md, err := parseMetadata([]byte(`{
		"id": "node",
		"version": "1.2.3",
		"dependsOn": {"./common": {"install": "true"}},
		"installsAfter": ["git", "curl"],
		"legacyIds": ["nodejs"]
	}`))
	if err != nil {
		t.Fatalf("parseMetadata: %v", err)
	}
	if md.ID != "node" {
		t.Errorf("ID = %q, want %q", md.ID, "node")
	}
	if md.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", md.Version, "1.2.3")
	}
	if len(md.DependsOn) != 1 || md.DependsOn[0].Ref != "./common" {
		t.Errorf("DependsOn = %+v, want a single ./common entry", md.DependsOn)
	}
	if diff := cmp.Diff([]string{"git", "curl"}, md.InstallsAfter); diff != "" {
		t.Errorf("InstallsAfter mismatch (-want +got):\n%s", diff)
	}
	// Aliases are the id followed by any legacyIds.
	if diff := cmp.Diff([]string{"node", "nodejs"}, md.Aliases); diff != "" {
		t.Errorf("Aliases mismatch (-want +got):\n%s", diff)
	}
}

// TestParseAliases covers the alias extraction edge cases: a missing id contributes nothing, and
// malformed legacyIds entries (empty strings and non-string values) are dropped so they cannot
// spuriously match another Feature.
func TestParseAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"id only, no legacyIds", `{"id": "a"}`, []string{"a"}},
		{"id with legacyIds", `{"id": "a", "legacyIds": ["b", "c"]}`, []string{"a", "b", "c"}},
		{"no id yields only legacyIds", `{"legacyIds": ["b"]}`, []string{"b"}},
		{"no id and no legacyIds", `{}`, nil},
		{"empty and non-string legacy entries dropped", `{"id": "a", "legacyIds": ["", "b", 1, null]}`, []string{"a", "b"}},
		{"legacyIds not an array is ignored", `{"id": "a", "legacyIds": "b"}`, []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			md, err := parseMetadata([]byte(tt.src))
			if err != nil {
				t.Fatalf("parseMetadata(%q): %v", tt.src, err)
			}
			if diff := cmp.Diff(tt.want, md.Aliases); diff != "" {
				t.Errorf("Aliases mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
