package format

import (
	"fmt"
	"io"

	"github.com/bare-devcontainer/decolint/linter"
)

// TextFormat prints one line per issue, matching linter.Issue.String.
type TextFormat struct{}

// WriteIssues writes issues to w, one line per issue.
func (TextFormat) WriteIssues(w io.Writer, issues []linter.Issue) error {
	var numErrors, numWarnings int

	for _, issue := range issues {
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

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
