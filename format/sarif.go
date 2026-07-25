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
}

// SARIFFormat prints a SARIF 2.1.0 log, suitable for upload to GitHub Code Scanning.
type SARIFFormat struct {
	// Version is the tool version recorded in the run.
	Version string
	// Rules is the rule catalog used to describe the rules referenced by issues.
	Rules []SARIFRule
}

const (
	sarifSchemaURI = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion   = "2.1.0"
	informationURI = "https://github.com/bare-devcontainer/decolint"
	// srcRootBaseID is the SARIF base id a relative path is reported against. An upload resolves it
	// to the root of the analyzed checkout.
	srcRootBaseID = "%SRCROOT%"
)

// The wire structs cover the subset of SARIF 2.1.0 decolint emits. Fields marshal in declaration
// order, so the output is deterministic.

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
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
	ID               string          `json:"id"`
	ShortDescription sarifMessage    `json:"shortDescription,omitzero"`
	Properties       sarifProperties `json:"properties,omitzero"`
}

type sarifProperties struct {
	Tags []string `json:"tags"`
}

type sarifMessage struct {
	Text string `json:"text"`
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
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitzero"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

// WriteIssues writes issues to w as a SARIF 2.1.0 log with a single run. It marshals into an
// in-memory buffer first so that a failure never leaves partial output on w.
//
// The run's rule catalog lists only the rules referenced by issues, sorted by rule ID; a rule
// missing from f.Rules is listed with its ID alone. Issue paths are reported as URIs; see
// [artifactLocationFor].
func (f SARIFFormat) WriteIssues(w io.Writer, issues []linter.Issue) error {
	catalog := make(map[string]SARIFRule, len(f.Rules))
	for _, r := range f.Rules {
		catalog[r.ID] = r
	}

	referenced := make(map[string]bool)
	for _, issue := range issues {
		referenced[issue.RuleID] = true
	}
	ids := slices.Sorted(maps.Keys(referenced))

	descriptors := make([]sarifRuleDescriptor, 0, len(ids))
	index := make(map[string]int, len(ids))
	for i, id := range ids {
		index[id] = i
		desc := sarifRuleDescriptor{ID: id}
		if r, ok := catalog[id]; ok {
			desc.ShortDescription = sarifMessage{Text: r.Description}
			desc.Properties = sarifProperties{Tags: []string{r.Category}}
		}
		descriptors = append(descriptors, desc)
	}

	results := make([]sarifResult, 0, len(issues))
	for _, issue := range issues {
		level := "error"
		if issue.Severity == linter.SeverityWarn {
			level = "warning"
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
					ArtifactLocation: artifactLocationFor(issue.Path),
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
			Results: results,
		}},
	}

	var buf bytes.Buffer
	if err := json.MarshalWrite(&buf, log); err != nil {
		return fmt.Errorf("marshal sarif: %w", err)
	}
	buf.WriteByte('\n')
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write issues: %w", err)
	}
	return nil
}

// artifactLocationFor returns the SARIF location of the file at path, whose members must be a URI
// rather than a filesystem path. A relative path is reported against [srcRootBaseID]; an absolute
// one, which decolint reports for a file outside the directory it runs in, becomes an absolute file
// URI, since resolving it against the source root would name a different file.
func artifactLocationFor(path string) sarifArtifactLocation {
	slashed := filepath.ToSlash(path)
	if !filepath.IsAbs(path) {
		return sarifArtifactLocation{URI: (&url.URL{Path: slashed}).String(), URIBaseID: srcRootBaseID}
	}
	// A Windows path is absolute without a leading "/", which a file URI's path needs.
	return sarifArtifactLocation{URI: (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(slashed, "/")}).String()}
}
