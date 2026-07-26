package format

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// GitHubFormat prints the linted files as a collapsible log group, then one GitHub Actions workflow
// command (::error/::warning) per issue.
type GitHubFormat struct{}

// WriteReport writes report to w as GitHub Actions workflow commands. Paths are written with "/"
// separators, which is what GitHub matches an annotation against.
//
// The linted files go in a "::group::" block rather than in annotations of their own, so that the
// run log gains the context without a file that has no finding gaining an annotation.
func (GitHubFormat) WriteReport(w io.Writer, report Report) error {
	if err := writeGitHubFileGroup(w, report.Files); err != nil {
		return err
	}
	for _, issue := range report.Issues {
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

// writeGitHubFileGroup writes files inside a collapsible log group, or nothing when no file was
// linted. Every line of the block is either a literal command or an indented path, so a path can
// never be read as a workflow command of its own.
func writeGitHubFileGroup(w io.Writer, files []File) error {
	if len(files) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "::group::decolint"); err != nil {
		return fmt.Errorf("write file group: %w", err)
	}
	// Workflow commands are parsed by the runner, not read off a terminal, so the block is written
	// undecorated.
	if err := writeFileList(w, styler{}, files); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "::endgroup::"); err != nil {
		return fmt.Errorf("write file group: %w", err)
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
