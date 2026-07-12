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
	"strings"
	"syscall"

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
	opts, configPath, err := parseOptions(args, stderr)
	if err != nil {
		if err == flag.ErrHelp {
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

	cfg, err := loadConfig(configPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, progName+":", err)
		return exitCodeError
	}
	opts.Config = cfg

	if opts.ListRules {
		if err := listRules(stdout, opts.Config); err != nil {
			_, _ = fmt.Fprintln(stderr, progName+":", err)
			return exitCodeError
		}
		return exitCodeSuccess
	}

	hasIssue, err := runLint(ctx, stdout, opts)
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
var rulesTableHeader = []string{"Rule ID", "Platform", "Default", "Current"}

// listRules writes a Markdown table of the built-in rules to output: each rule's ID, target
// platforms (or "(all)"), default severity, and current severity (the default overridden by cfg,
// if any), in the order rules.Builtin returns them. Columns are padded to a common width so the
// raw Markdown source itself reads as an aligned table.
func listRules(output io.Writer, cfg Config) error {
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
		current := reg.DefaultSeverity
		if s, ok := cfg.Rules[reg.Rule.ID]; ok {
			current = s
		}
		rows = append(rows, []string{
			reg.Rule.ID,
			platforms,
			severityEmoji[reg.DefaultSeverity],
			severityEmoji[current],
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
	_, err := fmt.Fprintf(output, "| %s |\n", strings.Join(padded, " | "))
	return err
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
		return err
	}

	fs.PrintDefaults()
	return nil
}

func runLint(ctx context.Context, stdout io.Writer, opts Options) (bool, error) {
	threshold := failThreshold
	if opts.DenyWarnings {
		threshold = linter.SeverityWarn
	}

	l := linter.New()
	if err := rules.RegisterRules(l, opts.Platforms, opts.Config.Rules); err != nil {
		return false, fmt.Errorf("register rules: %w", err)
	}

	var allIssues []linter.Issue
	var worstSeverity linter.Severity
	var lintErr error
	for _, path := range opts.Paths {
		issues, err := l.LintDir(ctx, path)
		for _, issue := range issues {
			if issue.Severity > worstSeverity {
				worstSeverity = issue.Severity
			}
		}
		allIssues = append(allIssues, issues...)
		if err != nil {
			lintErr = errors.Join(lintErr, fmt.Errorf("lint %s: %w", path, err))
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
