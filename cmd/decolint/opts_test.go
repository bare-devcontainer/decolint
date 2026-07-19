package main

import (
	"fmt"
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
		// Empty entries from stray commas or surrounding whitespace are skipped, not rejected.
		{"empty entries skipped", []string{"-platform=vscode, ,,codespaces"}, []linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces}, false},
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
			opts, err := parseOptions(tt.args, io.Discard)
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
			opts, err := parseOptions(tt.args, io.Discard)
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

// dashPrefixes are the two ways a boolean flag can be spelled on the command line; the standard
// flag package accepts either, so every bare boolean flag is tested with both automatically instead
// of listing each variant as a separate table row.
var dashPrefixes = []string{"-", "--"}

func TestParseOptionsBoolFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		flag string
		get  func(Options) bool
	}{
		{"version", func(o Options) bool { return o.Version }},
		{"rules", func(o Options) bool { return o.ListRules }},
		{"init", func(o Options) bool { return o.Init }},
		{"merge-features", func(o Options) bool { return o.MergeFeatures }},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			t.Parallel()
			for _, prefix := range dashPrefixes {
				t.Run(prefix, func(t *testing.T) {
					t.Parallel()
					args := []string{prefix + tt.flag}
					opts, err := parseOptions(args, io.Discard)
					if err != nil {
						t.Fatalf("parseOptions(%v): %v", args, err)
					}
					if !tt.get(opts) {
						t.Errorf("%s = false, want true", tt.flag)
					}
				})
			}
			for _, want := range []bool{true, false} {
				name := fmt.Sprintf("=%v", want)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					args := []string{fmt.Sprintf("-%s=%v", tt.flag, want)}
					opts, err := parseOptions(args, io.Discard)
					if err != nil {
						t.Fatalf("parseOptions(%v): %v", args, err)
					}
					if got := tt.get(opts); got != want {
						t.Errorf("%s = %v, want %v", tt.flag, got, want)
					}
				})
			}
		})
	}
}

func TestParseOptionsMergeFeaturesSet(t *testing.T) {
	t.Parallel()

	// The value of MergeFeatures itself is covered by TestParseOptionsBoolFlags; this exercises
	// mergeFeaturesSet, the bookkeeping unique to this flag (see its doc comment in opts.go).
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no flag", nil, false},
		{"bare flag", []string{"-merge-features"}, true},
		{"explicit true", []string{"-merge-features=true"}, true},
		{"explicit false", []string{"-merge-features=false"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if opts.mergeFeaturesSet != tt.want {
				t.Errorf("mergeFeaturesSet = %v, want %v", opts.mergeFeaturesSet, tt.want)
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
			opts, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if opts.ConfigPath != tt.want {
				t.Errorf("ConfigPath = %q, want %q", opts.ConfigPath, tt.want)
			}
		})
	}
}

func TestParseOptionsPaths(t *testing.T) {
	t.Parallel()

	opts, err := parseOptions(nil, io.Discard)
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if diff := cmp.Diff([]string{"."}, opts.Paths); diff != "" {
		t.Errorf("Paths mismatch (-want +got):\n%s", diff)
	}
}
