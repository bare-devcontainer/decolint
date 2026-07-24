package format

import (
	"errors"
	"strings"
	"testing"
)

// testSARIFFormat returns a SARIFFormat whose catalog knows only one of the two rules in
// testIssues, so both the known- and unknown-rule descriptor paths are exercised.
func testSARIFFormat() SARIFFormat {
	return SARIFFormat{
		Version: "1.2.3",
		Rules: []SARIFRule{
			{
				ID:          "no-image-latest",
				Description: "images should be pinned to a specific version",
				Category:    "reproducibility",
			},
		},
	}
}

func TestSARIFWriteIssues(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := testSARIFFormat().WriteIssues(&sb, testIssues()); err != nil {
		t.Fatalf("WriteIssues: %v", err)
	}

	want := `{"$schema":"https://json.schemastore.org/sarif-2.1.0.json","version":"2.1.0","runs":[` +
		`{"tool":{"driver":{"name":"decolint","version":"1.2.3","informationUri":"https://github.com/bare-devcontainer/decolint","rules":[` +
		`{"id":"no-image-latest","shortDescription":{"text":"images should be pinned to a specific version"},"properties":{"tags":["reproducibility"]}},` +
		`{"id":"some-error-rule"}]}},"results":[` +
		`{"ruleId":"no-image-latest","ruleIndex":0,"level":"warning","message":{"text":"image \"ubuntu:latest\" uses the \"latest\" tag; pin a specific version"},` +
		`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".devcontainer/devcontainer.json","uriBaseId":"%SRCROOT%"},"region":{"startLine":4,"startColumn":12}}}]},` +
		`{"ruleId":"some-error-rule","ruleIndex":1,"level":"error","message":{"text":"something is broken"},` +
		`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".devcontainer/devcontainer.json","uriBaseId":"%SRCROOT%"},"region":{"startLine":8,"startColumn":3}}}]}]}]}` +
		"\n"
	if sb.String() != want {
		t.Errorf("WriteIssues sarif = %q, want %q", sb.String(), want)
	}
}

func TestSARIFWriteIssues_Empty(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := testSARIFFormat().WriteIssues(&sb, nil); err != nil {
		t.Fatalf("WriteIssues: %v", err)
	}

	want := `{"$schema":"https://json.schemastore.org/sarif-2.1.0.json","version":"2.1.0","runs":[` +
		`{"tool":{"driver":{"name":"decolint","version":"1.2.3","informationUri":"https://github.com/bare-devcontainer/decolint","rules":[]}},"results":[]}]}` +
		"\n"
	if sb.String() != want {
		t.Errorf("WriteIssues sarif (empty) = %q, want %q", sb.String(), want)
	}
}

func TestSARIFWriteIssues_WriteError(t *testing.T) {
	t.Parallel()

	if err := testSARIFFormat().WriteIssues(errWriter{}, testIssues()); !errors.Is(err, errWrite) {
		t.Errorf("WriteIssues error = %v, want %v", err, errWrite)
	}
}
