package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/cmd/decolint/discovery"
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
			// deny-warnings.jsonc sets "denyWarnings": true in the config file, so the same security
			// warnings cross the fail threshold with no -deny-warnings flag.
			name: "deny-warnings from config",
			args: []string{"-config=testdata/e2e/deny-warnings.jsonc", "testdata/e2e/violations"},
			want: []firing{
				{violationsFile, "no-docker-socket-mount", linter.SeverityWarn},
				{violationsFile, "no-privileged-container", linter.SeverityWarn},
				{violationsFile, "no-seccomp-override", linter.SeverityWarn},
				{violationsFile, "require-cap-drop-all", linter.SeverityWarn},
				{violationsFile, "require-no-new-privileges", linter.SeverityWarn},
				{violationsFile, "require-non-root", linter.SeverityWarn},
			},
			wantExitCode: 1,
		},
		{
			// -deny-warnings=false, given explicitly, overrides deny-warnings.jsonc's
			// "denyWarnings": true, so the warnings no longer fail the run.
			name: "deny-warnings disabled by CLI flag overrides config",
			args: []string{"-deny-warnings=false", "-config=testdata/e2e/deny-warnings.jsonc", "testdata/e2e/violations"},
			want: []firing{
				{violationsFile, "no-docker-socket-mount", linter.SeverityWarn},
				{violationsFile, "no-privileged-container", linter.SeverityWarn},
				{violationsFile, "no-seccomp-override", linter.SeverityWarn},
				{violationsFile, "require-cap-drop-all", linter.SeverityWarn},
				{violationsFile, "require-no-new-privileges", linter.SeverityWarn},
				{violationsFile, "require-non-root", linter.SeverityWarn},
			},
			wantExitCode: 0, // warnings are reported but no longer cross the fail threshold
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
				{featureFile, "missing-feature-install-script", linter.SeverityError},
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
				{templateFile, "undefined-template-option", linter.SeverityError},
				// unused-template-option is a style rule, off by default, so it does not fire here even
				// though the fixture declares an unused option.
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

// TestRun_ReportedPathsAreWorkingDirectoryRelative checks that findings name files the same way
// however the lint target was named, and that a target outside the working directory is named
// absolutely.
func TestRun_ReportedPathsAreWorkingDirectoryRelative(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.

	const fixture = "testdata/e2e/violations"
	abs, err := filepath.Abs(fixture)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	reportedPaths := func(t *testing.T, target string) []string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		run(t.Context(), []string{"-format=json", "-platform=codespaces", target}, &stdout, &stderr)
		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
		}
		if len(issues) == 0 {
			t.Fatalf("no findings for %s; the fixture is expected to trip codespaces rules", target)
		}
		var paths []string
		for _, issue := range issues {
			paths = append(paths, issue.Path)
		}
		return paths
	}

	t.Run("a target inside the working directory is named relative to it", func(t *testing.T) {
		want := reportedPaths(t, fixture)
		if diff := cmp.Diff(want, reportedPaths(t, abs)); diff != "" {
			t.Errorf("paths from the absolute target differ from the relative one (-relative +absolute):\n%s", diff)
		}
		for _, p := range want {
			if !strings.HasPrefix(p, fixture) {
				t.Errorf("path = %q, want it under %q", p, fixture)
			}
		}
	})

	t.Run("a target outside the working directory is named absolutely", func(t *testing.T) {
		// Uses t.Chdir, which cannot be combined with t.Parallel.
		t.Chdir(t.TempDir())
		for _, p := range reportedPaths(t, abs) {
			if !filepath.IsAbs(p) {
				t.Errorf("path = %q, want an absolute path", p)
			}
		}
	})
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

	t.Run("-explain", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-explain=no-bind-mount"}, &stdout, &stderr)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		out := stdout.String()
		rule := builtinRule(t, "no-bind-mount")
		for _, want := range []string{
			rule.ID,
			rule.Category.String(),
			// The rule targets Codespaces, so the platform it is scoped to is named rather than "(all)".
			"codespaces",
			rule.Description,
			rules.DocsURL(rule.ID),
		} {
			if !strings.Contains(out, want) {
				t.Errorf("stdout = %q, want it to contain %q", out, want)
			}
		}
	})

	t.Run("-explain unknown rule", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-explain=no-such-rule"}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2", exitCode)
		}
		if stdout.String() != "" {
			t.Errorf("stdout = %q, want empty", stdout.String())
		}
		if !strings.Contains(stderr.String(), "no-such-rule") {
			t.Errorf("stderr = %q, want it to mention the unknown rule ID", stderr.String())
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

// The sarif* types decode the structural subset of a SARIF log the "sarif log" test asserts. The
// message texts and rule descriptions the format carries are left out on purpose: they are owned by
// the rule tests and the format package's own tests.
type (
	sarifLog struct {
		Version string     `json:"version"`
		Runs    []sarifRun `json:"runs"`
	}
	sarifRun struct {
		Tool    sarifTool     `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	sarifTool struct {
		Driver sarifDriver `json:"driver"`
	}
	sarifDriver struct {
		Name  string      `json:"name"`
		Rules []sarifRule `json:"rules"`
	}
	sarifRule struct {
		ID string `json:"id"`
	}
	sarifResult struct {
		RuleID    string          `json:"ruleId"`
		RuleIndex int             `json:"ruleIndex"`
		Level     string          `json:"level"`
		Locations []sarifLocation `json:"locations"`
	}
	sarifLocation struct {
		PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
	}
	sarifPhysicalLocation struct {
		ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	}
	sarifArtifactLocation struct {
		URI string `json:"uri"`
	}
)

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

	t.Run("sarif log", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-format=sarif", "-platform=vscode,codespaces", violationsDir}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}

		var log sarifLog
		if err := json.Unmarshal(stdout.Bytes(), &log); err != nil {
			t.Fatalf("output is not a SARIF log: %v\noutput: %s", err, stdout.String())
		}

		// The fixture is inside the working directory, so it is reported relative to it.
		locAt := func(uri string) []sarifLocation {
			return []sarifLocation{{PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: uri},
			}}}
		}
		// The catalog lists only referenced rules, sorted by ID, so each result's ruleIndex points at
		// its like-named catalog entry.
		want := sarifLog{
			Version: "2.1.0",
			Runs: []sarifRun{{
				Tool: sarifTool{Driver: sarifDriver{
					Name:  "decolint",
					Rules: []sarifRule{{ID: "no-bind-mount"}, {ID: "no-host-port-format"}},
				}},
				Results: []sarifResult{
					{RuleID: "no-bind-mount", RuleIndex: 0, Level: "error", Locations: locAt(violationsFile)},
					{RuleID: "no-host-port-format", RuleIndex: 1, Level: "error", Locations: locAt(violationsFile)},
				},
			}},
		}
		sortResults := cmpopts.SortSlices(func(a, b sarifResult) bool { return a.RuleID < b.RuleID })
		if diff := cmp.Diff(want, log, sortResults); diff != "" {
			t.Errorf("sarif log mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("sarif log outside the working directory", func(t *testing.T) {
		t.Parallel()

		// A directory outside the working directory is reported absolutely, which SARIF can only
		// express as an absolute file URI. The fixture trips missing-container-def, an error by
		// default and free of any platform or fetch.
		dir := writeDevcontainer(t, `{}`)

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-format=sarif", dir}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}

		var log sarifLog
		if err := json.Unmarshal(stdout.Bytes(), &log); err != nil {
			t.Fatalf("output is not a SARIF log: %v\noutput: %s", err, stdout.String())
		}

		// A file URI's path carries exactly one leading slash, which a POSIX path already has and a
		// Windows one does not.
		config := filepath.Join(dir, ".devcontainer", "devcontainer.json")
		uri := "file://" + path.Join("/", filepath.ToSlash(config))
		want := sarifLog{
			Version: "2.1.0",
			Runs: []sarifRun{{
				Tool: sarifTool{Driver: sarifDriver{
					Name:  "decolint",
					Rules: []sarifRule{{ID: "missing-container-def"}},
				}},
				Results: []sarifResult{{
					RuleID:    "missing-container-def",
					RuleIndex: 0,
					Level:     "error",
					Locations: []sarifLocation{{PhysicalLocation: sarifPhysicalLocation{
						ArtifactLocation: sarifArtifactLocation{URI: uri},
					}}},
				}},
			}},
		}
		if diff := cmp.Diff(want, log); diff != "" {
			t.Errorf("sarif log mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("format selected by config file", func(t *testing.T) {
		t.Parallel()

		// format.jsonc sets "format": "json", so output is a JSON array with no -format flag.
		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-config=testdata/e2e/format.jsonc", violationsDir}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("config-selected json format did not produce a JSON array: %v\noutput: %s", err, stdout.String())
		}
	})

	t.Run("format flag overrides config file", func(t *testing.T) {
		t.Parallel()

		// -format=text wins over format.jsonc's "format": "json", so output is the text format.
		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-format=text", "-config=testdata/e2e/format.jsonc", violationsDir}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Found 2 errors and 0 warnings.") {
			t.Errorf("want text output; got:\n%s", stdout.String())
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

func TestRunLint_DeduplicatesTargets(t *testing.T) {
	t.Parallel()

	dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)
	cfg := Config{
		Format: "json",
		Rules:  map[string]linter.Severity{"no-image-latest": linter.SeverityError},
	}

	lint := func(t *testing.T, paths []string) int {
		t.Helper()
		var stdout bytes.Buffer
		if _, err := runLint(t.Context(), &stdout, io.Discard, Options{Paths: paths}, cfg); err != nil {
			t.Fatalf("runLint: %v", err)
		}
		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
		}
		return len(issues)
	}

	want := lint(t, []string{dir})
	if want == 0 {
		t.Fatal("no findings for the fixture; the test cannot tell deduplication from a silent lint")
	}
	if got := lint(t, []string{dir, dir + string(filepath.Separator)}); got != want {
		t.Errorf("findings for the directory named twice = %d, want %d", got, want)
	}
}

func TestRunLint_UnresolvableTarget(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.

	// A target whose location cannot be resolved is recorded like any other per-target failure, so
	// the findings collected so far are still written.
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	hasIssue, err := runLint(t.Context(), &stdout, io.Discard, Options{Paths: []string{"."}}, Config{Format: "json"})
	if err == nil || !strings.Contains(err.Error(), "resolve directory") {
		t.Errorf("err = %v, want a directory resolution error", err)
	}
	if hasIssue {
		t.Error("hasIssue = true, want false")
	}
	if got := strings.TrimSpace(stdout.String()); got != "[]" {
		t.Errorf("stdout = %q, want an empty JSON issue array", got)
	}
}

func TestRunLint(t *testing.T) {
	t.Parallel()

	t.Run("no config file, exit code unchanged", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout bytes.Buffer
		hasIssue, runErr := runLint(t.Context(), &stdout, io.Discard, Options{Paths: []string{dir}}, Config{})
		if runErr != nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("config promotes a warn rule to error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout bytes.Buffer
		opts := Options{Paths: []string{dir}}
		cfg := Config{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, io.Discard, opts, cfg)
		if runErr != nil || !hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want true, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("config disables a rule that defaults to error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{}`) // triggers missing-container-def (error by default)

		var stdout bytes.Buffer
		opts := Options{Paths: []string{dir}}
		cfg := Config{Rules: map[string]linter.Severity{"missing-container-def": linter.SeverityOff}}
		hasIssue, runErr := runLint(t.Context(), &stdout, io.Discard, opts, cfg)
		if runErr != nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("unknown rule ID in config", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		opts := Options{}
		cfg := Config{Rules: map[string]linter.Severity{"no-image-latst": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, io.Discard, opts, cfg)
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
		opts := Options{Paths: []string{dir}}
		cfg := Config{Categories: map[string]linter.Severity{"reproducibility": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, io.Discard, opts, cfg)
		if runErr != nil || !hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want true, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})

	t.Run("unknown category name in config", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		opts := Options{}
		cfg := Config{Categories: map[string]linter.Severity{"secure": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, io.Discard, opts, cfg)
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
		hasIssue, runErr := runLint(t.Context(), &stdout, io.Discard, Options{Paths: []string{file}}, Config{})
		if runErr == nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, 'not a directory'", hasIssue, runErr)
		}
	})

	t.Run("override for unselected platform-scoped rule is not an error", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, `{"image": "ubuntu:latest"}`)

		var stdout bytes.Buffer
		opts := Options{Paths: []string{dir}}
		cfg := Config{Rules: map[string]linter.Severity{"no-bind-mount": linter.SeverityError}}
		hasIssue, runErr := runLint(t.Context(), &stdout, io.Discard, opts, cfg)
		if runErr != nil || hasIssue {
			t.Errorf("hasIssue = %v, err = %v, want false, nil; stdout: %s", hasIssue, runErr, stdout.String())
		}
	})
}

func TestRun_Merge(t *testing.T) {
	t.Parallel()

	t.Run("findings point at the feature reference", func(t *testing.T) {
		t.Parallel()

		host := ocitest.Registry(t)
		dir := copyFixture(t, "testdata/e2e/merge", map[string]string{"${BASE_IMAGE}": baseImageRef(t, host)})

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=testdata/e2e/merge.jsonc", dir}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}

		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
		}
		// The config references "./privileged-feature" on line 4; every merged-in property must be
		// reported there rather than at the base image on line 2.
		for _, issue := range issues {
			if issue.Line != 4 {
				t.Errorf("issue %s reported at line %d, want 4 (the feature reference)", issue.RuleID, issue.Line)
			}
		}
	})

	t.Run("merge on and off is controlled by flag and config", func(t *testing.T) {
		t.Parallel()

		// Each case pairs the same merged config with a different flag/config combination; the two
		// security rules fire only when merging is on. merge.jsonc enables them via the -merge flag,
		// merge-on.jsonc enables them and turns merging on through its own "merge" member.
		tests := []struct {
			name         string
			args         []string
			wantExitCode int
			wantMerged   bool
		}{
			{"flag enables merge", []string{"-merge", "-config=testdata/e2e/merge.jsonc"}, 1, true},
			{"no flag leaves merge off", []string{"-config=testdata/e2e/merge.jsonc"}, 0, false},
			{"config enables merge", []string{"-config=testdata/e2e/merge-on.jsonc"}, 1, true},
			{"flag overrides config merge", []string{"-merge=false", "-config=testdata/e2e/merge-on.jsonc"}, 0, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				host := ocitest.Registry(t)
				dir := copyFixture(t, "testdata/e2e/merge", map[string]string{"${BASE_IMAGE}": baseImageRef(t, host)})

				var stdout, stderr bytes.Buffer
				exitCode := run(t.Context(), append([]string{"-format=json"}, append(tt.args, dir)...), &stdout, &stderr)
				if exitCode != tt.wantExitCode {
					t.Errorf("exit code = %d, want %d; stdout: %s", exitCode, tt.wantExitCode, stdout.String())
				}
				for _, ruleID := range []string{"no-privileged-container", "no-docker-socket-mount"} {
					if got := hasRule(t, stdout.Bytes(), ruleID); got != tt.wantMerged {
						t.Errorf("hasRule(%s) = %v, want %v (merged=%v)", ruleID, got, tt.wantMerged, tt.wantMerged)
					}
				}
			})
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
		// is generated here rather than kept as a static fixture. The base image is a loopback
		// reference to the same in-process registry, so resolving its metadata stays hermetic.
		body := fmt.Sprintf(`{"image": %q, "features": {%q: {}}}`, baseImageRef(t, host), host+"/features/privileged:1")
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
		// must surface as exit code 2 rather than a lint result. The base image is a loopback
		// reference so it is not what fails.
		host := ocitest.Registry(t)
		body := fmt.Sprintf(`{"image": %q, "features": {"https://features.invalid/f.tgz": {}}}`, baseImageRef(t, host))
		dir := writeDevcontainer(t, body)

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
		host := ocitest.Registry(t)
		dir := writeDevcontainer(t, fmt.Sprintf(`{"image": %q, "features": {"./a": {}}}`, baseImageRef(t, host)))
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
		host := ocitest.Registry(t)
		dir := writeDevcontainer(t, fmt.Sprintf(`{"image": %q, "features": {"./missing": {}}}`, baseImageRef(t, host)))

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
		host := ocitest.Registry(t)
		dir := writeDevcontainer(t, fmt.Sprintf(`{"image": %q, "features": {"../sibling-feature": {}}}`, baseImageRef(t, host)))
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

	t.Run("merges base image metadata from an OCI registry", func(t *testing.T) {
		t.Parallel()

		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{
			"devcontainer.metadata": `[{"privileged": true, "mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]}]`,
		}, false)

		body := fmt.Sprintf(`{"image": %q}`, host+"/base:1")
		dir := writeDevcontainer(t, body)

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=testdata/e2e/merge.jsonc", dir}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		for _, ruleID := range []string{"no-privileged-container", "no-docker-socket-mount"} {
			if !hasRule(t, stdout.Bytes(), ruleID) {
				t.Errorf("want %s to fire on the merged image metadata; output: %s", ruleID, stdout.String())
			}
		}
		// The merged-in properties are reported at the "image" reference on line 1.
		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
		}
		for _, issue := range issues {
			if issue.Line != 1 {
				t.Errorf("issue %s reported at line %d, want 1 (the image reference)", issue.RuleID, issue.Line)
			}
		}
	})

	t.Run("an unreachable base image is a runtime error", func(t *testing.T) {
		t.Parallel()
		// The reserved .invalid TLD never resolves, so fetching the image fails and the failure must
		// surface as exit code 2 rather than a lint result.
		dir := writeDevcontainer(t, `{"image": "registry.invalid/base:1", "features": {"./f": {}}}`)
		featureDir := filepath.Join(dir, ".devcontainer", "f")
		if err := os.MkdirAll(featureDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(featureDir, "devcontainer-feature.json"), []byte(`{"id": "f"}`), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-merge", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2; stdout: %s", exitCode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "registry.invalid") {
			t.Errorf("stderr = %q, want it to mention the unreachable image", stderr.String())
		}
	})

	t.Run("merges metadata from a build Dockerfile LABEL", func(t *testing.T) {
		t.Parallel()

		// The Dockerfile's own LABEL carries the metadata, so the run is hermetic: FROM scratch needs
		// no registry. The "dockerfile" key sits on line 3, where every merged-in finding must anchor.
		body := "{\n  \"build\": {\n    \"dockerfile\": \"Dockerfile\"\n  }\n}"
		dir := writeDevcontainer(t, body)
		writeDockerfile(t, dir, `FROM scratch
LABEL devcontainer.metadata='[{"privileged": true, "mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]}]'
`)

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=testdata/e2e/merge.jsonc", dir}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		for _, ruleID := range []string{"no-privileged-container", "no-docker-socket-mount"} {
			if !hasRule(t, stdout.Bytes(), ruleID) {
				t.Errorf("want %s to fire on the merged Dockerfile metadata; output: %s", ruleID, stdout.String())
			}
		}
		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
		}
		for _, issue := range issues {
			if issue.Line != 3 {
				t.Errorf("issue %s reported at line %d, want 3 (the dockerfile reference)", issue.RuleID, issue.Line)
			}
		}
	})

	t.Run("merges base image metadata through a Dockerfile FROM", func(t *testing.T) {
		t.Parallel()

		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{
			"devcontainer.metadata": `[{"privileged": true, "mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]}]`,
		}, false)

		dir := writeDevcontainer(t, `{"build": {"dockerfile": "Dockerfile"}}`)
		writeDockerfile(t, dir, fmt.Sprintf("FROM %s/base:1\n", host))

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=testdata/e2e/merge.jsonc", dir}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		for _, ruleID := range []string{"no-privileged-container", "no-docker-socket-mount"} {
			if !hasRule(t, stdout.Bytes(), ruleID) {
				t.Errorf("want %s to fire on the inherited base image metadata; output: %s", ruleID, stdout.String())
			}
		}
	})

	t.Run("a Dockerfile base image fetch failure is a runtime error", func(t *testing.T) {
		t.Parallel()
		// The base image named by FROM never resolves, so building the Dockerfile fails and the
		// failure must surface as exit code 2 rather than a lint result.
		dir := writeDevcontainer(t, `{"build": {"dockerfile": "Dockerfile"}}`)
		writeDockerfile(t, dir, "FROM registry.invalid/base:1\n")

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-merge", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2; stdout: %s", exitCode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "registry.invalid") {
			t.Errorf("stderr = %q, want it to mention the unreachable base image", stderr.String())
		}
	})

	t.Run("merges metadata through a Compose service image", func(t *testing.T) {
		t.Parallel()

		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{
			"devcontainer.metadata": `[{"privileged": true, "mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]}]`,
		}, false)

		dir := writeDevcontainer(t, `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`)
		writeComposeFile(t, dir, fmt.Sprintf("services:\n  app:\n    image: %s/base:1\n", host))

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=testdata/e2e/merge.jsonc", dir}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		for _, ruleID := range []string{"no-privileged-container", "no-docker-socket-mount"} {
			if !hasRule(t, stdout.Bytes(), ruleID) {
				t.Errorf("want %s to fire on the merged Compose service image metadata; output: %s", ruleID, stdout.String())
			}
		}
	})

	t.Run("interpolates a Compose variable from the config localEnv", func(t *testing.T) {
		t.Parallel()

		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{
			"devcontainer.metadata": `[{"privileged": true}]`,
		}, false)

		// The Compose file names its image through "${IMAGE}", which resolves only from the config's
		// localEnv map: without the localEnv threaded into merge it would interpolate to empty and
		// contribute nothing, so this run also proves the wiring.
		dir := writeDevcontainer(t, `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`)
		writeComposeFile(t, dir, "services:\n  app:\n    image: ${IMAGE}\n")
		config := filepath.Join(t.TempDir(), "decolint.jsonc")
		body := fmt.Sprintf(`{"rules": {"no-privileged-container": "error"}, "localEnv": {"IMAGE": %q}}`, host+"/base:1")
		if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=" + config, dir}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		if !hasRule(t, stdout.Bytes(), "no-privileged-container") {
			t.Errorf("want no-privileged-container to fire on the interpolated Compose image metadata; output: %s", stdout.String())
		}
	})

	t.Run("merges metadata through a Compose service Dockerfile, anchored at dockerComposeFile", func(t *testing.T) {
		t.Parallel()

		body := "{\n" +
			`  "dockerComposeFile": "docker-compose.yml",` + "\n" +
			`  "service": "app"` + "\n}"
		dir := writeDevcontainer(t, body)
		writeComposeFile(t, dir, "services:\n  app:\n    build:\n      context: .\n")
		writeDockerfile(t, dir, `FROM scratch
LABEL devcontainer.metadata='[{"privileged": true, "mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"]}]'
`)

		var stdout, stderr bytes.Buffer
		args := []string{"-format=json", "-merge", "-config=testdata/e2e/merge.jsonc", dir}
		exitCode := run(t.Context(), args, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		for _, ruleID := range []string{"no-privileged-container", "no-docker-socket-mount"} {
			if !hasRule(t, stdout.Bytes(), ruleID) {
				t.Errorf("want %s to fire on the merged Compose Dockerfile metadata; output: %s", ruleID, stdout.String())
			}
		}
		// The config references the Compose file on line 2; every merged-in property must be reported
		// there rather than at the service or the Dockerfile.
		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
		}
		for _, issue := range issues {
			if issue.Line != 2 {
				t.Errorf("issue %s reported at line %d, want 2 (the dockerComposeFile reference)", issue.RuleID, issue.Line)
			}
		}
	})

	t.Run("a Compose service image fetch failure is a runtime error", func(t *testing.T) {
		t.Parallel()
		// The service names an image that never resolves, so fetching its metadata fails and the
		// failure must surface as exit code 2 rather than a lint result.
		dir := writeDevcontainer(t, `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`)
		writeComposeFile(t, dir, "services:\n  app:\n    image: registry.invalid/base:1\n")

		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-merge", dir}, &stdout, &stderr)
		if exitCode != 2 {
			t.Errorf("exit code = %d, want 2; stdout: %s", exitCode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "registry.invalid") {
			t.Errorf("stderr = %q, want it to mention the unreachable base image", stderr.String())
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
	noSubst := func(string, *linter.Document) {}
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

		issues, err := lintPath(t.Context(), newLinter(), noSubst, nil, absPath(dir))
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
		issues, err := lintDir(t.Context(), newLinter(), noSubst, nil, root)
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
		_, err := lintPath(t.Context(), newLinter(), noSubst, nil, absPath(t.TempDir()))
		if err == nil || !strings.Contains(err.Error(), "no devcontainer configuration found") {
			t.Errorf("err = %v, want 'no devcontainer configuration found'", err)
		}
	})

	t.Run("a configuration file is not a directory", func(t *testing.T) {
		t.Parallel()
		file := filepath.Join(writeDevcontainer(t, body), ".devcontainer", "devcontainer.json")
		_, err := lintPath(t.Context(), newLinter(), noSubst, nil, absPath(file))
		if err == nil || !strings.Contains(err.Error(), "is not a directory; pass the directory") {
			t.Errorf("err = %v, want the error to say a directory is expected", err)
		}
	})

	t.Run("merge error aborts the file", func(t *testing.T) {
		t.Parallel()
		dir := writeDevcontainer(t, body)
		wantErr := errors.New("fetch failed")
		merge := func(context.Context, discovery.ConfigFile, *linter.Document) error { return wantErr }

		if _, err := lintPath(t.Context(), newLinter(), noSubst, merge, absPath(dir)); !errors.Is(err, wantErr) {
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

		if _, err := lintPath(t.Context(), linter.New(), noSubst, merge, absPath(dir)); err != nil {
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
		subst := func(string, *linter.Document) {}

		if _, err := lintPath(t.Context(), l, subst, merge, absPath(dir)); err != nil {
			t.Fatalf("lintPath: %v", err)
		}
		if called {
			t.Error("merge ran on a Feature configuration")
		}
	})
}

// dirNameRule is a stub rule that reports the name of the directory the linted file sits in, used to
// observe the [linter.Dir] a lint hands to rules without depending on a rule that reads it.
var dirNameRule = &linter.Rule{
	ID:          "test-dir-name",
	Description: "reports the containing directory's name",
	FileTypes:   []linter.FileType{linter.Feature},
	Paths:       []string{"/id"},
	Check: func(ctx *linter.Context, node *linter.Node) []linter.Finding {
		return []linter.Finding{{Message: ctx.Dir.Name, Offset: node.Value.StartOffset}}
	},
}

// TestLintPath_DirName checks that rules are handed the name of the directory the configuration file
// sits in wherever the lint runs from, while the path the finding is reported at follows the working
// directory. Linting a directory from inside it leaves that path with no directory component, so the
// name cannot be taken from there.
func TestLintPath_DirName(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.

	parent := t.TempDir()
	// t.TempDir can hand back a path through a symlink (/var on macOS), which is not the path the
	// process reports as its working directory; ask for the one findings are reported against.
	t.Chdir(parent)
	parent, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	target := filepath.Join(parent, "my-feature")
	sibling := filepath.Join(parent, "elsewhere")
	for _, dir := range []string{target, sibling} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "devcontainer-feature.json"), []byte(`{"id": "my-feature"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		wd       string
		wantPath string
	}{
		{"from the directory itself", target, "devcontainer-feature.json"},
		{"from its parent", parent, filepath.Join("my-feature", "devcontainer-feature.json")},
		{"from outside", sibling, filepath.Join(target, "devcontainer-feature.json")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(tt.wd)
			l := linter.New()
			l.RegisterRule(dirNameRule, linter.SeverityWarn)

			issues, err := lintPath(t.Context(), l, nil, nil, absPath(target))
			if err != nil {
				t.Fatalf("lintPath: %v", err)
			}
			if len(issues) != 1 {
				t.Fatalf("got %d issues %v, want 1", len(issues), issues)
			}
			if issues[0].Message != "my-feature" {
				t.Errorf("ctx.Dir.Name = %q, want %q", issues[0].Message, "my-feature")
			}
			if issues[0].Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", issues[0].Path, tt.wantPath)
			}
		})
	}
}

func TestAbsPathString(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.

	wd := t.TempDir()
	// t.TempDir can hand back a path through a symlink (/var on macOS), which is not the path the
	// process reports as its working directory; ask for the one absPath renders against.
	t.Chdir(wd)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	tests := []struct {
		name string
		path absPath
		want string
	}{
		{"inside the working directory", absPath(filepath.Join(wd, ".devcontainer", "devcontainer.json")), filepath.Join(".devcontainer", "devcontainer.json")},
		{"the working directory itself", absPath(wd), "."},
		{"the parent of the working directory", absPath(filepath.Dir(wd)), filepath.Dir(wd)},
		{"a sibling of the working directory", absPath(filepath.Join(filepath.Dir(wd), "elsewhere")), filepath.Join(filepath.Dir(wd), "elsewhere")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.path.String(); got != tt.want {
				t.Errorf("absPath(%q).String() = %q, want %q", string(tt.path), got, tt.want)
			}
		})
	}
}

func TestAbsPathString_NoWorkingDirectory(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.

	// With the working directory gone there is nothing to render against, so the path stays absolute.
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}

	p := absPath(filepath.Join(dir, "devcontainer.json"))
	if got := p.String(); got != string(p) {
		t.Errorf("absPath(%q).String() = %q, want it unchanged", string(p), got)
	}
}

// TestRun_Substitution checks that ${...} variables resolve only under -merge, since substitution
// and merging together compute the effective configuration: with merging on, no-image-latest
// reports the resolved image reference; with merging off, it reports the raw ${localEnv:...} text.
func TestRun_Substitution(t *testing.T) {
	t.Parallel()

	// The image resolves against the in-process registry so a merge run stays hermetic; its "latest"
	// tag is what trips no-image-latest.
	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "latest", nil, false)
	resolved := host + "/base:latest"

	dir := writeDevcontainer(t, `{"image": "${localEnv:REGISTRY}/base:latest"}`)

	lintImage := func(t *testing.T, mergeMember string) linter.Issue {
		t.Helper()
		config := fmt.Sprintf(`{%s"localEnv": {"REGISTRY": %q}, "rules": {"no-image-latest": "error"}}`, mergeMember, host)
		path := filepath.Join(t.TempDir(), "config.jsonc")
		if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		exitCode := run(t.Context(), []string{"-format=json", "-config=" + path, dir}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1; stderr: %s", exitCode, stderr.String())
		}
		var issues []linter.Issue
		if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
			t.Fatalf("output is not a JSON issue array: %v\noutput: %s", err, stdout.String())
		}
		if len(issues) != 1 || issues[0].RuleID != "no-image-latest" {
			t.Fatalf("issues = %v, want one no-image-latest", issues)
		}
		return issues[0]
	}

	t.Run("merge resolves the variable", func(t *testing.T) {
		t.Parallel()
		issue := lintImage(t, `"merge": true, `)
		if !strings.Contains(issue.Message, `"`+resolved+`"`) {
			t.Errorf("message = %q, want it to name the resolved image %q", issue.Message, resolved)
		}
	})

	t.Run("without merge the variable is left as written", func(t *testing.T) {
		t.Parallel()
		issue := lintImage(t, "")
		if !strings.Contains(issue.Message, "${localEnv:REGISTRY}") {
			t.Errorf("message = %q, want it to keep the unresolved variable", issue.Message)
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

// builtinRule returns the built-in rule with the given ID, failing the test if there is none.
func builtinRule(t *testing.T, id string) *linter.Rule {
	t.Helper()
	for _, reg := range rules.Builtin() {
		if reg.Rule.ID == id {
			return reg.Rule
		}
	}
	t.Fatalf("no built-in rule with ID %q", id)
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

// writeDockerfile writes content as the .devcontainer/Dockerfile of the devcontainer directory dir
// created by writeDevcontainer, the path a "build.dockerfile" of "Dockerfile" resolves to.
func writeDockerfile(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".devcontainer", "Dockerfile")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// writeComposeFile writes content as the .devcontainer/docker-compose.yml of the devcontainer
// directory dir created by writeDevcontainer, the path a "dockerComposeFile" of
// "docker-compose.yml" resolves to.
func writeComposeFile(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".devcontainer", "docker-compose.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// baseImageRef pushes a metadata-less base image to host and returns its reference. Using it as the
// "image" property keeps a merge run hermetic: the reference resolves against the in-process
// registry rather than a public one, so decolint fetches the image's (empty) metadata without
// leaving the loopback interface. This is why merge test configs name a loopback image instead of a
// real one like "ubuntu:24.04" — the base image must resolve, and it must resolve locally.
func baseImageRef(t *testing.T, host string) string {
	t.Helper()
	ocitest.PushImage(t, host, "base", "1", nil, false)
	return host + "/base:1"
}

// copyFixture materializes the fixture tree rooted at src into a fresh temp directory, replacing
// every key of subst with its value in each file's contents, and returns that directory. It lets a
// test keep a readable, checked-in devcontainer fixture while injecting values known only at run
// time, such as a loopback registry reference: the fixture holds a "${BASE_IMAGE}" placeholder and
// the substitution writes the real reference into the copy decolint reads.
func copyFixture(t *testing.T, src string, subst map[string]string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relativize %s: %w", path, err)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		content := string(data)
		for old, replacement := range subst {
			content = strings.ReplaceAll(content, old, replacement)
		}
		return os.WriteFile(target, []byte(content), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}
