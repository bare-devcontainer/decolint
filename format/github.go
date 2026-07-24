package format

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// GitHubFormat prints one GitHub Actions workflow command (::error/::warning) per issue.
type GitHubFormat struct{}

// WriteIssues writes issues to w as GitHub Actions workflow commands. Paths are written with "/"
// separators, which is what GitHub matches an annotation against on every runner.
func (GitHubFormat) WriteIssues(w io.Writer, issues []linter.Issue) error {
	for _, issue := range issues {
		command := "error"
		if issue.Severity == linter.SeverityWarn {
			command = "warning"
		}
		_, err := fmt.Fprintf(
			w,
			"::%s file=%s,line=%d,col=%d,title=%s::%s\n",
			command,
			escapeGitHubProperty(filepath.ToSlash(issue.Path)),
			issue.Line,
			issue.Col,
			escapeGitHubProperty(issue.RuleID),
			escapeGitHubData(issue.Message),
		)
		if err != nil {
			return fmt.Errorf("write issue: %w", err)
		}
	}
	return nil
}

// githubDataReplacer escapes the data (message) portion of a GitHub Actions workflow command, per
// https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#escaping-properties.
// strings.Replacer is safe for concurrent use, so a single package-level instance is reused across calls.
var githubDataReplacer = strings.NewReplacer(
	"%", "%25",
	"\r", "%0D",
	"\n", "%0A",
)

// githubPropertyReplacer escapes a property value (e.g. file, title) of a GitHub Actions workflow
// command, per the same escaping rules as githubDataReplacer.
var githubPropertyReplacer = strings.NewReplacer(
	"%", "%25",
	"\r", "%0D",
	"\n", "%0A",
	":", "%3A",
	",", "%2C",
)

// escapeGitHubData escapes s for use as the data (message) portion of a GitHub Actions workflow
// command.
func escapeGitHubData(s string) string {
	return githubDataReplacer.Replace(s)
}

// escapeGitHubProperty escapes s for use as a property value (e.g. file, title) of a GitHub Actions
// workflow command.
func escapeGitHubProperty(s string) string {
	return githubPropertyReplacer.Replace(s)
}
