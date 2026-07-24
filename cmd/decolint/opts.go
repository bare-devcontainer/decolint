package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// Options holds the parsed command-line arguments. It is purely the CLI's view of the world; see
// Config for the on-disk config file's shape, and mergeConfig for how the two are reconciled.
type Options struct {
	// Paths are the directories to lint.
	Paths []string
	// DenyWarnings mirrors [Config.DenyWarnings]. When -deny-warnings is explicitly given it takes
	// precedence over the config file's "denyWarnings" member, in either direction (see
	// denyWarningsSet and mergeConfig).
	DenyWarnings bool
	// denyWarningsSet records whether -deny-warnings was explicitly passed, so mergeConfig can tell
	// "not given" (defer to the config file) apart from "explicitly given as false" (override the
	// config file's "denyWarnings": true).
	denyWarningsSet bool
	// ConfigPath is the raw -config flag value (empty if not given), resolved into a Config by
	// loadConfig.
	ConfigPath string
	// Platforms restricts registered rules to those targeting one of these platforms, plus any rule
	// with no target platform. If empty, only rules with no target platform are registered, unless
	// overridden by the config file's "platforms" member (see mergeConfig).
	Platforms []linter.Platform
	// Merge mirrors [Config.Merge]. When -merge is explicitly given it takes precedence over the
	// config file's "merge" member, in either direction (see mergeSet and mergeConfig).
	Merge bool
	// mergeSet records whether -merge was explicitly passed, so mergeConfig can tell "not given"
	// (defer to the config file) apart from "explicitly given as false" (override the config file's
	// "merge": true).
	mergeSet bool
	// Format is the raw -format flag value ("" if not given), naming how lint issues are written to
	// stdout: "text", "json", or "github". A non-empty value replaces the config file's "format"
	// member; it is resolved into a Format by parseFormat in runLint.
	Format string
	// Version, when set, causes the program to print its version and exit.
	Version bool
	// ListRules, when set, causes the program to print the built-in rules and exit.
	ListRules bool
	// Init, when set, causes the program to write a new .decolint.jsonc config file listing every
	// rule at its default severity, then exit.
	Init bool
}

// parseOptions parses args into Options. Flag errors and usage text are written to output.
func parseOptions(args []string, output io.Writer) (Options, error) {
	var opts Options
	var platformFlag string
	var formatFlag string

	fs := flag.NewFlagSet(progName, flag.ContinueOnError)
	fs.SetOutput(output)
	fs.BoolVar(&opts.DenyWarnings, "deny-warnings", false, "treat warnings as failures (exit code 1); overrides the config file's \"denyWarnings\" member")
	fs.StringVar(&opts.ConfigPath, "config", "", "path to a config file (default: auto-discover .decolint.jsonc or .decolint.json in the current directory)")
	fs.StringVar(&platformFlag, "platform", "", "comma-separated target platforms to include in addition to \"all\" (vscode, codespaces); overrides the config file's \"platforms\" member")
	fs.StringVar(&formatFlag, "format", "", "output format: text (default), json, or github; overrides the config file's \"format\" member")
	fs.BoolVar(&opts.Merge, "merge", false, "lint the merged (effective) configuration, including referenced Features and base image metadata; overrides the config file's \"merge\" member")
	fs.BoolVar(&opts.Version, "version", false, "print version information and exit")
	fs.BoolVar(&opts.ListRules, "rules", false, "print the built-in rules as a Markdown table (category, target platforms, current severity), then exit")
	fs.BoolVar(&opts.Init, "init", false, "write a new .decolint.jsonc config file listing every rule at its default severity, then exit")
	fs.Usage = func() { _ = usage(fs) }
	if err := fs.Parse(args); err != nil {
		return Options{}, fmt.Errorf("parse flags: %w", err)
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "merge":
			opts.mergeSet = true
		case "deny-warnings":
			opts.denyWarningsSet = true
		}
	})

	platforms, err := parsePlatforms(platformFlag)
	if err != nil {
		return Options{}, err
	}
	opts.Platforms = platforms

	// The raw name is validated and resolved into a Format by parseFormat in runLint, after the
	// config file's "format" member is merged in, so both sources go through one validation path.
	opts.Format = formatFlag

	opts.Paths = dedupePaths(fs.Args())
	if len(opts.Paths) == 0 {
		opts.Paths = []string{"."}
	}

	return opts, nil
}

// dedupePaths drops arguments naming a directory an earlier argument already names, so that passing
// e.g. both "." and its absolute path lints it once rather than reporting every finding twice. The
// spelling kept is the first one given, since that is the one the findings are reported under. An
// argument whose location cannot be resolved is kept as given, leaving it for the lint to reject.
func dedupePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	kept := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err == nil {
			if _, ok := seen[abs]; ok {
				continue
			}
			seen[abs] = struct{}{}
		}
		kept = append(kept, p)
	}
	return kept
}

// parsePlatforms parses a comma-separated list of platform names into a slice of linter.Platform.
// An empty string yields a nil slice.
func parsePlatforms(s string) ([]linter.Platform, error) {
	if s == "" {
		return nil, nil
	}
	var platforms []linter.Platform
	for part := range strings.SplitSeq(s, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		p, err := linter.ParsePlatform(name)
		if err != nil {
			return nil, fmt.Errorf("parse platform %q: %w", name, err)
		}
		platforms = append(platforms, p)
	}
	return platforms, nil
}
