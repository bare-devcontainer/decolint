package format

import (
	"errors"
	"strings"
	"testing"
)

func TestJSONWriteIssues(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := (JSONFormat{}).WriteIssues(&sb, testIssues()); err != nil {
		t.Fatalf("WriteIssues: %v", err)
	}

	want := "[{\"path\":\".devcontainer/devcontainer.json\",\"line\":4,\"col\":12,\"ruleId\":\"no-image-latest\",\"message\":\"image \\\"ubuntu:latest\\\" uses the \\\"latest\\\" tag; pin a specific version\",\"severity\":\"warn\"},{\"path\":\".devcontainer/devcontainer.json\",\"line\":8,\"col\":3,\"ruleId\":\"some-error-rule\",\"message\":\"something is broken\",\"severity\":\"error\"}]\n"
	if sb.String() != want {
		t.Errorf("WriteIssues json = %q, want %q", sb.String(), want)
	}
}

func TestJSONWriteIssues_Empty(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := (JSONFormat{}).WriteIssues(&sb, nil); err != nil {
		t.Fatalf("WriteIssues: %v", err)
	}

	if sb.String() != "[]\n" {
		t.Errorf("WriteIssues json (empty) = %q, want %q", sb.String(), "[]\n")
	}
}

func TestJSONWriteIssues_WriteError(t *testing.T) {
	t.Parallel()

	if err := (JSONFormat{}).WriteIssues(errWriter{}, testIssues()); !errors.Is(err, errWrite) {
		t.Errorf("WriteIssues error = %v, want %v", err, errWrite)
	}
}
