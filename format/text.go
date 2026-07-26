package format

import (
	"fmt"
	"io"

	"github.com/bare-devcontainer/decolint/linter"
)

// TextFormat prints a header naming the config file and the linted files, then one line per issue,
// matching linter.Issue.String, and a summary.
type TextFormat struct{}

// WriteReport writes report to w.
func (TextFormat) WriteReport(w io.Writer, report Report) error {
	if err := writeConfigLine(w, report.ConfigPath); err != nil {
		return err
	}
	if err := writeFileList(w, report.Files); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	var numErrors, numWarnings int

	for _, issue := range report.Issues {
		if _, err := fmt.Fprintln(w, issue); err != nil {
			return fmt.Errorf("write issue: %w", err)
		}

		switch issue.Severity {
		case linter.SeverityError:
			numErrors++
		case linter.SeverityWarn:
			numWarnings++
		}
	}

	if _, err := fmt.Fprintf(w, "Found %d error%s and %d warning%s.\n", numErrors, pluralize(numErrors), numWarnings, pluralize(numWarnings)); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	return nil
}

// writeConfigLine names the config file the run's settings came from. Running without one is a
// supported mode rather than a mistake, so an absent config file is reported as the defaults being
// in effect, with the command that generates one.
func writeConfigLine(w io.Writer, path string) error {
	line := "Config: " + path
	if path == "" {
		line = `Config: none (defaults; run "decolint -init" to create .decolint.jsonc)`
	}
	if _, err := fmt.Fprintln(w, line); err != nil {
		return fmt.Errorf("write config line: %w", err)
	}
	return nil
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
