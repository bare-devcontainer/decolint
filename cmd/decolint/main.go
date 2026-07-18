// Command decolint lints devcontainer configuration files (devcontainer.json and friends).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bare-devcontainer/decolint/discovery"
	"github.com/bare-devcontainer/decolint/feature"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

// progName is the program name, used in the flag set, usage text, and error messages.
const progName = "decolint"

// failThreshold is the default lowest severity that causes exit code 1.
const failThreshold = linter.SeverityError

const (
	exitCodeSuccess     = 0
	exitCodeIssuesFound = 1
	exitCodeError       = 2
)

// version and revision identify the build: the decolint version and the git commit hash it was built
// from. Both are overridden via -ldflags at build time (see the Makefile); when built with plain
// "go build" or "go run" they stay at these defaults.
var (
	version  = "dev"
	revision = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable body of main: it parses args, executes the lint, writes all output to the
// given writers, and returns the process exit code (0 = clean, 1 = issues found, 2 = usage or
// runtime error).
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitCodeSuccess
		}
		_, _ = fmt.Fprintln(stderr, progName+":", err)
		return exitCodeError
	}

	if opts.Version {
		_, _ = fmt.Fprintln(stdout, versionString())
		return exitCodeSuccess
	}

	if opts.Init {
		if err := initConfigFile(stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, progName+":", err)
			return exitCodeError
		}
		return exitCodeSuccess
	}

	cfg, err := loadConfig(opts.ConfigPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, progName+":", err)
		return exitCodeError
	}
	cfg = mergeConfig(opts, cfg)

	if opts.ListRules {
		if err := listRules(stdout, cfg); err != nil {
			_, _ = fmt.Fprintln(stderr, progName+":", err)
			return exitCodeError
		}
		return exitCodeSuccess
	}

	hasIssue, err := runLint(ctx, stdout, opts, cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, progName+":", err)
		return exitCodeError
	}
	if hasIssue {
		return exitCodeIssuesFound
	}
	return exitCodeSuccess
}

func versionString() string {
	return fmt.Sprintf("%s %s (revision %s)", progName, version, revision)
}

// severityEmoji renders a severity for the -rules table; see the legend printed above the table.
var severityEmoji = map[linter.Severity]string{
	linter.SeverityOff:   "",
	linter.SeverityWarn:  "🟡 WARN",
	linter.SeverityError: "🔴 ERROR",
}

// rulesTableHeader is the header row of the -rules Markdown table.
var rulesTableHeader = []string{"Rule ID", "Category", "Platform", "Current"}

// listRules writes a Markdown table of the built-in rules to output: each rule's ID, category,
// target platforms (or "(all)"), and current severity (its category's default, overridden by cfg
// if any), in the order rules.Builtin returns them. A rule's default severity is not listed
// separately since it is uniform within a category; see the README's Rule categories section.
// Columns are padded to a common width so the raw Markdown source itself reads as an aligned table.
func listRules(output io.Writer, cfg Config) error {
	overrides := rules.Overrides{Categories: cfg.Categories, Rules: cfg.Rules}
	rows := [][]string{rulesTableHeader}
	for _, reg := range rules.Builtin() {
		platforms := "(all)"
		if len(reg.Rule.Platforms) > 0 {
			names := make([]string, len(reg.Rule.Platforms))
			for i, p := range reg.Rule.Platforms {
				names[i] = p.String()
			}
			platforms = strings.Join(names, ",")
		}
		rows = append(rows, []string{
			reg.Rule.ID,
			reg.Rule.Category.String(),
			platforms,
			severityEmoji[overrides.SeverityFor(reg)],
		})
	}
	widths := columnWidths(rows)

	if err := writeTableRow(output, rows[0], widths); err != nil {
		return err
	}
	separator := make([]string, len(widths))
	for i, w := range widths {
		separator[i] = strings.Repeat("-", w)
	}
	if err := writeTableRow(output, separator, widths); err != nil {
		return err
	}
	for _, row := range rows[1:] {
		if err := writeTableRow(output, row, widths); err != nil {
			return err
		}
	}
	return nil
}

// columnWidths returns, for each column in rows, the display width of its widest cell, so every
// row can be padded to a common set of column widths.
func columnWidths(rows [][]string) []int {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if w := displayWidth(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// writeTableRow writes cells as a Markdown table row, padding each cell with trailing spaces to
// its column's width so the raw table source is aligned.
func writeTableRow(output io.Writer, cells []string, widths []int) error {
	padded := make([]string, len(cells))
	for i, cell := range cells {
		padded[i] = cell + strings.Repeat(" ", widths[i]-displayWidth(cell))
	}
	if _, err := fmt.Fprintf(output, "| %s |\n", strings.Join(padded, " | ")); err != nil {
		return fmt.Errorf("write table row: %w", err)
	}
	return nil
}

// displayWidth estimates s's width in terminal columns. Plain ASCII/Latin text is single-width;
// symbols and emoji (used for severities in the -rules table) render double-width in virtually
// every terminal, even though they're each a single rune, so utf8.RuneCountInString undercounts
// them and throws off column padding. Variation selectors (e.g. U+FE0F, which requests the emoji
// presentation of the preceding rune) contribute no width of their own.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r == 0xFE0E || r == 0xFE0F:
			// no width
		case r >= 0x1100:
			w += 2
		default:
			w++
		}
	}
	return w
}

func usage(fs *flag.FlagSet) error {
	if _, err := io.WriteString(fs.Output(), fmt.Sprintf(`%s

usage: %s [directory ...]

Lints devcontainer configuration files. Each directory is detected
as a dev container definition, a Feature, or a Template based on
its layout, and the configuration files it contains are linted.
Defaults to the current directory.

Flags:
`, versionString(), fs.Name())); err != nil {
		return fmt.Errorf("write usage: %w", err)
	}

	fs.PrintDefaults()
	return nil
}

func runLint(ctx context.Context, stdout io.Writer, opts Options, cfg Config) (bool, error) {
	threshold := failThreshold
	if opts.DenyWarnings {
		threshold = linter.SeverityWarn
	}

	l := linter.New()
	overrides := rules.Overrides{Categories: cfg.Categories, Rules: cfg.Rules}
	if err := rules.RegisterRules(l, cfg.Platforms, overrides); err != nil {
		return false, fmt.Errorf("register rules: %w", err)
	}
	var merge mergeFn
	if cfg.MergeFeatures {
		// One Fetcher per run, so a Feature shared by several files is fetched at most once.
		fetcher := feature.NewFetcher()
		merge = func(ctx context.Context, f discovery.ConfigFile, doc *linter.Document) error {
			return feature.Merge(ctx, fetcher, f.Root, filepath.Dir(f.Path), doc.Tree())
		}
	}

	var allIssues []linter.Issue
	var worstSeverity linter.Severity
	var lintErr error
	for _, path := range opts.Paths {
		issues, err := lintPath(ctx, l, merge, path)
		for _, issue := range issues {
			if issue.Severity > worstSeverity {
				worstSeverity = issue.Severity
			}
		}
		allIssues = append(allIssues, issues...)
		if err != nil {
			lintErr = errors.Join(lintErr, err)
		}
	}

	if err := opts.Format.WriteIssues(stdout, allIssues); err != nil {
		return false, errors.Join(lintErr, fmt.Errorf("write issues: %w", err))
	}
	if lintErr != nil {
		return false, lintErr
	}

	return worstSeverity >= threshold, nil
}

// mergeFn merges Feature-contributed properties into doc's tree before rules run; see lintFile.
type mergeFn func(ctx context.Context, f discovery.ConfigFile, doc *linter.Document) error

// lintPath lints the devcontainer directory at path. The directory is opened as an os.Root, so
// every file the lint reads is confined to it. It is an error if path is not a directory.
func lintPath(ctx context.Context, l *linter.Linter, merge mergeFn, path string) ([]linter.Issue, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("resolve directory %s: %w", path, err)
	}
	// The root is only read from, so a close error is inconsequential.
	defer func() { _ = root.Close() }()
	issues, err := lintDir(ctx, l, merge, root)
	if err != nil {
		return nil, fmt.Errorf("lint directory %s: %w", path, err)
	}
	return issues, nil
}

// lintDir lints every configuration file in the devcontainer directory root is opened on. It is an
// error if the directory contains no configuration. Issue paths are the files' locations joined
// onto root's name.
func lintDir(ctx context.Context, l *linter.Linter, merge mergeFn, root *os.Root) ([]linter.Issue, error) {
	dir := root.Name()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("aborted %s: %w", dir, err)
	}
	var issues []linter.Issue
	var errs []error
	found := false
	err := discovery.VisitConfigs(root, func(f discovery.ConfigFile) error {
		found = true
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("aborted %s: %w", filepath.Join(f.Root.Name(), f.Path), err)
		}
		fileIssues, err := lintFile(ctx, l, merge, f)
		if err != nil {
			// A broken file must not stop the remaining files from being linted, so record the
			// error and keep visiting.
			errs = append(errs, err)
			return nil
		}
		issues = append(issues, fileIssues...)
		return nil
	})
	if err != nil {
		return issues, errors.Join(append(errs, err)...)
	}
	if !found {
		return nil, fmt.Errorf("no devcontainer configuration found in %s", dir)
	}
	return issues, errors.Join(errs...)
}

// lintFile reads and lints the single configuration file f, reporting issues under
// filepath.Join(f.Root.Name(), f.Path). The file is read through f.Root, so its resolution cannot
// escape that boundary. merge, if non-nil, runs on dev container configurations before rules and is
// skipped when no rule applies to f's type, so a file with no active rules does no Feature fetches.
func lintFile(ctx context.Context, l *linter.Linter, merge mergeFn, f discovery.ConfigFile) ([]linter.Issue, error) {
	display := filepath.Join(f.Root.Name(), f.Path)
	src, err := f.Root.ReadFile(f.Path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", display, err)
	}
	doc, err := linter.ParseDocument(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", display, err)
	}
	if merge != nil && f.Type == linter.Devcontainer && l.HasRules(f.Type) {
		if err := merge(ctx, f, doc); err != nil {
			return nil, fmt.Errorf("merge features %s: %w", display, err)
		}
	}
	return l.LintDocument(display, f.Type, doc), nil
}
