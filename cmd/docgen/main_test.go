package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/rules"
)

func TestRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte(fixtureReadme), 0o644); err != nil {
		t.Fatalf("write fixture README: %v", err)
	}
	contentDir := filepath.Join(dir, "content")

	if err := run(readmePath, contentDir); err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(contentDir, "_index.md")); err != nil {
		t.Errorf("run did not create _index.md: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(contentDir, "rules"))
	if err != nil {
		t.Fatalf("read rules dir: %v", err)
	}
	if len(entries) != len(rules.Builtin()) {
		t.Errorf("run wrote %d rule page(s), want %d", len(entries), len(rules.Builtin()))
	}

	got, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %s: %v", readmePath, err)
	}
	if strings.Contains(string(got), "| `x` |") {
		t.Error("run left the fixture's stale placeholder row in the README's category summary")
	}
	if !strings.Contains(string(got), rules.Builtin()[0].Rule.Category.String()) {
		t.Errorf("README missing category %q from the refreshed summary", rules.Builtin()[0].Rule.Category)
	}
}

// TestRun_RegeneratesCleanly checks that run replaces contentDir's previous output rather than
// merely adding to it: a stale file left over from a since-removed rule must not survive a rerun.
func TestRun_RegeneratesCleanly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte(fixtureReadme), 0o644); err != nil {
		t.Fatalf("write fixture README: %v", err)
	}
	contentDir := filepath.Join(dir, "content")

	stale := filepath.Join(contentDir, "rules", "stale-rule.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(stale), err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write %s: %v", stale, err)
	}

	if err := run(readmePath, contentDir); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("run left %s behind, want it removed", stale)
	}
}
