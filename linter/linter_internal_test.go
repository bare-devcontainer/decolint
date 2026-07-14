package linter

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFindConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dir  string
		want []ConfigFile
	}{
		{
			"testdata/project",
			[]ConfigFile{
				{filepath.Join(".devcontainer", "devcontainer.json"), Devcontainer},
				{filepath.Join(".devcontainer", "go", "devcontainer.json"), Devcontainer},
			},
		},
		{
			"testdata/rootfile",
			[]ConfigFile{
				{".devcontainer.json", Devcontainer},
			},
		},
		{
			"testdata/feature",
			[]ConfigFile{
				{"devcontainer-feature.json", Feature},
			},
		},
		{
			"testdata/template",
			[]ConfigFile{
				{"devcontainer-template.json", Template},
				{filepath.Join(".devcontainer", "devcontainer.json"), Devcontainer},
			},
		},
		{
			"testdata/template-rootfile",
			[]ConfigFile{
				{"devcontainer-template.json", Template},
				{".devcontainer.json", Devcontainer},
			},
		},
		{
			"testdata/template-subfolder",
			[]ConfigFile{
				{"devcontainer-template.json", Template},
				{filepath.Join(".devcontainer", "go", "devcontainer.json"), Devcontainer},
			},
		},
		{
			"testdata/template-no-devcontainer",
			[]ConfigFile{
				{"devcontainer-template.json", Template},
			},
		},
		{"testdata", nil},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			t.Parallel()
			root, err := os.OpenRoot(tt.dir)
			if err != nil {
				t.Fatalf("os.OpenRoot(%q): %v", tt.dir, err)
			}
			t.Cleanup(func() { _ = root.Close() })
			got := findConfigs(root)
			if !slices.Equal(got, tt.want) {
				t.Errorf("findConfigs(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}
