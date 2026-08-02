package format

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
)

// errWriter is an io.Writer that always fails, used to exercise the write-error paths of the
// formatters.
type errWriter struct{}

var errWrite = errors.New("write failed")

func (errWriter) Write([]byte) (int, error) { return 0, errWrite }

// countingWriter counts the writes made to it, all of which succeed.
type countingWriter struct{ n int }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n++
	return len(p), nil
}

// failingWriter succeeds for ok writes and fails from then on.
type failingWriter struct{ ok int }

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.ok <= 0 {
		return 0, errWrite
	}
	w.ok--
	return len(p), nil
}

// assertWriteErrors calls write once per write it makes, failing a different one each time, and
// checks that every one is reported. Covering the write points this way keeps the assertion honest
// as a format grows lines rather than enumerating them by hand.
func assertWriteErrors(t *testing.T, write func(io.Writer) error) {
	t.Helper()

	var counter countingWriter
	if err := write(&counter); err != nil {
		t.Fatalf("write: %v", err)
	}
	if counter.n == 0 {
		t.Fatal("write made no writes, so no write error can be exercised")
	}
	for n := range counter.n {
		if err := write(&failingWriter{ok: n}); !errors.Is(err, errWrite) {
			t.Errorf("write %d of %d failing: error = %v, want %v", n+1, counter.n, err, errWrite)
		}
	}
}

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

func testReport() Report {
	return Report{
		ConfigPath: ".decolint.jsonc",
		Files: []File{
			{Path: ".devcontainer/devcontainer.json", Type: linter.Devcontainer},
			{Path: "src/devcontainer-feature.json", Type: linter.Feature},
		},
		Issues: testIssues(),
	}
}

func TestTextWriteReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report Report
		want   string
	}{
		{
			name:   "config file, files and issues",
			report: testReport(),
			want: `Config: .decolint.jsonc
Linted 2 files:
  .devcontainer/devcontainer.json (devcontainer)
  src/devcontainer-feature.json (feature)

.devcontainer/devcontainer.json:4:12: warn: image "ubuntu:latest" uses the "latest" tag; pin a specific version (no-image-latest)
.devcontainer/devcontainer.json:8:3: error: something is broken (some-error-rule)
Found 1 error and 1 warning.
`,
		},
		{
			// A file with no finding is still reported, so a clean run shows what it covered.
			name: "no config file, one clean file",
			report: Report{
				Files: []File{{Path: ".devcontainer.json", Type: linter.Devcontainer}},
			},
			want: `Config: none (defaults; run "decolint --init" to create .decolint.jsonc)
Linted 1 file:
  .devcontainer.json (devcontainer)

Found 0 errors and 0 warnings.
`,
		},
		{
			name:   "nothing linted",
			report: Report{ConfigPath: ".decolint.json"},
			want: `Config: .decolint.json

Found 0 errors and 0 warnings.
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sb strings.Builder
			if err := (TextFormat{}).WriteReport(&sb, tt.report); err != nil {
				t.Fatalf("WriteReport: %v", err)
			}
			if sb.String() != tt.want {
				t.Errorf("WriteReport text = %q, want %q", sb.String(), tt.want)
			}
		})
	}
}

// ANSI escape sequences the colored text report is made of; off ends the innermost one.
const (
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiGreen  = "\x1b[32m"
	ansiRed    = "\x1b[31;1m"
	ansiYellow = "\x1b[33;1m"
	ansiOff    = "\x1b[0m"
)

func TestTextWriteReport_Color(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report Report
		want   []string
	}{
		{
			name:   "the severity of each issue is colored and its position stands out",
			report: testReport(),
			want: []string{
				ansiBold + "Config:" + ansiOff + " .decolint.jsonc",
				ansiBold + "Linted 2 files:" + ansiOff,
				"  .devcontainer/devcontainer.json " + ansiDim + "(devcontainer)" + ansiOff,
				"  src/devcontainer-feature.json " + ansiDim + "(feature)" + ansiOff,
				"",
				ansiBold + ".devcontainer/devcontainer.json:4:12" + ansiOff + ": " + ansiYellow + "warn" + ansiOff +
					`: image "ubuntu:latest" uses the "latest" tag; pin a specific version ` + ansiDim + "(no-image-latest)" + ansiOff,
				ansiBold + ".devcontainer/devcontainer.json:8:3" + ansiOff + ": " + ansiRed + "error" + ansiOff +
					": something is broken " + ansiDim + "(some-error-rule)" + ansiOff,
				"Found " + ansiRed + "1 error" + ansiOff + " and " + ansiYellow + "1 warning" + ansiOff + ".",
				"",
			},
		},
		{
			// A run with nothing to report is colored as a whole, so it is recognizable without
			// reading the counts.
			name:   "a clean run is summarized in green",
			report: Report{Files: []File{{Path: ".devcontainer.json", Type: linter.Devcontainer}}},
			want: []string{
				ansiBold + "Config:" + ansiOff + " " + ansiDim + `none (defaults; run "decolint --init" to create .decolint.jsonc)` + ansiOff,
				ansiBold + "Linted 1 file:" + ansiOff,
				"  .devcontainer.json " + ansiDim + "(devcontainer)" + ansiOff,
				"",
				ansiGreen + "Found 0 errors and 0 warnings." + ansiOff,
				"",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sb strings.Builder
			if err := (TextFormat{Color: true}).WriteReport(&sb, tt.report); err != nil {
				t.Fatalf("WriteReport: %v", err)
			}
			want := strings.Join(tt.want, "\n")
			if sb.String() != want {
				t.Errorf("WriteReport text = %q, want %q", sb.String(), want)
			}
		})
	}
}

// TestTextWriteReport_Uncolored checks that an undecorated issue line is the one
// [linter.Issue.String] returns, so that the two renderings of a finding cannot drift apart.
func TestTextWriteReport_Uncolored(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := (TextFormat{}).WriteReport(&sb, testReport()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	for _, issue := range testIssues() {
		if !strings.Contains(sb.String(), issue.String()+"\n") {
			t.Errorf("WriteReport text = %q, want it to contain the line %q", sb.String(), issue.String())
		}
	}
	if strings.Contains(sb.String(), "\x1b") {
		t.Errorf("WriteReport text = %q, want no escape sequence in it", sb.String())
	}
}

func TestTextWriteReport_WriteError(t *testing.T) {
	t.Parallel()

	assertWriteErrors(t, func(w io.Writer) error {
		return (TextFormat{}).WriteReport(w, testReport())
	})
}
