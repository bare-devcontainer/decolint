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
	"github.com/bare-devcontainer/decolint/rules"
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
	// mergeFile is where every firing in the merge fixture is reported.
	const mergeFile = "testdata/e2e/merge/.devcontainer/devcontainer.json"

	tests := []struct {
		name         string
		args         []string // CLI args, excluding -format=json which is appended for all cases
		want         []firing
		wantExitCode int
	}{
		{
			// Only the correctness category is enabled by default, so of everything the fixture
			// trips, just its two platform-scoped correctness rules fire: no-bind-mount and
			// no-host-port-format. Every security/reproducibility violation (privileged container,
			// docker socket mount, unpinned image and feature) stays silent until those categories
			// are opted into.
			name: "violations",
			args: []string{"-platform=vscode,codespaces", "testdata/e2e/violations"},
			want: []firing{
				{violationsFile, "no-bind-mount", linter.SeverityError},
				{violationsFile, "no-host-port-format", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// Without a platform selection, the only enabled-by-default rules that apply to this
			// fixture (no-bind-mount, no-host-port-format) are both codespaces-scoped and so are not
			// registered; nothing else is on by default, so nothing fires.
			name:         "violations without platform selection",
			args:         []string{"testdata/e2e/violations"},
			want:         nil,
			wantExitCode: 0,
		},
		{
			// security-warn.jsonc opts the security category in at warn severity. With no platform
			// selection the platform-scoped correctness rules aren't registered, so deny-warnings is
			// the only thing standing between these warnings and a clean exit.
			name: "violations with deny-warnings",
			args: []string{"-deny-warnings", "-config=testdata/e2e/security-warn.jsonc", "testdata/e2e/violations"},
			want: []firing{
				{violationsFile, "no-docker-socket-mount", linter.SeverityWarn},
				{violationsFile, "no-privileged-container", linter.SeverityWarn},
				{violationsFile, "no-seccomp-override", linter.SeverityWarn},
				{violationsFile, "require-cap-drop-all", linter.SeverityWarn},
				{violationsFile, "require-no-new-privileges", linter.SeverityWarn},
				{violationsFile, "require-non-root", linter.SeverityWarn},
			},
			wantExitCode: 1, // warnings now cross the fail threshold
		},
		{
			// override.jsonc exercises every kind of severity override: promoting no-image-latest to
			// error, disabling pin-feature-version, and enabling pin-image-digest (off by default).
			// Every other reproducibility/security rule stays off, since the category itself is not
			// opted into.
			name: "violations with config overrides",
			args: []string{
				"-platform=vscode,codespaces",
				"-config=testdata/e2e/override.jsonc",
				"testdata/e2e/violations",
			},
			want: []firing{
				{violationsFile, "no-bind-mount", linter.SeverityError},
				{violationsFile, "no-host-port-format", linter.SeverityError},
				{violationsFile, "no-image-latest", linter.SeverityError},
				{violationsFile, "pin-image-digest", linter.SeverityWarn},
			},
			wantExitCode: 1,
		},
		{
			// categories.jsonc raises security rules to error (enabling the off-by-default
			// hardening rules), turns reproducibility rules off (already the default), and keeps a
			// per-rule override (no-privileged-container back to warn) winning over its category.
			// Correctness rules are unaffected and stay at their enabled-by-default error severity.
			name: "violations with category overrides",
			args: []string{
				"-platform=vscode,codespaces",
				"-config=testdata/e2e/categories.jsonc",
				"testdata/e2e/violations",
			},
			want: []firing{
				{violationsFile, "no-bind-mount", linter.SeverityError},
				{violationsFile, "no-host-port-format", linter.SeverityError},
				{violationsFile, "no-docker-socket-mount", linter.SeverityError},
				{violationsFile, "no-privileged-container", linter.SeverityWarn},
				{violationsFile, "no-seccomp-override", linter.SeverityError},
				{violationsFile, "require-cap-drop-all", linter.SeverityError},
				{violationsFile, "require-no-new-privileges", linter.SeverityError},
				{violationsFile, "require-non-root", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// platforms.jsonc selects vscode and codespaces via the config file's "platforms"
			// member, so the same platform-scoped correctness rules fire as in the "violations"
			// case, without any -platform flag.
			name: "violations with platforms from config",
			args: []string{"-config=testdata/e2e/platforms.jsonc", "testdata/e2e/violations"},
			want: []firing{
				{violationsFile, "no-bind-mount", linter.SeverityError},
				{violationsFile, "no-host-port-format", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// The -platform flag overrides the config's "platforms" member. The fixture's firing
			// rules are codespaces-scoped, so narrowing the selection to vscode via the flag
			// silences everything platforms.jsonc would otherwise enable.
			name: "platform flag overrides config platforms",
			args: []string{
				"-platform=vscode",
				"-config=testdata/e2e/platforms.jsonc",
				"testdata/e2e/violations",
			},
			want:         nil,
			wantExitCode: 0,
		},
		{
			name:         "clean",
			args:         []string{"-platform=vscode,codespaces", "testdata/e2e/clean"},
			want:         nil,
			wantExitCode: 0,
		},
		{
			// With -merge-features, the local Feature's contributions (privileged mode and a Docker
			// socket mount) become part of the effective configuration and trip the security rules
			// merge.jsonc enables.
			name: "merge features",
			args: []string{"-merge-features", "-config=testdata/e2e/merge.jsonc", "testdata/e2e/merge"},
			want: []firing{
				{mergeFile, "no-docker-socket-mount", linter.SeverityError},
				{mergeFile, "no-privileged-container", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// Without the flag only the raw file is linted, and the raw file is clean.
			name:         "merge features disabled",
			args:         []string{"-config=testdata/e2e/merge.jsonc", "testdata/e2e/merge"},
			want:         nil,
			wantExitCode: 0,
		},
		{
			// merge-on.jsonc enables merging via the config file's "mergeFeatures" member.
			name: "merge features enabled by config",
			args: []string{"-config=testdata/e2e/merge-on.jsonc", "testdata/e2e/merge"},
			want: []firing{
				{mergeFile, "no-docker-socket-mount", linter.SeverityError},
				{mergeFile, "no-privileged-container", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// -merge-features=false, given explicitly, overrides merge-on.jsonc's "mergeFeatures":
			// true and disables merging.
			name:         "merge features disabled by CLI flag overrides config",
			args:         []string{"-merge-features=false", "-config=testdata/e2e/merge-on.jsonc", "testdata/e2e/merge"},
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

	t.Run("-rules", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-rules"}, &stdout, &stderr)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		out := stdout.String()

		header := mdTableRow(t, out, rulesTableHeader[0])
		if diff := cmp.Diff(rulesTableHeader, header); diff != "" {
			t.Errorf("header row mismatch (-want +got):\n%s", diff)
		}

		row := mdTableRow(t, out, "no-image-latest")
		wantRow := []string{"no-image-latest", "reproducibility", "(all)", severityEmoji[linter.SeverityOff]}
		if diff := cmp.Diff(wantRow, row); diff != "" {
			t.Errorf("no-image-latest row mismatch (-want +got):\n%s", diff)
		}

		row = mdTableRow(t, out, "no-bind-mount")
		wantRow = []string{"no-bind-mount", "correctness", "codespaces", severityEmoji[linter.SeverityError]}
		if diff := cmp.Diff(wantRow, row); diff != "" {
			t.Errorf("no-bind-mount row mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("-rules with -config", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-rules", "-config=testdata/e2e/override.jsonc"}, &stdout, &stderr)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		out := stdout.String()

		// no-image-latest: default off, overridden to error.
		row := mdTableRow(t, out, "no-image-latest")
		wantRow := []string{"no-image-latest", "reproducibility", "(all)", severityEmoji[linter.SeverityError]}
		if diff := cmp.Diff(wantRow, row); diff != "" {
			t.Errorf("no-image-latest row mismatch (-want +got):\n%s", diff)
		}

		// pin-feature-version: default off, overridden to off (a no-op, but still an explicit entry).
		row = mdTableRow(t, out, "pin-feature-version")
		wantRow = []string{"pin-feature-version", "reproducibility", "(all)", severityEmoji[linter.SeverityOff]}
		if diff := cmp.Diff(wantRow, row); diff != "" {
			t.Errorf("pin-feature-version row mismatch (-want +got):\n%s", diff)
		}

		// pin-image-digest: default off, overridden to warn.
		row = mdTableRow(t, out, "pin-image-digest")
		wantRow = []string{"pin-image-digest", "reproducibility", "(all)", severityEmoji[linter.SeverityWarn]}
		if diff := cmp.Diff(wantRow, row); diff != "" {
			t.Errorf("pin-image-digest row mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("-rules with category overrides in -config", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-rules", "-config=testdata/e2e/categories.jsonc"}, &stdout, &stderr)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		out := stdout.String()

		// no-seccomp-override: default off, raised to error by its security category.
		row := mdTableRow(t, out, "no-seccomp-override")
		wantRow := []string{"no-seccomp-override", "security", "(all)", severityEmoji[linter.SeverityError]}
		if diff := cmp.Diff(wantRow, row); diff != "" {
			t.Errorf("no-seccomp-override row mismatch (-want +got):\n%s", diff)
		}

		// no-privileged-container: default off, the per-rule override (warn) wins over its category
		// override (error).
		row = mdTableRow(t, out, "no-privileged-container")
		wantRow = []string{"no-privileged-container", "security", "(all)", severityEmoji[linter.SeverityWarn]}
		if diff := cmp.Diff(wantRow, row); diff != "" {
			t.Errorf("no-privileged-container row mismatch (-want +got):\n%s", diff)
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

func TestRunInit(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.

	t.Run("writes every rule at its default severity", func(t *testing.T) {
		t.Chdir(t.TempDir())

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-init"}, &stdout, &stderr)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}

		data, err := os.ReadFile(".decolint.jsonc")
		if err != nil {
			t.Fatalf("ReadFile(.decolint.jsonc): %v", err)
		}
		cfg, err := parseConfig(".decolint.jsonc", data)
		if err != nil {
			t.Fatalf("generated config does not parse: %v\ncontent:\n%s", err, data)
		}

		want := make(map[string]linter.Severity)
		for _, reg := range rules.Builtin() {
			want[reg.Rule.ID] = reg.DefaultSeverity
		}
		if diff := cmp.Diff(want, cfg.Rules); diff != "" {
			t.Errorf("Rules mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("refuses to overwrite an existing config", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if err := os.WriteFile(".decolint.jsonc", []byte(`{"rules":{}}`), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-init"}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2", exitCode)
		}
		if !strings.Contains(stderr.String(), "already exists") {
			t.Errorf("stderr = %q, want it to mention the file already exists", stderr.String())
		}
	})
}

func TestRunLint(t *testing.T) {
	t.Parallel()

	t.Run("no config file, exit code unchanged", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout bytes.Buffer
		hasIssue, runErr := runLint(t.Context(), &stdout, Options{Paths: []string{dir}, Format: format.TextFormat{}}, Config{})
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
		}
		cfg := Config{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts, cfg)
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
		}
		cfg := Config{Rules: map[string]linter.Severity{"missing-container-def": linter.SeverityOff}}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts, cfg)
		if runErr != nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("unknown rule ID in config", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		opts := Options{
			Format: format.TextFormat{},
		}
		cfg := Config{Rules: map[string]linter.Severity{"no-image-latst": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts, cfg)
		if runErr == nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, non-nil", hasIssue, runErr)
		}
		if runErr != nil && !strings.Contains(runErr.Error(), "no-image-latst") {
			t.Errorf("err = %q, want it to mention the unknown rule ID", runErr)
		}
	})

	t.Run("config category override promotes a warn rule to error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`) // no-image-latest is in reproducibility

		var stdout bytes.Buffer
		opts := Options{
			Paths:  []string{dir},
			Format: format.TextFormat{},
		}
		cfg := Config{Categories: map[string]linter.Severity{"reproducibility": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts, cfg)
		if runErr != nil || !hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want true, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("unknown category name in config", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		opts := Options{
			Format: format.TextFormat{},
		}
		cfg := Config{Categories: map[string]linter.Severity{"secure": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts, cfg)
		if runErr == nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, non-nil", hasIssue, runErr)
		}
		if runErr != nil && !strings.Contains(runErr.Error(), "secure") {
			t.Errorf("err = %q, want it to mention the unknown category name", runErr)
		}
	})

	t.Run("file path is rejected", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:24.04"}`)
		file := filepath.Join(dir, ".devcontainer", "devcontainer.json")

		var stdout bytes.Buffer
		hasIssue, runErr := runLint(t.Context(), &stdout, Options{Paths: []string{file}, Format: format.TextFormat{}}, Config{})
		if runErr == nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, 'not a directory'", hasIssue, runErr)
		}
	})

	t.Run("override for unselected platform-scoped rule is not an error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout bytes.Buffer
		opts := Options{
			Paths:  []string{dir},
			Format: format.TextFormat{},
		}
		cfg := Config{Rules: map[string]linter.Severity{"no-bind-mount": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, opts, cfg)
		if runErr != nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})
}

func TestRunMergeFeatures(t *testing.T) {
	t.Parallel()

	t.Run("findings point at the feature reference", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge-features", "-config=testdata/e2e/merge.jsonc", "testdata/e2e/merge"}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}

		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
		}
		// The fixture references "./privileged-feature" on line 5; every merged-in property must be
		// reported there.
		for _, issue := range issues {
			if issue.Line != 5 {
				t.Errorf("issue %s reported at line %d, want 5 (the feature reference)", issue.RuleID, issue.Line)
			}
		}
	})

	t.Run("unresolvable feature is a runtime error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:24.04", "features": {"./missing": {}}}`)

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-merge-features", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2", exitCode)
		}
		if !strings.Contains(stderr.String(), "./missing") {
			t.Errorf("stderr = %q, want it to mention the unresolvable feature", stderr.String())
		}
	})

	t.Run("local feature escaping .devcontainer is a runtime error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:24.04", "features": {"../sibling-feature": {}}}`)
		// sibling-feature exists on disk, but as a project-root sibling of .devcontainer, not inside
		// it, so it is outside the boundary local Feature references are confined to.
		if err := os.MkdirAll(filepath.Join(dir, "sibling-feature"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sibling-feature", "devcontainer-feature.json"),
			[]byte(`{"id": "sibling-feature"}`), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-merge-features", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2; stdout: %s", exitCode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "../sibling-feature") {
			t.Errorf("stderr = %q, want it to mention the unresolvable feature", stderr.String())
		}
	})
}

// mdTableRow finds the row of a Markdown table in out whose first cell, after trimming the padding
// listRules adds for alignment, equals key, and returns its cells trimmed of that padding. It fails
// the test if no such row exists.
func mdTableRow(t *testing.T, out, key string) []string {
	t.Helper()
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i, cell := range cells {
			cells[i] = strings.TrimSpace(cell)
		}
		if len(cells) > 0 && cells[0] == key {
			return cells
		}
	}
	t.Fatalf("no table row starting with %q in:\n%s", key, out)
	return nil
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
