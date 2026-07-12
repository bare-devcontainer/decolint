package main

import (
	"io"
	"testing"

	"github.com/bare-devcontainer/decolint/format"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/google/go-cmp/cmp"
)

func TestParseOptionsPlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    []linter.Platform
		wantErr bool
	}{
		{"no flag", nil, nil, false},
		{"single platform", []string{"-platform=vscode"}, []linter.Platform{linter.PlatformVSCode}, false},
		{
			"multiple platforms",
			[]string{"-platform=vscode,codespaces"},
			[]linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces},
			false,
		},
		{"mixed case", []string{"-platform=VSCode"}, []linter.Platform{linter.PlatformVSCode}, false},
		{"unknown platform", []string{"-platform=bogus"}, nil, true},
		{
			"combined with other flags and paths",
			[]string{"-deny-warnings", "-platform=vscode", "."},
			[]linter.Platform{linter.PlatformVSCode},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, _, err := parseOptions(tt.args, io.Discard)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseOptions(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(tt.want, opts.Platforms); diff != "" {
				t.Errorf("Platforms mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseOptionsFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    Format
		wantErr bool
	}{
		{"no flag", nil, format.TextFormat{}, false},
		{"text", []string{"-format=text"}, format.TextFormat{}, false},
		{"json", []string{"-format=json"}, format.JSONFormat{}, false},
		{"github", []string{"-format=github"}, format.GitHubFormat{}, false},
		{"mixed case", []string{"-format=JSON"}, format.JSONFormat{}, false},
		{"unknown format", []string{"-format=bogus"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, _, err := parseOptions(tt.args, io.Discard)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseOptions(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if opts.Format != tt.want {
				t.Errorf("Format = %v, want %v", opts.Format, tt.want)
			}
		})
	}
}

func TestParseOptionsVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"single dash", []string{"-version"}},
		{"double dash", []string{"--version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, _, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if !opts.Version {
				t.Errorf("Version = false, want true")
			}
		})
	}
}

func TestParseOptionsListRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"single dash", []string{"-rules"}},
		{"double dash", []string{"--rules"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, _, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if !opts.ListRules {
				t.Errorf("ListRules = false, want true")
			}
		})
	}
}

func TestParseOptionsInit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"single dash", []string{"-init"}},
		{"double dash", []string{"--init"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, _, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if !opts.Init {
				t.Errorf("Init = false, want true")
			}
		})
	}
}

func TestParseOptionsMergeFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no flag", nil, false},
		{"single dash", []string{"-merge-features"}, true},
		{"double dash", []string{"--merge-features"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, _, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if opts.MergeFeatures != tt.want {
				t.Errorf("MergeFeatures = %v, want %v", opts.MergeFeatures, tt.want)
			}
		})
	}
}

func TestParseOptionsConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no flag", nil, ""},
		{"config flag", []string{"-config=path/to/config.jsonc"}, "path/to/config.jsonc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, configPath, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if configPath != tt.want {
				t.Errorf("configPath = %q, want %q", configPath, tt.want)
			}
		})
	}
}

func TestParseOptionsPaths(t *testing.T) {
	t.Parallel()

	opts, _, err := parseOptions(nil, io.Discard)
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if diff := cmp.Diff([]string{"."}, opts.Paths); diff != "" {
		t.Errorf("Paths mismatch (-want +got):\n%s", diff)
	}
}
