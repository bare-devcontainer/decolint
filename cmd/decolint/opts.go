package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/spf13/pflag"
)

// Options holds the parsed command-line arguments. It is purely the CLI's view of the world; see
// Config for the on-disk config file's shape, and mergeConfig for how the two are reconciled.
type Options struct {
	// Paths are the directories to lint, as named on the command line; runLint resolves them.
	Paths []string
	// DenyWarnings mirrors [Config.DenyWarnings]. When --deny-warnings is explicitly given it takes
	// precedence over the config file's "denyWarnings" member, in either direction (see
	// denyWarningsSet and mergeConfig).
	DenyWarnings bool
	// denyWarningsSet records whether --deny-warnings was explicitly passed, so mergeConfig can tell
	// "not given" (defer to the config file) apart from "explicitly given as false" (override the
	// config file's "denyWarnings": true).
	denyWarningsSet bool
	// ConfigPath is the raw --config flag value (empty if not given), resolved into a Config by
	// loadConfig.
	ConfigPath string
	// Platforms restricts registered rules to those targeting one of these platforms, plus any rule
	// with no target platform. If empty, only rules with no target platform are registered, unless
	// overridden by the config file's "platforms" member (see mergeConfig).
	Platforms []linter.Platform
	// Merge mirrors [Config.Merge]. When --merge is explicitly given it takes precedence over the
	// config file's "merge" member, in either direction (see mergeSet and mergeConfig).
	Merge bool
	// mergeSet records whether --merge was explicitly passed, so mergeConfig can tell "not given"
	// (defer to the config file) apart from "explicitly given as false" (override the config file's
	// "merge": true).
	mergeSet bool
	// Format is the raw --format flag value ("" if not given), naming how lint issues are written to
	// stdout: "text", "json", "github", or "sarif". A non-empty value replaces the config file's
	// "format" member; it is resolved into a Format by parseFormat in runLint.
	Format string
	// Color is when the text output should be colored, from the --color flag. It is CLI-only: whether
	// escape sequences can be rendered depends on where decolint runs, not on the project it lints.
	Color colorMode
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
	var colorFlag string

	fs := pflag.NewFlagSet(progName, pflag.ContinueOnError)
	fs.SetOutput(output)
	fs.BoolVar(&opts.DenyWarnings, "deny-warnings", false, "treat warnings as failures (exit code 1); overrides the config file's \"denyWarnings\" member")
	fs.StringVarP(&opts.ConfigPath, "config", "c", "", "path to a config file (default: auto-discover .decolint.jsonc or .decolint.json in the current directory)")
	fs.StringVarP(&platformFlag, "platform", "p", "", "comma-separated target platforms to include in addition to \"all\" (vscode, codespaces); overrides the config file's \"platforms\" member")
	fs.StringVarP(&formatFlag, "format", "f", "", "output format: text (default), json, github, or sarif; overrides the config file's \"format\" member")
	fs.StringVar(&colorFlag, "color", "", "when to color the text output: auto (default; only when writing to a terminal), always, or never")
	fs.BoolVarP(&opts.Merge, "merge", "m", false, "lint the merged (effective) configuration, including referenced Features and base image metadata; overrides the config file's \"merge\" member")
	fs.BoolVarP(&opts.Version, "version", "v", false, "print version information and exit")
	fs.BoolVar(&opts.ListRules, "rules", false, "print the built-in rules as a Markdown table (category, target platforms, current severity), then exit")
	fs.BoolVar(&opts.Init, "init", false, "write a new .decolint.jsonc config file listing every rule at its default severity, then exit")
	fs.Usage = func() { _ = usage(fs) }
	if err := fs.Parse(args); err != nil {
		// pflag leaves it to the caller to report a parse error, unlike --help, which it reports
		// itself before returning [pflag.ErrHelp].
		if !errors.Is(err, pflag.ErrHelp) {
			_ = usage(fs)
		}
		return Options{}, fmt.Errorf("parse flags: %w", err)
	}
	opts.mergeSet = fs.Changed("merge")
	opts.denyWarningsSet = fs.Changed("deny-warnings")

	platforms, err := parsePlatforms(platformFlag)
	if err != nil {
		return Options{}, err
	}
	opts.Platforms = platforms

	// The raw name is validated and resolved into a Format by parseFormat in runLint, after the
	// config file's "format" member is merged in, so both sources go through one validation path.
	opts.Format = formatFlag

	color, err := parseColorMode(colorFlag)
	if err != nil {
		return Options{}, err
	}
	opts.Color = color

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
			return nil, fmt.Errorf("parse platform %q: %w", name, err)
		}
		platforms = append(platforms, p)
	}
	return platforms, nil
}
