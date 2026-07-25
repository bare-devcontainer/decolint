package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRawURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		repo string
		want string
	}{
		{"spec", "https://raw.githubusercontent.com/devcontainers/spec/abc123/schemas/x.json"},
		{"vscode", "https://raw.githubusercontent.com/microsoft/vscode/abc123/schemas/x.json"},
	}
	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			t.Parallel()
			if got := rawURL(tt.repo, "abc123", "schemas/x.json"); got != tt.want {
				t.Errorf("rawURL(%q) = %q, want %q", tt.repo, got, tt.want)
			}
		})
	}
}

func TestWriteRevisions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rev := revisions{
		Spec:    "spec-sha",
		VSCode:  "vscode-sha",
		Sources: map[string]string{"devContainer.schema.json": "https://example.invalid/s.json"},
	}
	if err := writeRevisions(dir, rev); err != nil {
		t.Fatalf("writeRevisions: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "REVISIONS.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(b)
	for _, want := range []string{`"spec": "spec-sha"`, `"vscode": "vscode-sha"`, "https://example.invalid/s.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("REVISIONS.json missing %q; got:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "}\n") {
		t.Errorf("REVISIONS.json should end with a newline; got:\n%s", got)
	}
}

// TestWriteRevisions_Deterministic checks that the same revisions always marshal to the same bytes.
// The sources are a map, whose order is unspecified, so without pinning it the file would be
// rewritten in a new order on every run and the sync workflow would raise a pull request that
// changes no schema.
func TestWriteRevisions_Deterministic(t *testing.T) {
	t.Parallel()

	rev := revisions{
		Spec:   "spec-sha",
		VSCode: "vscode-sha",
		Sources: map[string]string{
			"devContainer.base.schema.json":       "https://example.invalid/base.json",
			"devContainer.schema.json":            "https://example.invalid/main.json",
			"devContainerFeature.schema.json":     "https://example.invalid/feature.json",
			"devContainer.codespaces.schema.json": "https://example.invalid/codespaces.json",
			"devContainer.vscode.schema.json":     "https://example.invalid/vscode.json",
		},
	}
	// One write per iteration, each into a fresh directory, so any dependence on map iteration order
	// shows up as a differing result.
	var first string
	for i := range 10 {
		dir := t.TempDir()
		if err := writeRevisions(dir, rev); err != nil {
			t.Fatalf("writeRevisions: %v", err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "REVISIONS.json"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if i == 0 {
			first = string(b)
			continue
		}
		if diff := cmp.Diff(first, string(b)); diff != "" {
			t.Fatalf("run %d differs from the first (-first +got):\n%s", i, diff)
		}
	}
}
