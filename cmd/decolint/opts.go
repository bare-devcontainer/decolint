package main

import (
	"flag"
	"io"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// Options holds the parsed command-line arguments. It is purely the CLI's view of the world; see
// Config for the on-disk config file's shape, and mergeConfig for how the two are reconciled.
type Options struct {
	// Paths are the directories to lint.
	Paths []string
	// DenyWarnings, when set, lowers the fail threshold to linter.Warn so that warnings also cause
	// exit code 1.
	DenyWarnings bool
	// ConfigPath is the raw -config flag value (empty if not given), resolved into a Config by
	// loadConfig.
	ConfigPath string
	// Platforms restricts registered rules to those targeting one of these platforms, plus any rule
	// with no target platform. If empty, only rules with no target platform are registered, unless
	// overridden by the config file's "platforms" member (see mergeConfig).
	Platforms []linter.Platform
	// Format selects how lint issues are written to stdout.
	Format Format
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
	fs.BoolVar(&opts.DenyWarnings, "deny-warnings", false, "treat warnings as failures (exit code 1)")
	fs.StringVar(&opts.ConfigPath, "config", "", "path to a config file (default: auto-discover .decolint.jsonc or .decolint.json in the current directory)")
	fs.StringVar(&platformFlag, "platform", "", "comma-separated target platforms to include in addition to \"all\" (vscode, codespaces); overrides the config file's \"platforms\" member")
	fs.StringVar(&formatFlag, "format", "text", "output format: text, json, or github")
	fs.BoolVar(&opts.Version, "version", false, "print version information and exit")
	fs.BoolVar(&opts.ListRules, "rules", false, "print the built-in rules as a Markdown table (category, target platforms, current severity), then exit")
	fs.BoolVar(&opts.Init, "init", false, "write a new .decolint.jsonc config file listing every rule at its default severity, then exit")
	fs.Usage = func() { _ = usage(fs) }
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	platforms, err := parsePlatforms(platformFlag)
	if err != nil {
		return Options{}, err
	}
	opts.Platforms = platforms

	format, err := parseFormat(formatFlag)
	if err != nil {
		return Options{}, err
	}
	opts.Format = format

	opts.Paths = fs.Args()
	if len(opts.Paths) == 0 {
		opts.Paths = []string{"."}
	}

	return opts, nil
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
			return nil, err
		}
		platforms = append(platforms, p)
	}
	return platforms, nil
}
