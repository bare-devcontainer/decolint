package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
)

// symlink creates a symbolic link, skipping the test on platforms where symlink creation is not
// permitted (e.g. Windows without the required privilege).
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

// openRoot opens dir as an os.Root, closed when the test ends, to visit configs through.
func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestVisitConfigsSymlink(t *testing.T) {
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

	// visit runs VisitConfigs on dir, verifies every visited entry is readable through its Root, and
	// returns how many entries were visited.
	visit := func(t *testing.T, dir string) int {
		t.Helper()
		count := 0
		err := VisitConfigs(openRoot(t, dir), func(f ConfigFile) error {
			count++
			if _, err := f.Root.ReadFile(f.Path); err != nil {
				t.Errorf("entry %q is not readable through its Root: %v", f.Rel, err)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("VisitConfigs: %v", err)
		}
		return count
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

		if got := visit(t, proj); got != 0 {
			t.Errorf("visited %d configs, want 0", got)
		}
	})

	t.Run("link leaving .devcontainer is treated as nonexistent", func(t *testing.T) {
		t.Parallel()
		proj := setup(t, filepath.Join("..", "real.json"))
		// The target is inside the lint directory, but outside .devcontainer.
		if err := os.WriteFile(filepath.Join(proj, "real.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := visit(t, proj); got != 0 {
			t.Errorf("visited %d configs, want 0", got)
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

		if got := visit(t, proj); got != 0 {
			t.Errorf("visited %d configs, want 0", got)
		}
	})

	t.Run("link resolving within .devcontainer is followed", func(t *testing.T) {
		t.Parallel()
		proj := setup(t, "main.json")
		if err := os.WriteFile(filepath.Join(proj, ".devcontainer", "main.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := visit(t, proj); got != 1 {
			t.Errorf("visited %d configs, want 1", got)
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

		if got := visit(t, proj); got != 1 {
			t.Errorf("visited %d configs, want 1", got)
		}
	})
}

// configRef identifies a visited configuration file independently of the root it is read through:
// its path relative to the lint directory, and its type.
type configRef struct {
	rel string
	typ linter.FileType
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
				{filepath.Join(".devcontainer", "devcontainer.json"), linter.Devcontainer},
				{filepath.Join(".devcontainer", "go", "devcontainer.json"), linter.Devcontainer},
			},
		},
		{
			"testdata/rootfile",
			[]configRef{
				{".devcontainer.json", linter.Devcontainer},
			},
		},
		{
			"testdata/feature",
			[]configRef{
				{"devcontainer-feature.json", linter.Feature},
			},
		},
		{
			"testdata/template",
			[]configRef{
				{"devcontainer-template.json", linter.Template},
				{filepath.Join(".devcontainer", "devcontainer.json"), linter.Devcontainer},
			},
		},
		{
			"testdata/template-rootfile",
			[]configRef{
				{"devcontainer-template.json", linter.Template},
				{".devcontainer.json", linter.Devcontainer},
			},
		},
		{
			"testdata/template-subfolder",
			[]configRef{
				{"devcontainer-template.json", linter.Template},
				{filepath.Join(".devcontainer", "go", "devcontainer.json"), linter.Devcontainer},
			},
		},
		{
			"testdata/template-no-devcontainer",
			[]configRef{
				{"devcontainer-template.json", linter.Template},
			},
		},
		{"testdata", nil},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			t.Parallel()
			var got []configRef
			err := VisitConfigs(openRoot(t, tt.dir), func(f ConfigFile) error {
				if _, err := f.Root.Stat(f.Path); err != nil {
					t.Errorf("entry %q is not accessible through its Root: %v", f.Rel, err)
				}
				got = append(got, configRef{f.Rel, f.Type})
				return nil
			})
			if err != nil {
				t.Fatalf("VisitConfigs(%q): %v", tt.dir, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("VisitConfigs(%q) visited %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestVisitConfigsStopsOnError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("stop")
	calls := 0
	// testdata/project contains two configs, so a first-call error must prevent a second call.
	err := VisitConfigs(openRoot(t, "testdata/project"), func(ConfigFile) error {
		calls++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("VisitConfigs returned %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want 1", calls)
	}
}
