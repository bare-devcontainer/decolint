package format

import (
	"errors"
	"strings"
	"testing"
)

func TestJSONWriteReport(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := (JSONFormat{}).WriteReport(&sb, testReport()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	want := "{\"files\":[{\"path\":\".devcontainer/devcontainer.json\",\"type\":\"devcontainer\"},{\"path\":\"src/devcontainer-feature.json\",\"type\":\"feature\"}]," +
		"\"issues\":[{\"path\":\".devcontainer/devcontainer.json\",\"line\":4,\"col\":12,\"ruleId\":\"no-image-latest\",\"message\":\"image \\\"ubuntu:latest\\\" uses the \\\"latest\\\" tag; pin a specific version\",\"severity\":\"warn\"},{\"path\":\".devcontainer/devcontainer.json\",\"line\":8,\"col\":3,\"ruleId\":\"some-error-rule\",\"message\":\"something is broken\",\"severity\":\"error\"}]}\n"
	if sb.String() != want {
		t.Errorf("WriteReport json = %q, want %q", sb.String(), want)
	}
}

// TestJSONWriteReport_Empty checks that both members stay arrays when there is nothing to report,
// so a consumer never has to handle null.
func TestJSONWriteReport_Empty(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := (JSONFormat{}).WriteReport(&sb, Report{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	want := "{\"files\":[],\"issues\":[]}\n"
	if sb.String() != want {
		t.Errorf("WriteReport json (empty) = %q, want %q", sb.String(), want)
	}
}

func TestJSONWriteReport_WriteError(t *testing.T) {
	t.Parallel()

	if err := (JSONFormat{}).WriteReport(errWriter{}, testReport()); !errors.Is(err, errWrite) {
		t.Errorf("WriteReport error = %v, want %v", err, errWrite)
	}
}
