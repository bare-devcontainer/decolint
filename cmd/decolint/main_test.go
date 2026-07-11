package main

import (
	"bytes"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/format"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestRun(t *testing.T) {
	t.Parallel()

	// firing identifies a rule that fired, without the message/position details asserted elsewhere.
	type firing struct {
		Path     string
		RuleID   string
		Severity linter.Severity
	}

	// violationsFile is where every firing in the violations fixture is reported.
	const violationsFile = "testdata/e2e/violations/.devcontainer/devcontainer.json"

	tests := []struct {
		name         string
		args         []string // CLI args, excluding -format=json which is appended for all cases
		want         []firing
		wantExitCode int
	}{
		{
			name: "violations",
			args: []string{"-platform=vscode,codespaces", "testdata/e2e/violations"},
			want: []firing{
				// The fixture uses every ignore directive kind, each suppressing a rule that would
				// otherwise fire: decolint-ignore-file (no-seccomp-unconfined), decolint-ignore-line
				// (no-cap-add-all), and decolint-ignore-next-line (no-app-port).
				{violationsFile, "codespaces-no-bind-mount", linter.Warn},
				{violationsFile, "codespaces-no-host-port-format", linter.Error},
				{violationsFile, "no-docker-socket-mount", linter.Warn},
				{violationsFile, "no-image-latest", linter.Warn},
				{violationsFile, "no-privileged-container", linter.Warn},
				{violationsFile, "pin-feature-version", linter.Warn},
			},
			wantExitCode: 1, // codespaces-no-host-port-format is an error by default
		},
		{
			// Without a platform selection the codespaces-scoped rules are not registered, and with
			// them goes the only error-severity firing, so the exit signal flips too.
			name: "violations without platform selection",
			args: []string{"testdata/e2e/violations"},
			want: []firing{
				{violationsFile, "no-docker-socket-mount", linter.Warn},
				{violationsFile, "no-image-latest", linter.Warn},
				{violationsFile, "no-privileged-container", linter.Warn},
				{violationsFile, "pin-feature-version", linter.Warn},
			},
			wantExitCode: 0,
		},
		{
			name: "violations with deny-warnings",
			args: []string{"-deny-warnings", "testdata/e2e/violations"},
			want: []firing{
				{violationsFile, "no-docker-socket-mount", linter.Warn},
				{violationsFile, "no-image-latest", linter.Warn},
				{violationsFile, "no-privileged-container", linter.Warn},
				{violationsFile, "pin-feature-version", linter.Warn},
			},
			wantExitCode: 1, // warnings now cross the fail threshold
		},
		{
			// override.jsonc exercises every kind of severity override: promoting no-image-latest to
			// error, disabling pin-feature-version, and enabling pin-image-digest (off by default).
			name: "violations with config overrides",
			args: []string{
				"-platform=vscode,codespaces",
				"-config=testdata/e2e/override.jsonc",
				"testdata/e2e/violations",
			},
			want: []firing{
				{violationsFile, "codespaces-no-bind-mount", linter.Warn},
				{violationsFile, "codespaces-no-host-port-format", linter.Error},
				{violationsFile, "no-docker-socket-mount", linter.Warn},
				{violationsFile, "no-image-latest", linter.Error},
				{violationsFile, "no-privileged-container", linter.Warn},
				{violationsFile, "pin-image-digest", linter.Warn},
			},
			wantExitCode: 1,
		},
		{
			name:         "clean",
			args:         []string{"-platform=vscode,codespaces", "testdata/e2e/clean"},
			want:         nil,
			wantExitCode: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := append([]string{"-format=json"}, tt.args...)

			var stdout, stderr bytes.Buffer
			exitCode := run(t.Context(), args, &stdout, &stderr)
			if stderr.String() != "" {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if exitCode != tt.wantExitCode {
				t.Errorf("exit code = %d, want %d", exitCode, tt.wantExitCode)
			}

			var issues []linter.Issue
			if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
				t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
			}

			var got []firing
			for _, issue := range issues {
				if issue.Line <= 0 || issue.Col <= 0 {
					t.Errorf("issue %s has no position: line %d, col %d", issue.RuleID, issue.Line, issue.Col)
				}
				got = append(got, firing{issue.Path, issue.RuleID, issue.Severity})
			}
			sortFirings := cmpopts.SortSlices(func(a, b firing) bool {
				if a.Path != b.Path {
					return a.Path < b.Path
				}
				return a.RuleID < b.RuleID
			})
			if diff := cmp.Diff(tt.want, got, sortFirings); diff != "" {
				t.Errorf("fired rules mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRun_Flags(t *testing.T) {
	t.Parallel()

	t.Run("-version", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-version"}, &stdout, &stderr)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		if !strings.Contains(stdout.String(), "decolint") {
			t.Errorf("stdout = %q, want it to mention decolint", stdout.String())
		}
	})

	t.Run("-help", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-help"}, &stdout, &stderr)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if stdout.String() != "" {
			t.Errorf("stdout = %q, want empty", stdout.String())
		}
		if !strings.Contains(stderr.String(), "usage:") {
			t.Errorf("stderr = %q, want it to contain usage text", stderr.String())
		}
	})

	t.Run("unknown flag", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-bogus"}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2", exitCode)
		}
		if stdout.String() != "" {
			t.Errorf("stdout = %q, want empty", stdout.String())
		}
		if !strings.Contains(stderr.String(), "-bogus") {
			t.Errorf("stderr = %q, want it to mention the unknown flag", stderr.String())
		}
	})

	t.Run("-config path does not exist", func(t *testing.T) {
		t.Parallel()

		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-config=nonexistent.jsonc", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2", exitCode)
		}
		if stdout.String() != "" {
			t.Errorf("stdout = %q, want empty", stdout.String())
		}
		if !strings.Contains(stderr.String(), "nonexistent.jsonc") {
			t.Errorf("stderr = %q, want it to mention the missing path", stderr.String())
		}
	})
}

func TestRunLint(t *testing.T) {
	t.Parallel()

	t.Run("no config file, exit code unchanged", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout bytes.Buffer
		hasIssue, runErr := runLint(t.Context(), &stdout, Options{Paths: []string{dir}, Format: format.TextFormat{}})
		if runErr != nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("config promotes a warn rule to error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout bytes.Buffer
		opts := Options{
			Paths:  []string{dir},
			Format: format.TextFormat{},
			Config: Config{Rules: map[string]linter.Severity{"no-image-latest": linter.Error}},
		}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts)
		if runErr != nil || !hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want true, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("config disables a rule that defaults to error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{}`) // triggers missing-container-def (error by default)

		var stdout bytes.Buffer
		opts := Options{
			Paths:  []string{dir},
			Format: format.TextFormat{},
			Config: Config{Rules: map[string]linter.Severity{"missing-container-def": linter.Off}},
		}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts)
		if runErr != nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("unknown rule ID in config", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		opts := Options{
			Format: format.TextFormat{},
			Config: Config{Rules: map[string]linter.Severity{"no-image-latst": linter.Error}},
		}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts)
		if runErr == nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, non-nil", hasIssue, runErr)
		}
		if runErr != nil && !strings.Contains(runErr.Error(), "no-image-latst") {
			t.Errorf("err = %q, want it to mention the unknown rule ID", runErr)
		}
	})

	t.Run("override for unselected platform-scoped rule is not an error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout bytes.Buffer
		opts := Options{
			Paths:  []string{dir},
			Format: format.TextFormat{},
			Config: Config{Rules: map[string]linter.Severity{"codespaces-no-bind-mount": linter.Error}},
		}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts)
		if runErr != nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})
}

// writeDevcontainer writes a minimal devcontainer.json, with the given body as its content, at the
// spec-defined location under a fresh temp directory, and returns that directory's path.
func writeDevcontainer(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir
}
