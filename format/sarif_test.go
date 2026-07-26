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

func TestSARIFWriteReport(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := testSARIFFormat().WriteReport(&sb, testReport()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	want := `{"$schema":"https://json.schemastore.org/sarif-2.1.0.json","version":"2.1.0","runs":[` +
		`{"tool":{"driver":{"name":"decolint","version":"1.2.3","informationUri":"https://github.com/bare-devcontainer/decolint","rules":[` +
		`{"id":"no-image-latest","shortDescription":{"text":"images should be pinned to a specific version"},` +
		`"helpUri":"https://example.invalid/rules/no-image-latest/",` +
		`"help":{"text":"Documentation: https://example.invalid/rules/no-image-latest/",` +
		`"markdown":"[Rule documentation](https://example.invalid/rules/no-image-latest/)"},` +
		`"properties":{"tags":["reproducibility"]}},` +
		`{"id":"some-error-rule"}]}},` +
		`"artifacts":[{"location":{"uri":".devcontainer/devcontainer.json"}},{"location":{"uri":"src/devcontainer-feature.json"}}],` +
		`"results":[` +
		`{"ruleId":"no-image-latest","ruleIndex":0,"level":"warning","message":{"text":"image \"ubuntu:latest\" uses the \"latest\" tag; pin a specific version"},` +
		`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".devcontainer/devcontainer.json","index":0},"region":{"startLine":4,"startColumn":12}}}]},` +
		`{"ruleId":"some-error-rule","ruleIndex":1,"level":"error","message":{"text":"something is broken"},` +
		`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".devcontainer/devcontainer.json","index":0},"region":{"startLine":8,"startColumn":3}}}]}]}]}` +
		"\n"
	if sb.String() != want {
		t.Errorf("WriteReport sarif = %q, want %q", sb.String(), want)
	}
}

// TestSARIFWriteReport_ArtifactLocation checks how a path becomes a SARIF artifact location: a path
// built with the host's separators stays relative but is written with "/" ones, while an absolute
// path — which decolint reports for a file outside the directory it runs in — is written as a file
// URI.
func TestSARIFWriteReport_ArtifactLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "relative path",
			path: filepath.Join(".devcontainer", "go", "devcontainer.json"),
			want: `"artifactLocation":{"uri":".devcontainer/go/devcontainer.json","index":0}`,
		},
		{
			// A UNC path names its host in the leading "//" segment, which belongs in the URI's
			// authority rather than its path. The path is given in the form filepath.ToSlash yields
			// for it on Windows, which is absolute on every host, so this holds off Windows too.
			name: "UNC path",
			path: "//server/share/devcontainer.json",
			want: `"artifactLocation":{"uri":"file://server/share/devcontainer.json","index":0}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sb strings.Builder
			if err := testSARIFFormat().WriteReport(&sb, reportForPath(tt.path)); err != nil {
				t.Fatalf("WriteReport: %v", err)
			}
			if !strings.Contains(sb.String(), tt.want) {
				t.Errorf("WriteReport sarif = %q, want it to contain %q", sb.String(), tt.want)
			}
		})
	}

	t.Run("absolute path", func(t *testing.T) {
		t.Parallel()

		var sb strings.Builder
		if err := testSARIFFormat().WriteReport(&sb, reportForPath(filepath.Join(t.TempDir(), "devcontainer.json"))); err != nil {
			t.Fatalf("WriteReport: %v", err)
		}

		// The URI's own path varies with the host, so only its form is asserted: a file URI with an
		// empty authority.
		out := sb.String()
		if !strings.Contains(out, `"artifactLocation":{"uri":"file:///`) {
			t.Errorf("WriteReport sarif = %q, want an absolute file URI", out)
		}
	})
}

// reportForPath returns a one-file, one-issue report for the file at path.
func reportForPath(path string) Report {
	issues := testIssues()[:1]
	issues[0].Path = path
	return Report{Files: []File{{Path: path, Type: "devcontainer"}}, Issues: issues}
}

// TestSARIFWriteReport_UnlintedIssuePath checks that an issue whose file is not among the linted
// ones is still located, just without an artifact index to point at.
func TestSARIFWriteReport_UnlintedIssuePath(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := testSARIFFormat().WriteReport(&sb, Report{Issues: testIssues()[:1]}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	want := `"artifactLocation":{"uri":".devcontainer/devcontainer.json"}`
	if !strings.Contains(sb.String(), want) {
		t.Errorf("WriteReport sarif = %q, want it to contain %q", sb.String(), want)
	}
}

// TestSARIFWriteReport_Empty checks that an enabled rule is declared even when it produced no
// result, which is how a consumer tells a rule that ran clean from one that was switched off.
func TestSARIFWriteReport_Empty(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := testSARIFFormat().WriteReport(&sb, Report{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	want := `{"$schema":"https://json.schemastore.org/sarif-2.1.0.json","version":"2.1.0","runs":[` +
		`{"tool":{"driver":{"name":"decolint","version":"1.2.3","informationUri":"https://github.com/bare-devcontainer/decolint","rules":[` +
		`{"id":"no-image-latest","shortDescription":{"text":"images should be pinned to a specific version"},` +
		`"helpUri":"https://example.invalid/rules/no-image-latest/",` +
		`"help":{"text":"Documentation: https://example.invalid/rules/no-image-latest/",` +
		`"markdown":"[Rule documentation](https://example.invalid/rules/no-image-latest/)"},` +
		`"properties":{"tags":["reproducibility"]}}]}},` +
		`"artifacts":[],"results":[]}]}` +
		"\n"
	if sb.String() != want {
		t.Errorf("WriteReport sarif (empty) = %q, want %q", sb.String(), want)
	}
}

// TestSARIFWriteReport_RuleHelp checks how a rule's documentation address becomes the descriptor's
// helpUri and help, including that a rule with nowhere to link to carries neither field.
func TestSARIFWriteReport_RuleHelp(t *testing.T) {
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
			if err := f.WriteReport(&sb, Report{Issues: testIssues()[1:]}); err != nil {
				t.Fatalf("WriteReport: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(sb.String(), want) {
					t.Errorf("WriteReport sarif = %q, want it to contain %q", sb.String(), want)
				}
			}
			for _, omitted := range tt.wantOmitted {
				if strings.Contains(sb.String(), omitted) {
					t.Errorf("WriteReport sarif = %q, want it to omit %q", sb.String(), omitted)
				}
			}
		})
	}
}

func TestSARIFWriteReport_WriteError(t *testing.T) {
	t.Parallel()

	if err := testSARIFFormat().WriteReport(errWriter{}, testReport()); !errors.Is(err, errWrite) {
		t.Errorf("WriteReport error = %v, want %v", err, errWrite)
	}
}
