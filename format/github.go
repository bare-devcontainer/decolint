package format

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// GitHubFormat prints one GitHub Actions workflow command (::error/::warning) per issue.
type GitHubFormat struct {
	// BaseDir is the directory absolute issue paths are made relative to; see [annotationPath]. It
	// may be empty, leaving such paths as they are.
	BaseDir string
}

// WriteIssues writes issues to w as GitHub Actions workflow commands.
func (f GitHubFormat) WriteIssues(w io.Writer, issues []linter.Issue) error {
	for _, issue := range issues {
		command := "error"
		if issue.Severity == linter.SeverityWarn {
			command = "warning"
		}
		_, err := fmt.Fprintf(
			w,
			"::%s file=%s,line=%d,col=%d,title=%s::%s\n",
			command,
			escapeGitHubProperty(annotationPath(issue.Path, f.BaseDir)),
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

// annotationPath renders path for a workflow command's "file" property. GitHub places an annotation
// by resolving that property against the repository checkout, so an absolute path lands nowhere: one
// inside baseDir is rewritten relative to it, and separators are normalized to "/", which is what
// GitHub matches on every runner. A path outside baseDir is written as is, since there is nothing
// better to say about it.
func annotationPath(path, baseDir string) string {
	if baseDir != "" && filepath.IsAbs(path) {
		if rel, err := filepath.Rel(baseDir, path); err == nil && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			path = rel
		}
	}
	return filepath.ToSlash(path)
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
