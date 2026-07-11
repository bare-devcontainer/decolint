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
	for _, issue := range issues {
		if _, err := fmt.Fprintln(w, issue); err != nil {
			return err
		}
	}
	return nil
}
