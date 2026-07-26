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
				HelpURI:     "https://example.invalid/rules/no-image-latest/",
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
		`{"id":"no-image-latest","shortDescription":{"text":"images should be pinned to a specific version"},` +
		`"helpUri":"https://example.invalid/rules/no-image-latest/",` +
		`"help":{"text":"Documentation: https://example.invalid/rules/no-image-latest/",` +
		`"markdown":"[Rule documentation](https://example.invalid/rules/no-image-latest/)"},` +
		`"properties":{"tags":["reproducibility"]}},` +
		`{"id":"some-error-rule"}]}},"results":[` +
		`{"ruleId":"no-image-latest","ruleIndex":0,"level":"warning","message":{"text":"image \"ubuntu:latest\" uses the \"latest\" tag; pin a specific version"},` +
		`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".devcontainer/devcontainer.json"},"region":{"startLine":4,"startColumn":12}}}]},` +
		`{"ruleId":"some-error-rule","ruleIndex":1,"level":"error","message":{"text":"something is broken"},` +
		`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".devcontainer/devcontainer.json"},"region":{"startLine":8,"startColumn":3}}}]}]}]}` +
		"\n"
	if sb.String() != want {
		t.Errorf("WriteIssues sarif = %q, want %q", sb.String(), want)
	}
}

// TestSARIFWriteIssues_ArtifactLocation checks how a finding's path becomes a SARIF artifact
// location: a path built with the host's separators stays relative but is written with "/" ones,
// while an absolute path — which decolint reports for a file outside the directory it runs in — is
// written as a file URI.
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

		want := `"artifactLocation":{"uri":".devcontainer/go/devcontainer.json"}`
		if !strings.Contains(sb.String(), want) {
			t.Errorf("WriteIssues sarif = %q, want it to contain %q", sb.String(), want)
		}
	})

	t.Run("UNC path", func(t *testing.T) {
		t.Parallel()

		// A UNC path names its host in the leading "//" segment, which belongs in the URI's authority
		// rather than its path. The path is given in the form filepath.ToSlash yields for it on
		// Windows, which is absolute on every host, so this holds off Windows too.
		issues := testIssues()[:1]
		issues[0].Path = "//server/share/devcontainer.json"

		var sb strings.Builder
		if err := testSARIFFormat().WriteIssues(&sb, issues); err != nil {
			t.Fatalf("WriteIssues: %v", err)
		}

		want := `"artifactLocation":{"uri":"file://server/share/devcontainer.json"}`
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
		// empty authority.
		out := sb.String()
		if !strings.Contains(out, `"artifactLocation":{"uri":"file:///`) {
			t.Errorf("WriteIssues sarif = %q, want an absolute file URI", out)
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

// TestSARIFWriteIssues_RuleHelp checks how a rule's documentation address becomes the descriptor's
// helpUri and help, including that a rule with nowhere to link to carries neither field.
func TestSARIFWriteIssues_RuleHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rule        SARIFRule
		want        []string
		wantOmitted []string
	}{
		{
			name: "documented rule",
			rule: SARIFRule{ID: "some-error-rule", HelpURI: "https://example.invalid/rules/some-error-rule/"},
			want: []string{
				`"helpUri":"https://example.invalid/rules/some-error-rule/"`,
				`"help":{"text":"Documentation: https://example.invalid/rules/some-error-rule/",` +
					`"markdown":"[Rule documentation](https://example.invalid/rules/some-error-rule/)"}`,
			},
		},
		{
			name:        "no documentation address",
			rule:        SARIFRule{ID: "some-error-rule", Description: "short only"},
			want:        []string{`"shortDescription":{"text":"short only"}`},
			wantOmitted: []string{`"helpUri"`, `"help"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := SARIFFormat{Version: "1.2.3", Rules: []SARIFRule{tt.rule}}
			var sb strings.Builder
			if err := f.WriteIssues(&sb, testIssues()[1:]); err != nil {
				t.Fatalf("WriteIssues: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(sb.String(), want) {
					t.Errorf("WriteIssues sarif = %q, want it to contain %q", sb.String(), want)
				}
			}
			for _, omitted := range tt.wantOmitted {
				if strings.Contains(sb.String(), omitted) {
					t.Errorf("WriteIssues sarif = %q, want it to omit %q", sb.String(), omitted)
				}
			}
		})
	}
}

func TestSARIFWriteIssues_WriteError(t *testing.T) {
	t.Parallel()

	if err := testSARIFFormat().WriteIssues(errWriter{}, testIssues()); !errors.Is(err, errWrite) {
		t.Errorf("WriteIssues error = %v, want %v", err, errWrite)
	}
}
