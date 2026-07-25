package format

import (
	"errors"
	"path/filepath"
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

// TestSARIFWriteIssues_ArtifactLocation checks how a finding's path becomes a SARIF artifact
// location: a path built with the host's separators is reported against the source root with "/"
// ones, while an absolute path — which decolint reports for a file outside the directory it runs in
// — is reported as a file URI that no base id applies to.
func TestSARIFWriteIssues_ArtifactLocation(t *testing.T) {
	t.Parallel()

	t.Run("relative path", func(t *testing.T) {
		t.Parallel()

		issues := testIssues()[:1]
		issues[0].Path = filepath.Join(".devcontainer", "go", "devcontainer.json")

		var sb strings.Builder
		if err := testSARIFFormat().WriteIssues(&sb, issues); err != nil {
			t.Fatalf("WriteIssues: %v", err)
		}

		want := `"artifactLocation":{"uri":".devcontainer/go/devcontainer.json","uriBaseId":"%SRCROOT%"}`
		if !strings.Contains(sb.String(), want) {
			t.Errorf("WriteIssues sarif = %q, want it to contain %q", sb.String(), want)
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		t.Parallel()

		issues := testIssues()[:1]
		issues[0].Path = filepath.Join(t.TempDir(), "devcontainer.json")

		var sb strings.Builder
		if err := testSARIFFormat().WriteIssues(&sb, issues); err != nil {
			t.Fatalf("WriteIssues: %v", err)
		}

		// The URI's own path varies with the host, so only its form is asserted: a file URI with an
		// empty authority, and no base id to resolve it against.
		out := sb.String()
		if !strings.Contains(out, `"artifactLocation":{"uri":"file:///`) {
			t.Errorf("WriteIssues sarif = %q, want an absolute file URI", out)
		}
		if strings.Contains(out, srcRootBaseID) {
			t.Errorf("WriteIssues sarif = %q, want no %s for an absolute path", out, srcRootBaseID)
		}
	})
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
