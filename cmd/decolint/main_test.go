package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/discovery"
	"github.com/bare-devcontainer/decolint/format"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/ocitest"
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
	// featureFile is the Feature metadata in the feature fixture.
	const featureFile = "testdata/e2e/feature/devcontainer-feature.json"
	// templateFile is the Template metadata in the template fixture; templateBundledFile is the dev
	// container configuration bundled alongside it.
	const templateFile = "testdata/e2e/template/devcontainer-template.json"
	const templateBundledFile = "testdata/e2e/template/.devcontainer/devcontainer.json"

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
			// A Feature directory is detected by its devcontainer-feature.json and linted with the
			// Feature-scoped correctness rules, all enabled by default.
			name: "feature directory",
			args: []string{"testdata/e2e/feature"},
			want: []firing{
				{featureFile, "id-dir-mismatch", linter.SeverityError},
				{featureFile, "invalid-semver", linter.SeverityError},
				{featureFile, "missing-required-props", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// A Template directory is linted both for its devcontainer-template.json and for the dev
			// container configuration it bundles, so findings appear at both files.
			name: "template directory",
			args: []string{"testdata/e2e/template"},
			want: []firing{
				{templateFile, "id-dir-mismatch", linter.SeverityError},
				{templateBundledFile, "missing-container-def", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// Multiple directories are linted in one run and their issues aggregated; the clean
			// directory contributes nothing while the violations directory drives the exit code.
			name: "multiple directories aggregate issues",
			args: []string{"-platform=vscode,codespaces", "testdata/e2e/clean", "testdata/e2e/violations"},
			want: []firing{
				{violationsFile, "no-bind-mount", linter.SeverityError},
				{violationsFile, "no-host-port-format", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// With -merge, the local Feature's contributions (privileged mode and a Docker
			// socket mount) become part of the effective configuration and trip the security rules
			// merge.jsonc enables.
			name: "merge features",
			args: []string{"-merge", "-config=testdata/e2e/merge.jsonc", "testdata/e2e/merge"},
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
			// merge-on.jsonc enables merging via the config file's "merge" member.
			name: "merge features enabled by config",
			args: []string{"-config=testdata/e2e/merge-on.jsonc", "testdata/e2e/merge"},
			want: []firing{
				{mergeFile, "no-docker-socket-mount", linter.SeverityError},
				{mergeFile, "no-privileged-container", linter.SeverityError},
			},
			wantExitCode: 1,
		},
		{
			// -merge=false, given explicitly, overrides merge-on.jsonc's "merge":
			// true and disables merging.
			name:         "merge features disabled by CLI flag overrides config",
			args:         []string{"-merge=false", "-config=testdata/e2e/merge-on.jsonc", "testdata/e2e/merge"},
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

	t.Run("invalid -format value", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-format=bogus", "testdata/e2e/clean"}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2", exitCode)
		}
		if stdout.String() != "" {
			t.Errorf("stdout = %q, want empty", stdout.String())
		}
		if !strings.Contains(stderr.String(), "bogus") {
			t.Errorf("stderr = %q, want it to mention the invalid format", stderr.String())
		}
	})

	t.Run("invalid -platform value", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-platform=bogus", "testdata/e2e/clean"}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2", exitCode)
		}
		if stdout.String() != "" {
			t.Errorf("stdout = %q, want empty", stdout.String())
		}
		if !strings.Contains(stderr.String(), "bogus") {
			t.Errorf("stderr = %q, want it to mention the invalid platform", stderr.String())
		}
	})
}

func TestRun_OutputFormat(t *testing.T) {
	t.Parallel()

	// The violations fixture, with both platforms selected, fires exactly two error-severity rules:
	// no-bind-mount and no-host-port-format.
	const violationsDir = "testdata/e2e/violations"
	const violationsFile = "testdata/e2e/violations/.devcontainer/devcontainer.json"

	t.Run("text is the default format", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-platform=vscode,codespaces", violationsDir}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		out := stdout.String()
		for _, want := range []string{
			violationsFile + ":",
			"(no-bind-mount)",
			"(no-host-port-format)",
			"Found 2 errors and 0 warnings.",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("text output missing %q; got:\n%s", want, out)
			}
		}
	})

	t.Run("github workflow commands", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-format=github", "-platform=vscode,codespaces", violationsDir}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		out := stdout.String()
		for _, want := range []string{
			"::error file=" + violationsFile,
			"title=no-bind-mount",
			"title=no-host-port-format",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("github output missing %q; got:\n%s", want, out)
			}
		}
	})
}

func TestRun_BrokenConfig(t *testing.T) {
	t.Parallel()

	// A devcontainer.json that does not parse: the run reports the parse failure as exit code 2
	// with a message naming the broken file, and emits no issues for the directory.
	dir := writeDevcontainer(t, `{`)

	var stdout, stderr bytes.Buffer
	exitCode := run(t.Context(), []string{"-format=json", dir}, &stdout, &stderr)
	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "devcontainer.json") {
		t.Errorf("stderr = %q, want it to name the broken file", stderr.String())
	}

	// A parse failure anywhere in a directory discards that directory's issues, so the JSON output
	// is a well-formed empty array rather than a partial or truncated result.
	var issues []linter.Issue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
	}
	if len(issues) != 0 {
		t.Errorf("issues = %v, want none", issues)
	}
}

func TestRun_DefaultDirectory(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".devcontainer", "devcontainer.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	// No path argument: the current directory is linted. The config trips missing-container-def.
	var stdout, stderr bytes.Buffer
	exitCode := run(t.Context(), []string{"-format=json"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
	}
	var issues []linter.Issue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
	}
	if len(issues) == 0 || issues[0].RuleID != "missing-container-def" {
		t.Errorf("issues = %v, want missing-container-def from the current directory", issues)
	}
}

func TestRun_ConfigDiscovery(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.

	// devcontainerBody uses image:latest, which trips no-image-latest (off by default, in the
	// reproducibility category) once a discovered config turns it on.
	const devcontainerBody = `{"image": "ubuntu:latest"}`

	writeProject := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".devcontainer", "devcontainer.json"), []byte(devcontainerBody), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("discovers .decolint.jsonc without -config", func(t *testing.T) {
		project := writeProject(t)
		t.Chdir(t.TempDir())
		if err := os.WriteFile(".decolint.jsonc", []byte(`{"rules": {"no-image-latest": "error"}}`), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-format=json", project}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		if !hasRule(t, stdout.Bytes(), "no-image-latest") {
			t.Errorf("want no-image-latest enabled by the discovered config; output: %s", stdout.String())
		}
	})

	t.Run(".jsonc takes precedence over .json", func(t *testing.T) {
		project := writeProject(t)
		t.Chdir(t.TempDir())
		// The .jsonc enables the rule; the .json would disable it. The .jsonc must win.
		if err := os.WriteFile(".decolint.jsonc", []byte(`{"rules": {"no-image-latest": "error"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(".decolint.json", []byte(`{"rules": {"no-image-latest": "off"}}`), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-format=json", project}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		if !hasRule(t, stdout.Bytes(), "no-image-latest") {
			t.Errorf("want the .jsonc config to win; output: %s", stdout.String())
		}
	})
}

func TestRun_Init(t *testing.T) {
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

func TestRun_Merge(t *testing.T) {
	t.Parallel()

	t.Run("findings point at the feature reference", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=testdata/e2e/merge.jsonc", "testdata/e2e/merge"}
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

	t.Run("merges a Feature fetched from an OCI registry", func(t *testing.T) {
		t.Parallel()

		host := ocitest.Registry(t)
		ocitest.PushFeature(t, host, "features/privileged", "1", ocitest.FeatureArchive(t, `{
			"id": "privileged",
			"version": "1.0.0",
			"name": "Privileged",
			"privileged": true,
			"mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]
		}`, false), false)

		// The devcontainer references the just-published Feature by its registry address, so the body
		// is generated here rather than kept as a static fixture.
		body := fmt.Sprintf(`{"image": "ubuntu:24.04", "features": {%q: {}}}`, host+"/features/privileged:1")
		dir := writeDevcontainer(t, body)

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=testdata/e2e/merge.jsonc", dir}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		for _, ruleID := range []string{"no-privileged-container", "no-docker-socket-mount"} {
			if !hasRule(t, stdout.Bytes(), ruleID) {
				t.Errorf("want %s to fire on the merged OCI Feature; output: %s", ruleID, stdout.String())
			}
		}
	})

	t.Run("a Feature fetch failure is a runtime error", func(t *testing.T) {
		t.Parallel()
		// The reserved .invalid TLD never resolves, so fetching the tarball fails and the failure
		// must surface as exit code 2 rather than a lint result.
		dir := writeDevcontainer(t, `{"image": "ubuntu:24.04", "features": {"https://features.invalid/f.tgz": {}}}`)

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-merge", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2; stdout: %s", exitCode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "features.invalid") {
			t.Errorf("stderr = %q, want it to mention the unreachable Feature", stderr.String())
		}
	})

	t.Run("a Feature dependency cycle is a runtime error", func(t *testing.T) {
		t.Parallel()
		// Two local Features depend on each other, forming a cycle the install-order resolution
		// rejects. Local dependsOn references resolve relative to the config directory, so the
		// Features are siblings under .devcontainer.
		dir := writeDevcontainer(t, `{"image": "ubuntu:24.04", "features": {"./a": {}}}`)
		devcontainer := filepath.Join(dir, ".devcontainer")
		for name, dep := range map[string]string{"a": "./b", "b": "./a"} {
			featureDir := filepath.Join(devcontainer, name)
			if err := os.MkdirAll(featureDir, 0o755); err != nil {
				t.Fatal(err)
			}
			body := fmt.Sprintf(`{"id": %q, "version": "1.0.0", "name": %q, "dependsOn": {%q: {}}}`, name, name, dep)
			if err := os.WriteFile(filepath.Join(featureDir, "devcontainer-feature.json"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-merge", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2; stdout: %s", exitCode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "cycle") {
			t.Errorf("stderr = %q, want it to mention the dependency cycle", stderr.String())
		}
	})

	t.Run("unresolvable feature is a runtime error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:24.04", "features": {"./missing": {}}}`)

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-merge", dir}, &stdout, &stderr)
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
		exitCode := run(t.Context(), []string{"-merge", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2; stdout: %s", exitCode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "../sibling-feature") {
			t.Errorf("stderr = %q, want it to mention the unresolvable feature", stderr.String())
		}
	})
}

// imageRule is a stub rule that flags any "image" property, used to observe which files a lint
// reaches without depending on the rules package.
var imageRule = &linter.Rule{
	ID:          "test-image",
	Description: "flags any image property",
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{"/image"},
	Check: func(_ *linter.Context, node *linter.Node) []linter.Finding {
		return []linter.Finding{{Message: "image present", Offset: node.Value.StartOffset}}
	},
}

func TestLintPath(t *testing.T) {
	t.Parallel()

	newLinter := func() *linter.Linter {
		l := linter.New()
		l.RegisterRule(imageRule, linter.SeverityWarn)
		return l
	}
	body := `{"image": "ubuntu:24.04"}`

	t.Run("aggregates issues across configs", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, body)
		if err := os.MkdirAll(filepath.Join(dir, ".devcontainer", "go"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".devcontainer", "go", "devcontainer.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

		issues, err := lintPath(t.Context(), newLinter(), nil, dir)
		if err != nil {
			t.Fatalf("lintPath: %v", err)
		}
		var got []string
		for _, issue := range issues {
			got = append(got, issue.Path)
		}
		want := []string{
			filepath.Join(dir, ".devcontainer", "devcontainer.json"),
			filepath.Join(dir, ".devcontainer", "go", "devcontainer.json"),
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("issue paths mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("a broken file does not stop other files in the same directory", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{`)
		if err := os.MkdirAll(filepath.Join(dir, ".devcontainer", "good"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".devcontainer", "good", "devcontainer.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = root.Close() }()
		issues, err := lintDir(t.Context(), newLinter(), nil, root)
		if err == nil {
			t.Fatal("got nil error, want a parse error for the broken config")
		}
		if len(issues) != 1 {
			t.Fatalf("got %d issues %v, want 1", len(issues), issues)
		}
		wantPath := filepath.Join(dir, ".devcontainer", "good", "devcontainer.json")
		if issues[0].Path != wantPath {
			t.Errorf("Path = %q, want %q", issues[0].Path, wantPath)
		}
	})

	t.Run("directory without config", func(t *testing.T) {
		t.Parallel()
		_, err := lintPath(t.Context(), newLinter(), nil, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "no devcontainer configuration found") {
			t.Errorf("err = %v, want 'no devcontainer configuration found'", err)
		}
	})

	t.Run("merge error aborts the file", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, body)
		wantErr := errors.New("fetch failed")
		merge := func(context.Context, discovery.ConfigFile, *linter.Document) error { return wantErr }

		if _, err := lintPath(t.Context(), newLinter(), merge, dir); !errors.Is(err, wantErr) {
			t.Errorf("lintPath error = %v, want %v", err, wantErr)
		}
	})

	t.Run("merge is skipped when no rule applies", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, body)
		called := false
		merge := func(context.Context, discovery.ConfigFile, *linter.Document) error {
			called = true
			return nil
		}

		if _, err := lintPath(t.Context(), linter.New(), merge, dir); err != nil {
			t.Fatalf("lintPath: %v", err)
		}
		if called {
			t.Error("merge ran although no rule is registered")
		}
	})

	t.Run("merge is skipped for a non-devcontainer file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "devcontainer-feature.json"), []byte(`{"id": "f"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		l := linter.New()
		// A Feature-type rule keeps HasRules true for the file, so only the type guard can skip the
		// merge.
		l.RegisterRule(&linter.Rule{
			ID:        "test-feature",
			FileTypes: []linter.FileType{linter.Feature},
			Paths:     []string{"/id"},
			Check:     func(*linter.Context, *linter.Node) []linter.Finding { return nil },
		}, linter.SeverityWarn)
		called := false
		merge := func(context.Context, discovery.ConfigFile, *linter.Document) error {
			called = true
			return nil
		}

		if _, err := lintPath(t.Context(), l, merge, dir); err != nil {
			t.Fatalf("lintPath: %v", err)
		}
		if called {
			t.Error("merge ran on a Feature configuration")
		}
	})
}

// hasRule reports whether the JSON issue array in data contains an issue for ruleID.
func hasRule(t *testing.T, data []byte, ruleID string) bool {
	t.Helper()
	var issues []linter.Issue
	if err := json.Unmarshal(data, &issues); err != nil {
		t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, data)
	}
	for _, issue := range issues {
		if issue.RuleID == ruleID {
			return true
		}
	}
	return false
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
