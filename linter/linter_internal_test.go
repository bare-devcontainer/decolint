package linter

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// configRef identifies a visited configuration file independently of the root it is read through:
// its path relative to the lint directory, and its type.
type configRef struct {
	rel string
	typ FileType
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
