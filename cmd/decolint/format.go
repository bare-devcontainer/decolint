package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/bare-devcontainer/decolint/format"
	"github.com/bare-devcontainer/decolint/linter"
)

// Format identifies how lint issues are written to stdout.
type Format interface {
	// WriteIssues writes issues to w in this format.
	WriteIssues(w io.Writer, issues []linter.Issue) error
}

// parseFormat parses a format name, matched case-insensitively, into a Format. An empty string
// yields the text format. It returns an error if name does not name a known format.
func parseFormat(name string) (Format, error) {
	switch strings.ToLower(name) {
	case "", "text":
		return format.TextFormat{}, nil
	case "json":
		return format.JSONFormat{}, nil
	case "github":
		return format.GitHubFormat{}, nil
	default:
		return nil, fmt.Errorf("unknown format %q (want one of: text, json, github)", name)
	}
}
