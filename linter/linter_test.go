package linter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
)

// noImageLatestRule is a test double for the rules package's no-image-latest rule: it flags an
// "image" property with no tag or the "latest" tag. Reusing its ID keeps testdata's
// decolint-ignore-next-line comments meaningful without this package depending on rules.
var noImageLatestRule = &Rule{
	ID:        "no-image-latest",
	FileTypes: []FileType{Devcontainer},
	Paths:     []string{"/image"},
	Check: func(_ *Context, node *Node) []Finding {
		lit, ok := node.Value.Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' {
			return nil
		}
		image := lit.String()
		tag, hasTag := "", false
		if i := strings.LastIndex(image, ":"); i >= 0 {
			tag, hasTag = image[i+1:], true
		}
		switch {
		case !hasTag:
			return []Finding{{Message: fmt.Sprintf("image %q has no explicit tag", image), Offset: node.Value.StartOffset}}
		case tag == "latest":
			return []Finding{{Message: fmt.Sprintf("image %q uses the \"latest\" tag", image), Offset: node.Value.StartOffset}}
		}
		return nil
	},
}

// lintSource parses src and applies l's registered rules to it as a file at the given path and of
// the given type, failing the test on any parse error. Like the rule tests' entry, it runs no
// Transform (see LintDocument).
func lintSource(t *testing.T, l *Linter, path string, fileType FileType, src string) []Issue {
	t.Helper()
	doc, err := ParseDocument([]byte(src))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	return l.LintDocument(path, fileType, doc)
}

// symlink creates a symbolic link, skipping the test on platforms where symlink creation is not
// permitted (e.g. Windows without the required privilege).
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

// openRoot opens dir as an os.Root, closed when the test ends, to lint or visit configs through.
func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
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

		l := New()
		if _, err := l.LintDir(t.Context(), openRoot(t, proj)); err == nil {
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

		l := New()
		if _, err := l.LintDir(t.Context(), openRoot(t, proj)); err == nil {
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

		l := New()
		if _, err := l.LintDir(t.Context(), openRoot(t, proj)); err == nil {
			t.Error("LintDir: got nil error, want 'no devcontainer configuration found'")
		}
	})

	t.Run("link resolving within .devcontainer is followed", func(t *testing.T) {
		t.Parallel()
		proj := setup(t, "main.json")
		if err := os.WriteFile(filepath.Join(proj, ".devcontainer", "main.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}

		l := New()
		if _, err := l.LintDir(t.Context(), openRoot(t, proj)); err != nil {
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

		l := New()
		if _, err := l.LintDir(t.Context(), openRoot(t, proj)); err != nil {
			t.Errorf("LintDir: %v", err)
		}
	})
}

func TestLintDir(t *testing.T) {
	t.Parallel()

	l := New()
	l.RegisterRule(noImageLatestRule, SeverityWarn)

	t.Run("definition with multiple configs", func(t *testing.T) {
		t.Parallel()
		issues, err := l.LintDir(t.Context(), openRoot(t, "testdata/project"))
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
		issues, err := l.LintDir(t.Context(), openRoot(t, "testdata/rootfile"))
		if err != nil {
			t.Fatalf("LintDir: %v", err)
		}
		if len(issues) != 0 {
			t.Errorf("got %d issues %v, want 0", len(issues), issues)
		}
	})

	t.Run("feature is not checked by devcontainer rules", func(t *testing.T) {
		t.Parallel()
		issues, err := l.LintDir(t.Context(), openRoot(t, "testdata/feature"))
		if err != nil {
			t.Fatalf("LintDir: %v", err)
		}
		if len(issues) != 0 {
			t.Errorf("got %d issues %v, want 0", len(issues), issues)
		}
	})

	t.Run("template lints the shipped devcontainer config", func(t *testing.T) {
		t.Parallel()
		issues, err := l.LintDir(t.Context(), openRoot(t, "testdata/template"))
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
		issues, err := l.LintDir(t.Context(), openRoot(t, "testdata/broken"))
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

	t.Run("directory without config", func(t *testing.T) {
		t.Parallel()
		if _, err := l.LintDir(t.Context(), openRoot(t, t.TempDir())); err == nil {
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
	l := New()
	l.RegisterRule(noImageLatestRule, SeverityWarn)
	got := lintSource(t, l, "devcontainer.json", Devcontainer, src)
	if len(got) != 1 {
		t.Fatalf("got %d issues, want 1", len(got))
	}
	if got[0].Line != 3 || got[0].Col != 12 {
		t.Errorf("position = %d:%d, want 3:12", got[0].Line, got[0].Col)
	}
}

func TestLintParseError(t *testing.T) {
	t.Parallel()

	if _, err := ParseDocument([]byte(`{`)); err == nil {
		t.Error("ParseDocument on malformed input: got nil error, want parse error")
	}
}

// flagRule is a stub Rule that reports the value at "/flag" when it is true, used to observe
// mutations a Transform makes to the syntax tree.
var flagRule = &Rule{
	ID:          "flag-rule",
	Description: "reports a true /flag value",
	FileTypes:   []FileType{Devcontainer},
	Paths:       []string{"/flag"},
	Check: func(_ *Context, node *Node) []Finding {
		if lit, ok := node.Value.Value.(hujson.Literal); ok && lit.Bool() {
			return []Finding{{Message: "flag is true", Offset: node.Value.StartOffset}}
		}
		return nil
	},
}

// lintTempDir writes src as the .devcontainer.json of a fresh temp directory and lints it through
// LintDir, exercising the full production path including any installed Transform.
func lintTempDir(t *testing.T, l *Linter, src string) ([]Issue, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".devcontainer.json"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return l.LintDir(t.Context(), openRoot(t, dir))
}

func TestLintTransform(t *testing.T) {
	t.Parallel()

	// The transform adds a synthetic "flag": true member whose offsets point at the "name" member of
	// the original source, so the finding must resolve to that position.
	src := "{\n  \"name\": \"test\"\n}"
	addFlag := func(_ context.Context, fctx *Context) error {
		obj := fctx.Root.Value.(*hujson.Object)
		anchor := obj.Members[0].Name.StartOffset
		obj.Members = append(obj.Members, hujson.ObjectMember{
			Name:  hujson.Value{Value: hujson.String("flag"), StartOffset: anchor, EndOffset: anchor},
			Value: hujson.Value{Value: hujson.Bool(true), StartOffset: anchor, EndOffset: anchor},
		})
		return nil
	}

	t.Run("rules see the transformed tree at anchored positions", func(t *testing.T) {
		t.Parallel()
		l := New()
		l.RegisterRule(flagRule, SeverityWarn)
		l.SetTransform(addFlag)
		issues, err := lintTempDir(t, l, src)
		if err != nil {
			t.Fatalf("LintDir: %v", err)
		}
		if len(issues) != 1 {
			t.Fatalf("got %d issues %v, want 1", len(issues), issues)
		}
		if issues[0].Line != 2 || issues[0].Col != 3 {
			t.Errorf("position = %d:%d, want 2:3", issues[0].Line, issues[0].Col)
		}
	})

	t.Run("transform error aborts the lint", func(t *testing.T) {
		t.Parallel()
		l := New()
		l.RegisterRule(flagRule, SeverityWarn)
		wantErr := errors.New("fetch failed")
		l.SetTransform(func(context.Context, *Context) error { return wantErr })
		if _, err := lintTempDir(t, l, src); !errors.Is(err, wantErr) {
			t.Errorf("LintDir error = %v, want %v", err, wantErr)
		}
	})

	t.Run("transform is skipped when no rule applies", func(t *testing.T) {
		t.Parallel()
		l := New()
		called := false
		l.SetTransform(func(context.Context, *Context) error {
			called = true
			return nil
		})
		if _, err := lintTempDir(t, l, src); err != nil {
			t.Fatalf("LintDir: %v", err)
		}
		if called {
			t.Error("transform ran although no rule is registered")
		}
	})
}

// panicRule is a stub Rule whose Check always panics, used to verify that the engine survives a
// defective rule instead of letting it abort the whole run.
var panicRule = &Rule{
	ID:          "panic-rule",
	Description: "always panics",
	FileTypes:   []FileType{Devcontainer},
	Paths:       []string{""},
	Check: func(*Context, *Node) []Finding {
		panic("boom")
	},
}

func TestLintRulePanicIsRecovered(t *testing.T) {
	t.Parallel()

	l := New()
	l.RegisterRule(panicRule, SeverityError)
	issues := lintSource(t, l, "devcontainer.json", Devcontainer, `{}`)
	if len(issues) != 1 {
		t.Fatalf("got %d issues %v, want 1", len(issues), issues)
	}
	if issues[0].RuleID != "panic-rule" {
		t.Errorf("RuleID = %q, want %q", issues[0].RuleID, "panic-rule")
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("Severity = %v, want %v", issues[0].Severity, SeverityError)
	}
}

// configRef identifies a visited configuration file independently of the root it is read through:
// its path relative to the lint directory, and its type.
type configRef struct {
	rel string
	typ FileType
}

func TestVisitConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dir  string
		want []configRef
	}{
		{
			"testdata/project",
			[]configRef{
				{filepath.Join(".devcontainer", "devcontainer.json"), Devcontainer},
				{filepath.Join(".devcontainer", "go", "devcontainer.json"), Devcontainer},
			},
		},
		{
			"testdata/rootfile",
			[]configRef{
				{".devcontainer.json", Devcontainer},
			},
		},
		{
			"testdata/feature",
			[]configRef{
				{"devcontainer-feature.json", Feature},
			},
		},
		{
			"testdata/template",
			[]configRef{
				{"devcontainer-template.json", Template},
				{filepath.Join(".devcontainer", "devcontainer.json"), Devcontainer},
			},
		},
		{
			"testdata/template-rootfile",
			[]configRef{
				{"devcontainer-template.json", Template},
				{".devcontainer.json", Devcontainer},
			},
		},
		{
			"testdata/template-subfolder",
			[]configRef{
				{"devcontainer-template.json", Template},
				{filepath.Join(".devcontainer", "go", "devcontainer.json"), Devcontainer},
			},
		},
		{
			"testdata/template-no-devcontainer",
			[]configRef{
				{"devcontainer-template.json", Template},
			},
		},
		{"testdata", nil},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			t.Parallel()
			var got []configRef
			err := visitConfigs(openRoot(t, tt.dir), func(f configEntry) error {
				if _, err := f.root.Stat(f.path); err != nil {
					t.Errorf("entry %q is not accessible through its root: %v", f.rel, err)
				}
				got = append(got, configRef{f.rel, f.typ})
				return nil
			})
			if err != nil {
				t.Fatalf("visitConfigs(%q): %v", tt.dir, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("visitConfigs(%q) visited %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestVisitConfigsStopsOnError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("stop")
	calls := 0
	// testdata/project contains two configs, so a first-call error must prevent a second call.
	err := visitConfigs(openRoot(t, "testdata/project"), func(configEntry) error {
		calls++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("visitConfigs returned %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want 1", calls)
	}
}
