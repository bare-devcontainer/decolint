package linter_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func lintSrc(t *testing.T, src string) []linter.Issue {
	t.Helper()
	l := linter.New()
	if err := rules.RegisterRules(l, nil, nil); err != nil {
		t.Fatalf("RegisterRules: %v", err)
	}
	issues, err := l.Lint(t.Context(), "devcontainer.json", []byte(src), linter.Devcontainer)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	return issues
}

func TestFindConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dir  string
		want []linter.ConfigFile
	}{
		{
			"testdata/project",
			[]linter.ConfigFile{
				{filepath.Join("testdata", "project", ".devcontainer", "devcontainer.json"), linter.Devcontainer},
				{filepath.Join("testdata", "project", ".devcontainer", "go", "devcontainer.json"), linter.Devcontainer},
			},
		},
		{
			"testdata/rootfile",
			[]linter.ConfigFile{
				{filepath.Join("testdata", "rootfile", ".devcontainer.json"), linter.Devcontainer},
			},
		},
		{
			"testdata/feature",
			[]linter.ConfigFile{
				{filepath.Join("testdata", "feature", "devcontainer-feature.json"), linter.Feature},
			},
		},
		{
			"testdata/template",
			[]linter.ConfigFile{
				{filepath.Join("testdata", "template", "devcontainer-template.json"), linter.Template},
				{filepath.Join("testdata", "template", ".devcontainer", "devcontainer.json"), linter.Devcontainer},
			},
		},
		{
			"testdata/template-rootfile",
			[]linter.ConfigFile{
				{filepath.Join("testdata", "template-rootfile", "devcontainer-template.json"), linter.Template},
				{filepath.Join("testdata", "template-rootfile", ".devcontainer.json"), linter.Devcontainer},
			},
		},
		{
			"testdata/template-subfolder",
			[]linter.ConfigFile{
				{filepath.Join("testdata", "template-subfolder", "devcontainer-template.json"), linter.Template},
				{filepath.Join("testdata", "template-subfolder", ".devcontainer", "go", "devcontainer.json"), linter.Devcontainer},
			},
		},
		{
			"testdata/template-no-devcontainer",
			[]linter.ConfigFile{
				{filepath.Join("testdata", "template-no-devcontainer", "devcontainer-template.json"), linter.Template},
			},
		},
		{"testdata", nil},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			t.Parallel()
			got := linter.FindConfigs(tt.dir)
			if !slices.Equal(got, tt.want) {
				t.Errorf("linter.FindConfigs(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestLintDir(t *testing.T) {
	t.Parallel()

	l := linter.New()
	if err := rules.RegisterRules(l, nil, nil); err != nil {
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
	if err := rules.RegisterRules(l, nil, nil); err != nil {
		t.Fatalf("RegisterRules: %v", err)
	}
	if _, err := l.Lint(t.Context(), "bad.json", []byte(`{`), linter.Devcontainer); err == nil {
		t.Error("Lint on malformed input: got nil error, want parse error")
	}
}

// panicRule is a stub Rule whose Check always panics, used to verify that the engine survives a
// defective rule instead of letting it abort the whole run.
type panicRule struct{}

func (panicRule) ID() string                   { return "panic-rule" }
func (panicRule) Description() string          { return "always panics" }
func (panicRule) FileTypes() []linter.FileType { return []linter.FileType{linter.Devcontainer} }
func (panicRule) Platforms() []linter.Platform { return nil }
func (panicRule) Paths() []string              { return []string{""} }
func (panicRule) Check(*linter.Context, *linter.Node) []linter.Finding {
	panic("boom")
}

func TestLintRulePanicIsRecovered(t *testing.T) {
	t.Parallel()

	l := linter.New()
	l.RegisterRule(panicRule{}, linter.Error)
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
	if issues[0].Severity != linter.Error {
		t.Errorf("Severity = %v, want %v", issues[0].Severity, linter.Error)
	}
}
