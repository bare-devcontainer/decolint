package format

import (
	"errors"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
)

// errWriter is an io.Writer that always fails, used to exercise the write-error paths of the
// formatters.
type errWriter struct{}

var errWrite = errors.New("write failed")

func (errWriter) Write([]byte) (int, error) { return 0, errWrite }

func testIssues() []linter.Issue {
	return []linter.Issue{
		{
			Path:     ".devcontainer/devcontainer.json",
			Line:     4,
			Col:      12,
			RuleID:   "no-image-latest",
			Message:  `image "ubuntu:latest" uses the "latest" tag; pin a specific version`,
			Severity: linter.SeverityWarn,
		},
		{
			Path:     ".devcontainer/devcontainer.json",
			Line:     8,
			Col:      3,
			RuleID:   "some-error-rule",
			Message:  "something is broken",
			Severity: linter.SeverityError,
		},
	}
}

func TestTextWriteIssues(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := (TextFormat{}).WriteIssues(&sb, testIssues()); err != nil {
		t.Fatalf("WriteIssues: %v", err)
	}

	want := `.devcontainer/devcontainer.json:4:12: warn: image "ubuntu:latest" uses the "latest" tag; pin a specific version (no-image-latest)
.devcontainer/devcontainer.json:8:3: error: something is broken (some-error-rule)
Found 1 error and 1 warning.
`
	if sb.String() != want {
		t.Errorf("WriteIssues text = %q, want %q", sb.String(), want)
	}
}

func TestTextWriteIssuesWriteError(t *testing.T) {
	t.Parallel()

	// A non-empty issue list fails on the per-issue write; an empty list writes no issues and
	// reaches the summary write, so both write points are covered.
	tests := []struct {
		name   string
		issues []linter.Issue
	}{
		{"per-issue write", testIssues()},
		{"summary write", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := (TextFormat{}).WriteIssues(errWriter{}, tt.issues); !errors.Is(err, errWrite) {
				t.Errorf("WriteIssues error = %v, want %v", err, errWrite)
			}
		})
	}
}
