package format

import (
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
)

func testIssues() []linter.Issue {
	return []linter.Issue{
		{
			Path:     ".devcontainer/devcontainer.json",
			Line:     4,
			Col:      12,
			RuleID:   "no-image-latest",
			Message:  `image "ubuntu:latest" uses the "latest" tag; pin a specific version`,
			Severity: linter.Warn,
		},
		{
			Path:     ".devcontainer/devcontainer.json",
			Line:     8,
			Col:      3,
			RuleID:   "some-error-rule",
			Message:  "something is broken",
			Severity: linter.Error,
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
`
	if sb.String() != want {
		t.Errorf("WriteIssues text = %q, want %q", sb.String(), want)
	}
}
