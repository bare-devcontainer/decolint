package format

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// SARIFRule describes one rule for the SARIF rule catalog. It mirrors the [linter.Rule] fields the
// SARIF output needs, so this package does not depend on the rules package.
type SARIFRule struct {
	ID          string
	Description string
	Category    string
	// HelpURI is where the rule is documented in full.
	HelpURI string
}

// SARIFFormat prints a SARIF 2.1.0 log, suitable for upload to GitHub Code Scanning.
type SARIFFormat struct {
	// Version is the tool version recorded in the run.
	Version string
	// Rules are the rules the run has enabled. They are declared in the log whether or not they
	// produced a result, which is how a consumer tells a rule that ran clean from one that was
	// switched off.
	Rules []SARIFRule
}

const (
	sarifSchemaURI = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion   = "2.1.0"
	informationURI = "https://github.com/bare-devcontainer/decolint"
)

// The wire structs cover the subset of SARIF 2.1.0 decolint emits. Fields marshal in declaration
// order, so the output is deterministic.

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool      sarifTool       `json:"tool"`
	Artifacts []sarifArtifact `json:"artifacts"`
	Results   []sarifResult   `json:"results"`
}

type sarifArtifact struct {
	Location sarifArtifactLocation `json:"location"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string                `json:"name"`
	Version        string                `json:"version"`
	InformationURI string                `json:"informationUri"`
	Rules          []sarifRuleDescriptor `json:"rules"`
}

type sarifRuleDescriptor struct {
	ID               string                  `json:"id"`
	ShortDescription sarifMessage            `json:"shortDescription,omitzero"`
	HelpURI          string                  `json:"helpUri,omitzero"`
	Help             sarifMultiformatMessage `json:"help,omitzero"`
	Properties       sarifProperties         `json:"properties,omitzero"`
}

type sarifProperties struct {
	Tags []string `json:"tags"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

// sarifMultiformatMessage is SARIF's multiformatMessageString: a plain-text rendering plus an
// optional Markdown one, which viewers prefer when they can render it. Text is required whenever
// Markdown is present.
type sarifMultiformatMessage struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown,omitzero"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
	// Index points into the run's artifacts array. It is a pointer so that index 0, a valid
	// reference, is not mistaken for "unset" and omitted.
	Index *int `json:"index,omitzero"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

// WriteReport writes report to w as a SARIF 2.1.0 log with a single run. It marshals into an
// in-memory buffer first so that a failure never leaves partial output on w.
//
// The run's rule catalog lists f.Rules, sorted by rule ID, so the log declares the rules the run
// covered and not just the ones that fired; a rule referenced by an issue but missing from f.Rules
// is listed with its ID alone. The linted files become the run's artifacts, which each result
// refers to by index. Paths are reported as URIs; see [artifactURIFor].
func (f SARIFFormat) WriteReport(w io.Writer, report Report) error {
	catalog := make(map[string]SARIFRule, len(f.Rules))
	for _, r := range f.Rules {
		catalog[r.ID] = r
	}

	listed := make(map[string]bool, len(f.Rules)+len(report.Issues))
	for _, r := range f.Rules {
		listed[r.ID] = true
	}
	for _, issue := range report.Issues {
		listed[issue.RuleID] = true
	}
	ids := slices.Sorted(maps.Keys(listed))

	descriptors := make([]sarifRuleDescriptor, 0, len(ids))
	index := make(map[string]int, len(ids))
	for i, id := range ids {
		index[id] = i
		desc := sarifRuleDescriptor{ID: id}
		if r, ok := catalog[id]; ok {
			desc.ShortDescription = sarifMessage{Text: r.Description}
			desc.HelpURI = r.HelpURI
			desc.Help = sarifHelpFor(r)
			desc.Properties = sarifProperties{Tags: []string{r.Category}}
		}
		descriptors = append(descriptors, desc)
	}

	artifacts := make([]sarifArtifact, 0, len(report.Files))
	artifactIndex := make(map[string]int, len(report.Files))
	for i, file := range report.Files {
		artifactIndex[file.Path] = i
		artifacts = append(artifacts, sarifArtifact{
			Location: sarifArtifactLocation{URI: artifactURIFor(file.Path)},
		})
	}

	results := make([]sarifResult, 0, len(report.Issues))
	for _, issue := range report.Issues {
		level := "error"
		if issue.Severity == linter.SeverityWarn {
			level = "warning"
		}
		location := sarifArtifactLocation{URI: artifactURIFor(issue.Path)}
		if i, ok := artifactIndex[issue.Path]; ok {
			location.Index = &i
		}
		// startColumn is emitted as the issue's byte column even though SARIF defaults to UTF-16
		// code units; the two agree on ASCII lines, and alerts anchor by line regardless.
		results = append(results, sarifResult{
			RuleID:    issue.RuleID,
			RuleIndex: index[issue.RuleID],
			Level:     level,
			Message:   sarifMessage{Text: issue.Message},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: location,
					Region:           sarifRegion{StartLine: issue.Line, StartColumn: issue.Col},
				},
			}},
		})
	}

	log := sarifLog{
		Schema:  sarifSchemaURI,
		Version: sarifVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "decolint",
				Version:        f.Version,
				InformationURI: informationURI,
				Rules:          descriptors,
			}},
			Artifacts: artifacts,
			Results:   results,
		}},
	}

	var buf bytes.Buffer
	if err := json.MarshalWrite(&buf, log); err != nil {
		return fmt.Errorf("marshal sarif: %w", err)
	}
	buf.WriteByte('\n')
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// sarifHelpFor returns a link to the rule's documentation as SARIF help, the text a viewer shows
// when a reader asks why an alert was raised. GitHub Code Scanning renders the Markdown form on the
// alert but does not surface helpUri, so the link has to be in the message to be reachable. It is
// the zero value, and so omitted, for a rule with nowhere to link to.
func sarifHelpFor(r SARIFRule) sarifMultiformatMessage {
	if r.HelpURI == "" {
		return sarifMultiformatMessage{}
	}
	return sarifMultiformatMessage{
		Text:     "Documentation: " + r.HelpURI,
		Markdown: "[Rule documentation](" + r.HelpURI + ")",
	}
}

// artifactURIFor returns the SARIF location of the file at path, which must be a URI rather than a
// filesystem path. A relative path stays relative, for the consumer to resolve against the root of
// the analyzed project; an absolute one, which decolint reports for a file outside the directory it
// runs in, becomes an absolute file URI, since resolving it that way would name a different file.
func artifactURIFor(path string) string {
	slashed := filepath.ToSlash(path)
	if !filepath.IsAbs(path) {
		return (&url.URL{Path: slashed}).String()
	}
	// A UNC path names a host, which a file URI carries as its authority rather than in its path.
	if rest, ok := strings.CutPrefix(slashed, "//"); ok {
		host, share, _ := strings.Cut(rest, "/")
		return (&url.URL{Scheme: "file", Host: host, Path: "/" + share}).String()
	}
	// A Windows drive path is absolute without a leading "/", which a file URI's path needs.
	return (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(slashed, "/")}).String()
}
