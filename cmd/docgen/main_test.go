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

	for _, name := range []string{"_index.md", "getting-started.md", "reference.md"} {
		if _, err := os.Stat(filepath.Join(contentDir, name)); err != nil {
			t.Errorf("run did not create %s: %v", name, err)
		}
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
	if string(got) == fixtureReadme {
		t.Error("run did not touch the README's rules table (fixture has a stale placeholder row)")
	}

	// reference.md is split from the README, so it must carry the table run just refreshed above,
	// not the fixture's stale placeholder row that was on disk when run started.
	reference, err := os.ReadFile(filepath.Join(contentDir, "reference.md"))
	if err != nil {
		t.Fatalf("read reference.md: %v", err)
	}
	if strings.Contains(string(reference), "`x`") {
		t.Error("reference.md still has the fixture's stale placeholder row")
	}
	if !strings.Contains(string(reference), rules.Builtin()[0].Rule.ID) {
		t.Errorf("reference.md missing rule ID %q from the refreshed table", rules.Builtin()[0].Rule.ID)
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
