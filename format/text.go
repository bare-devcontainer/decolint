package format

import (
	"fmt"
	"io"

	"github.com/bare-devcontainer/decolint/linter"
)

// TextFormat prints a header naming the config file and the linted files, then one line per issue,
// matching linter.Issue.String, and a summary.
type TextFormat struct {
	// Color, when true, decorates the report with ANSI escape sequences: the severity of an issue is
	// colored, the position it is at stands out, and secondary details recede. Callers should enable
	// it only for a destination that renders them, i.e. a terminal.
	Color bool
}

// WriteReport writes report to w.
func (f TextFormat) WriteReport(w io.Writer, report Report) error {
	st := styler{enabled: f.Color}

	if err := writeConfigLine(w, st, report.ConfigPath); err != nil {
		return err
	}
	if err := writeFileList(w, st, report.Files); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	var numErrors, numWarnings int

	for _, issue := range report.Issues {
		if err := writeIssue(w, st, issue); err != nil {
			return err
		}

		switch issue.Severity {
		case linter.SeverityError:
			numErrors++
		case linter.SeverityWarn:
			numWarnings++
		}
	}

	return writeSummary(w, st, numErrors, numWarnings)
}

// writeIssue writes issue as one line. Undecorated, the line is the one [linter.Issue.String]
// returns; styling only wraps its parts.
func writeIssue(w io.Writer, st styler, issue linter.Issue) error {
	position := fmt.Sprintf("%s:%d:%d", issue.Path, issue.Line, issue.Col)
	_, err := fmt.Fprintf(
		w,
		"%s: %s: %s %s\n",
		st.bold(position),
		st.severity(issue.Severity, issue.Severity.String()),
		issue.Message,
		st.dim("("+issue.RuleID+")"),
	)
	if err != nil {
		return fmt.Errorf("write issue: %w", err)
	}
	return nil
}

// writeSummary writes the closing line counting what was found. A run with nothing to report is
// colored as a whole, so a clean result is recognizable without reading the counts.
func writeSummary(w io.Writer, st styler, numErrors, numWarnings int) error {
	errorCount := fmt.Sprintf("%d error%s", numErrors, pluralize(numErrors))
	warningCount := fmt.Sprintf("%d warning%s", numWarnings, pluralize(numWarnings))

	if numErrors > 0 {
		errorCount = st.severity(linter.SeverityError, errorCount)
	}
	if numWarnings > 0 {
		warningCount = st.severity(linter.SeverityWarn, warningCount)
	}
	line := fmt.Sprintf("Found %s and %s.", errorCount, warningCount)
	if numErrors == 0 && numWarnings == 0 {
		line = st.green(line)
	}

	if _, err := fmt.Fprintln(w, line); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

// writeConfigLine names the config file the run's settings came from. Running without one is a
// supported mode rather than a mistake, so an absent config file is reported as the defaults being
// in effect, with the command that generates one.
func writeConfigLine(w io.Writer, st styler, path string) error {
	line := st.bold("Config:") + " " + path
	if path == "" {
		line = st.bold("Config:") + " " + st.dim(`none (defaults; run "decolint --init" to create .decolint.jsonc)`)
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
