package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
