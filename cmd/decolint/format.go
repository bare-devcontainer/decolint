package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/bare-devcontainer/decolint/format"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

// Format identifies how a lint report is written to stdout.
type Format interface {
	// WriteReport writes report to w in this format.
	WriteReport(w io.Writer, report format.Report) error
}

// parseFormat resolves cfg.Format, matched case-insensitively, into a Format. An empty name yields
// the text format. color applies to that format alone: the machine-readable ones are consumed by
// other tools, which escape sequences would only corrupt. It returns an error if the name does not
// name a known format.
func parseFormat(cfg Config, color bool) (Format, error) {
	switch strings.ToLower(cfg.Format) {
	case "", "text":
		return format.TextFormat{Color: color}, nil
	case "json":
		return format.JSONFormat{}, nil
	case "github":
		return format.GitHubFormat{}, nil
	case "sarif":
		return format.SARIFFormat{Version: version, Rules: sarifRules(cfg)}, nil
	default:
		return nil, fmt.Errorf("unknown format %q (want one of: text, json, github, sarif)", cfg.Format)
	}
}

// sarifRules adapts the rules cfg enables into the shape [format.SARIFFormat] consumes, so the
// format package does not depend on the rules package.
func sarifRules(cfg Config) []format.SARIFRule {
	regs := rules.Enabled(cfg.Platforms, rules.Overrides{Categories: cfg.Categories, Rules: cfg.Rules})
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

// ruleDocs adapts every built-in rule, and the severity cfg currently gives it, into the shape
// "decolint -rules -format=json" prints ([format.RuleDoc]), so the format package itself does not
// need to depend on the rules package to describe it.
func ruleDocs(cfg Config) []format.RuleDoc {
	overrides := rules.Overrides{Categories: cfg.Categories, Rules: cfg.Rules}
	builtin := rules.Builtin()
	out := make([]format.RuleDoc, len(builtin))
	for i, reg := range builtin {
		out[i] = format.RuleDoc{
			ID:              reg.Rule.ID,
			Description:     reg.Rule.Description,
			LongDescription: reg.Rule.LongDescription,
			References:      reg.Rule.References,
			Category:        reg.Rule.Category.String(),
			Platforms:       platformStrings(reg.Rule.Platforms),
			FileTypes:       fileTypeStrings(reg.Rule.FileTypes),
			Example:         ruleExample(reg.Rule.Example),
			DocsURL:         rules.DocsURL(reg.Rule.ID),
			Severity:        overrides.SeverityFor(reg).String(),
		}
	}
	return out
}

func platformStrings(platforms []linter.Platform) []string {
	out := make([]string, len(platforms))
	for i, p := range platforms {
		out[i] = p.String()
	}
	return out
}

func fileTypeStrings(fileTypes []linter.FileType) []string {
	out := make([]string, len(fileTypes))
	for i, ft := range fileTypes {
		out[i] = string(ft)
	}
	return out
}

func ruleExample(ex linter.Example) format.RuleExample {
	return format.RuleExample{
		Bad:  ruleSnippet(ex.Bad),
		Good: ruleSnippet(ex.Good),
		Note: ex.Note,
	}
}

func ruleSnippet(s linter.Snippet) format.RuleSnippet {
	files := make([]format.RuleExampleFile, len(s.Files))
	for i, f := range s.Files {
		files[i] = format.RuleExampleFile{Path: f.Path, Content: f.Content, Mode: uint32(f.Mode.Perm())}
	}
	return format.RuleSnippet{Files: files, DirName: s.DirName}
}
