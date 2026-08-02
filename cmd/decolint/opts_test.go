package main

import (
	"fmt"
	"io"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/google/go-cmp/cmp"
)

func TestParseOptions_Platform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    []linter.Platform
		wantErr bool
	}{
		{"no flag", nil, nil, false},
		{"single platform", []string{"--platform=vscode"}, []linter.Platform{linter.PlatformVSCode}, false},
		{
			"multiple platforms",
			[]string{"--platform=vscode,codespaces"},
			[]linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces},
			false,
		},
		{"mixed case", []string{"--platform=VSCode"}, []linter.Platform{linter.PlatformVSCode}, false},
		// Empty entries from stray commas or surrounding whitespace are skipped, not rejected.
		{"empty entries skipped", []string{"--platform=vscode, ,,codespaces"}, []linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces}, false},
		{"unknown platform", []string{"--platform=bogus"}, nil, true},
		{
			"combined with other flags and paths",
			[]string{"--deny-warnings", "--platform=vscode", "."},
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

func TestParseOptions_Format(t *testing.T) {
	t.Parallel()

	// parseOptions captures the raw --format value verbatim; validation and resolution into a Format
	// happen later, in runLint, so both the flag and the config file's "format" member go through
	// one path (an invalid value is rejected there, see TestRun_Flags/"invalid --format value").
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no flag", nil, ""},
		{"text", []string{"--format=text"}, "text"},
		{"json", []string{"--format=json"}, "json"},
		{"github", []string{"--format=github"}, "github"},
		{"unrecognized value captured verbatim", []string{"--format=bogus"}, "bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if opts.Format != tt.want {
				t.Errorf("Format = %q, want %q", opts.Format, tt.want)
			}
		})
	}
}

func TestParseOptions_Color(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    colorMode
		wantErr bool
	}{
		{name: "no flag", want: colorAuto},
		{name: "auto", args: []string{"--color=auto"}, want: colorAuto},
		{name: "always", args: []string{"--color=always"}, want: colorAlways},
		{name: "never", args: []string{"--color=never"}, want: colorNever},
		{name: "unknown value is rejected", args: []string{"--color=bogus"}, wantErr: true},
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
			if opts.Color != tt.want {
				t.Errorf("Color = %v, want %v", opts.Color, tt.want)
			}
		})
	}
}

func TestParseOptions_BoolFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		flag string
		get  func(Options) bool
	}{
		{"version", func(o Options) bool { return o.Version }},
		{"rules", func(o Options) bool { return o.ListRules }},
		{"init", func(o Options) bool { return o.Init }},
		{"merge", func(o Options) bool { return o.Merge }},
		{"deny-warnings", func(o Options) bool { return o.DenyWarnings }},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			t.Parallel()
			t.Run("bare", func(t *testing.T) {
				t.Parallel()
				args := []string{"--" + tt.flag}
				opts, err := parseOptions(args, io.Discard)
				if err != nil {
					t.Fatalf("parseOptions(%v): %v", args, err)
				}
				if !tt.get(opts) {
					t.Errorf("%s = false, want true", tt.flag)
				}
			})
			for _, want := range []bool{true, false} {
				name := fmt.Sprintf("=%v", want)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					args := []string{fmt.Sprintf("--%s=%v", tt.flag, want)}
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

// TestParseOptions_Shorthands checks that each shorthand sets the same field as its long flag, and
// that a shorthand taking a value accepts it as the following argument.
func TestParseOptions_Shorthands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want Options
	}{
		{"config", []string{"-c", "custom.jsonc"}, Options{ConfigPath: "custom.jsonc"}},
		{"format", []string{"-f", "json"}, Options{Format: "json"}},
		{"platform", []string{"-p", "vscode"}, Options{Platforms: []linter.Platform{linter.PlatformVSCode}}},
		{"merge", []string{"-m"}, Options{Merge: true, mergeSet: true}},
		{"version", []string{"-v"}, Options{Version: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			want := tt.want
			want.Paths = []string{"."}
			if diff := cmp.Diff(want, opts, cmp.AllowUnexported(Options{})); diff != "" {
				t.Errorf("parseOptions(%v) mismatch (-want +got):\n%s", tt.args, diff)
			}
		})
	}
}

func TestParseOptions_MergeSet(t *testing.T) {
	t.Parallel()

	// The value of Merge itself is covered by TestParseOptions_BoolFlags; this exercises
	// mergeSet, the bookkeeping unique to this flag (see its doc comment in opts.go).
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no flag", nil, false},
		{"bare flag", []string{"--merge"}, true},
		{"explicit true", []string{"--merge=true"}, true},
		{"explicit false", []string{"--merge=false"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if opts.mergeSet != tt.want {
				t.Errorf("mergeSet = %v, want %v", opts.mergeSet, tt.want)
			}
		})
	}
}

func TestParseOptions_DenyWarningsSet(t *testing.T) {
	t.Parallel()

	// The value of DenyWarnings itself is covered by TestParseOptions_BoolFlags; this exercises
	// denyWarningsSet, the bookkeeping that lets an explicit --deny-warnings=false override the config
	// file's "denyWarnings": true (see its doc comment in opts.go).
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no flag", nil, false},
		{"bare flag", []string{"--deny-warnings"}, true},
		{"explicit true", []string{"--deny-warnings=true"}, true},
		{"explicit false", []string{"--deny-warnings=false"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tt.args, err)
			}
			if opts.denyWarningsSet != tt.want {
				t.Errorf("denyWarningsSet = %v, want %v", opts.denyWarningsSet, tt.want)
			}
		})
	}
}

func TestParseOptions_Config(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no flag", nil, ""},
		{"config flag", []string{"--config=path/to/config.jsonc"}, "path/to/config.jsonc"},
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

func TestParseOptions_Paths(t *testing.T) {
	t.Parallel()

	// Arguments are taken as given; resolving and deduplicating them is runLint's job (see
	// TestRunLint_DeduplicatesTargets).
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"no arguments lint the working directory", nil, []string{"."}},
		{"arguments are kept as named", []string{".", "/work/repo", "."}, []string{".", "/work/repo", "."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts, err := parseOptions(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions: %v", err)
			}
			if diff := cmp.Diff(tt.want, opts.Paths); diff != "" {
				t.Errorf("Paths mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
