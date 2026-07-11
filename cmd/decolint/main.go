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
	"syscall"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

// progName is the program name, used in the flag set, usage text, and error messages.
const progName = "decolint"

// failThreshold is the default lowest severity that causes exit code 1.
const failThreshold = linter.Error

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

	cfg, err := loadConfig(configPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, progName+":", err)
		return exitCodeError
	}
	opts.Config = cfg

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
		threshold = linter.Warn
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
