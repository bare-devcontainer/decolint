package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/bare-devcontainer/decolint/format"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
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
	case "sarif":
		return format.SARIFFormat{Version: version, Rules: sarifRules()}, nil
	default:
		return nil, fmt.Errorf("unknown format %q (want one of: text, json, github, sarif)", name)
	}
}

// sarifRules adapts the built-in rule catalog into the shape [format.SARIFFormat] consumes, so the
// format package does not depend on the rules package.
func sarifRules() []format.SARIFRule {
	regs := rules.Builtin()
	out := make([]format.SARIFRule, len(regs))
	for i, reg := range regs {
		out[i] = format.SARIFRule{
			ID:          reg.Rule.ID,
			Description: reg.Rule.Description,
			Category:    reg.Rule.Category.String(),
			HelpURI:     rules.DocsURL(reg.Rule.ID),
		}
	}
	return out
}
