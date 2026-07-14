package linter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func lintSrc(t *testing.T, src string) []linter.Issue {
	t.Helper()
	l := linter.New()
	// Only the correctness category is enabled by default; enable no-image-latest specifically,
	// since tests using this helper rely on it firing.
	overrides := rules.Overrides{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityWarn}}
	if err := rules.RegisterRules(l, nil, overrides); err != nil {
		t.Fatalf("RegisterRules: %v", err)
	}
	issues, err := l.Lint(t.Context(), "devcontainer.json", []byte(src), linter.Devcontainer)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	return issues
}

// symlink creates a symbolic link, skipping the test on platforms where symlink creation is not
// permitted (e.g. Windows without the required privilege).
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func TestLintDirSymlink(t *testing.T) {
	t.Parallel()

	// setup creates a dev container definition directory whose .devcontainer/devcontainer.json is a
	// symbolic link to target, and returns the directory.
	setup := func(t *testing.T, target string) string {
		t.Helper()
		proj := t.TempDir()
		if err := os.MkdirAll(filepath.Join(proj, ".devcontainer"), 0o755); err != nil {
			t.Fatal(err)
		}
		symlink(t, target, filepath.Join(proj, ".devcontainer", "devcontainer.json"))
		return proj
	}

	t.Run("link escaping the lint directory is treated as nonexistent", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmp, "devcontainer.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
		proj := filepath.Join(tmp, "proj")
		if err := os.MkdirAll(filepath.Join(proj, ".devcontainer"), 0o755); err != nil {
			t.Fatal(err)
		}
		symlink(t, filepath.Join("..", "..", "devcontainer.json"),
			filepath.Join(proj, ".devcontainer", "devcontainer.json"))

		l := linter.New()
		if _, err := l.LintDir(t.Context(), proj); err == nil {
			t.Error("LintDir: got nil error, want 'no devcontainer configuration found'")
		}
	})

	t.Run("link leaving .devcontainer is treated as nonexistent", func(t *testing.T) {
		t.Parallel()
		proj := setup(t, filepath.Join("..", "real.json"))
		// The target is inside the lint directory, but outside .devcontainer.
		if err := os.WriteFile(filepath.Join(proj, "real.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}

		l := linter.New()
		if _, err := l.LintDir(t.Context(), proj); err == nil {
			t.Error("LintDir: got nil error, want 'no devcontainer configuration found'")
		}
	})

	t.Run("link with an absolute target is treated as nonexistent", func(t *testing.T) {
		t.Parallel()
		// The target is inside .devcontainer, but os.Root rejects absolute symlink targets.
		proj := t.TempDir()
		if err := os.MkdirAll(filepath.Join(proj, ".devcontainer"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proj, ".devcontainer", "real.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
		symlink(t, filepath.Join(proj, ".devcontainer", "real.json"),
			filepath.Join(proj, ".devcontainer", "devcontainer.json"))

		l := linter.New()
		if _, err := l.LintDir(t.Context(), proj); err == nil {
			t.Error("LintDir: got nil error, want 'no devcontainer configuration found'")
		}
	})

	t.Run("link resolving within .devcontainer is followed", func(t *testing.T) {
		t.Parallel()
		proj := setup(t, "main.json")
		if err := os.WriteFile(filepath.Join(proj, ".devcontainer", "main.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}

		l := linter.New()
		if _, err := l.LintDir(t.Context(), proj); err != nil {
			t.Errorf("LintDir: %v", err)
		}
	})

	t.Run("link from a subfolder resolving within .devcontainer is followed", func(t *testing.T) {
		t.Parallel()
		proj := t.TempDir()
		if err := os.MkdirAll(filepath.Join(proj, ".devcontainer", "go"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proj, ".devcontainer", "shared.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
		symlink(t, filepath.Join("..", "shared.json"),
			filepath.Join(proj, ".devcontainer", "go", "devcontainer.json"))

		l := linter.New()
		if _, err := l.LintDir(t.Context(), proj); err != nil {
			t.Errorf("LintDir: %v", err)
		}
	})
}

func TestLintDir(t *testing.T) {
	t.Parallel()

	l := linter.New()
	// Only the correctness category is enabled by default; enable no-image-latest specifically,
	// since the fixtures below rely on it firing.
	overrides := rules.Overrides{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityWarn}}
	if err := rules.RegisterRules(l, nil, overrides); err != nil {
		t.Fatalf("RegisterRules: %v", err)
	}

	t.Run("definition with multiple configs", func(t *testing.T) {
		t.Parallel()
		issues, err := l.LintDir(t.Context(), "testdata/project")
		if err != nil {
			t.Fatalf("LintDir: %v", err)
		}
		// The main config uses ubuntu:latest (1 issue); the "go" folder config has an untagged image
		// suppressed by an ignore comment.
		if len(issues) != 1 {
			t.Fatalf("got %d issues %v, want 1", len(issues), issues)
		}
		wantPath := filepath.Join("testdata", "project", ".devcontainer", "devcontainer.json")
		if issues[0].Path != wantPath {
			t.Errorf("Path = %q, want %q", issues[0].Path, wantPath)
		}
		if issues[0].RuleID != "no-image-latest" {
			t.Errorf("RuleID = %q, want %q", issues[0].RuleID, "no-image-latest")
		}
	})

	t.Run("clean definition", func(t *testing.T) {
		t.Parallel()
		issues, err := l.LintDir(t.Context(), "testdata/rootfile")
		if err != nil {
			t.Fatalf("LintDir: %v", err)
		}
		if len(issues) != 0 {
			t.Errorf("got %d issues %v, want 0", len(issues), issues)
		}
	})

	t.Run("feature is not checked by devcontainer rules", func(t *testing.T) {
		t.Parallel()
		issues, err := l.LintDir(t.Context(), "testdata/feature")
		if err != nil {
			t.Fatalf("LintDir: %v", err)
		}
		if len(issues) != 0 {
			t.Errorf("got %d issues %v, want 0", len(issues), issues)
		}
	})

	t.Run("template lints the shipped devcontainer config", func(t *testing.T) {
		t.Parallel()
		issues, err := l.LintDir(t.Context(), "testdata/template")
		if err != nil {
			t.Fatalf("LintDir: %v", err)
		}
		if len(issues) != 1 {
			t.Fatalf("got %d issues %v, want 1", len(issues), issues)
		}
		wantPath := filepath.Join("testdata", "template", ".devcontainer", "devcontainer.json")
		if issues[0].Path != wantPath {
			t.Errorf("Path = %q, want %q", issues[0].Path, wantPath)
		}
	})

	t.Run("a broken file does not stop other files in the same directory from being linted", func(t *testing.T) {
		t.Parallel()
		issues, err := l.LintDir(t.Context(), "testdata/broken")
		if err == nil {
			t.Fatal("got nil error, want a parse error for the broken config")
		}
		if len(issues) != 1 {
			t.Fatalf("got %d issues %v, want 1", len(issues), issues)
		}
		wantPath := filepath.Join("testdata", "broken", ".devcontainer", "good", "devcontainer.json")
		if issues[0].Path != wantPath {
			t.Errorf("Path = %q, want %q", issues[0].Path, wantPath)
		}
	})

	t.Run("file path is rejected", func(t *testing.T) {
		t.Parallel()
		file := filepath.Join("testdata", "project", ".devcontainer", "devcontainer.json")
		if _, err := l.LintDir(t.Context(), file); err == nil {
			t.Error("got nil error, want 'not a directory'")
		}
	})

	t.Run("directory without config", func(t *testing.T) {
		t.Parallel()
		if _, err := l.LintDir(t.Context(), t.TempDir()); err == nil {
			t.Error("got nil error, want 'no devcontainer configuration found'")
		}
	})
}

func TestIssuePosition(t *testing.T) {
	t.Parallel()

	src := `{
  "name": "test",
  "image": "ubuntu:latest"
}`
	got := lintSrc(t, src)
	if len(got) != 1 {
		t.Fatalf("got %d issues, want 1", len(got))
	}
	if got[0].Line != 3 || got[0].Col != 12 {
		t.Errorf("position = %d:%d, want 3:12", got[0].Line, got[0].Col)
	}
}

func TestLintParseError(t *testing.T) {
	t.Parallel()

	l := linter.New()
	if err := rules.RegisterRules(l, nil, rules.Overrides{}); err != nil {
		t.Fatalf("RegisterRules: %v", err)
	}
	if _, err := l.Lint(t.Context(), "bad.json", []byte(`{`), linter.Devcontainer); err == nil {
		t.Error("Lint on malformed input: got nil error, want parse error")
	}
}

// panicRule is a stub Rule whose Check always panics, used to verify that the engine survives a
// defective rule instead of letting it abort the whole run.
var panicRule = &linter.Rule{
	ID:          "panic-rule",
	Description: "always panics",
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{""},
	Check: func(*linter.Context, *linter.Node) []linter.Finding {
		panic("boom")
	},
}

func TestLintRulePanicIsRecovered(t *testing.T) {
	t.Parallel()

	l := linter.New()
	l.RegisterRule(panicRule, linter.SeverityError)
	issues, err := l.Lint(t.Context(), "devcontainer.json", []byte(`{}`), linter.Devcontainer)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues %v, want 1", len(issues), issues)
	}
	if issues[0].RuleID != "panic-rule" {
		t.Errorf("RuleID = %q, want %q", issues[0].RuleID, "panic-rule")
	}
	if issues[0].Severity != linter.SeverityError {
		t.Errorf("Severity = %v, want %v", issues[0].Severity, linter.SeverityError)
	}
}
