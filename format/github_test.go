package format

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubWriteReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report Report
		want   string
	}{
		{
			name:   "files and issues",
			report: testReport(),
			want: `::group::decolint
Linted 2 files:
  .devcontainer/devcontainer.json (devcontainer)
  src/devcontainer-feature.json (feature)
::endgroup::
::warning file=.devcontainer/devcontainer.json,line=4,col=12,title=no-image-latest::image "ubuntu:latest" uses the "latest" tag; pin a specific version
::error file=.devcontainer/devcontainer.json,line=8,col=3,title=some-error-rule::something is broken
`,
		},
		{
			// Nothing linted means nothing to group, so no empty block is written.
			name:   "no files",
			report: Report{Issues: testIssues()[1:]},
			want: `::error file=.devcontainer/devcontainer.json,line=8,col=3,title=some-error-rule::something is broken
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sb strings.Builder
			if err := (GitHubFormat{}).WriteReport(&sb, tt.report); err != nil {
				t.Fatalf("WriteReport: %v", err)
			}
			if sb.String() != tt.want {
				t.Errorf("WriteReport github = %q, want %q", sb.String(), tt.want)
			}
		})
	}
}

// TestGitHubWriteReport_PathSeparators checks that a path built with the host's separators is
// written with "/" ones, which is what GitHub matches an annotation against.
func TestGitHubWriteReport_PathSeparators(t *testing.T) {
	t.Parallel()

	issues := testIssues()[:1]
	issues[0].Path = filepath.Join(".devcontainer", "go", "devcontainer.json")

	var sb strings.Builder
	if err := (GitHubFormat{}).WriteReport(&sb, Report{Issues: issues}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	want := `::warning file=.devcontainer/go/devcontainer.json,line=4,col=12,title=no-image-latest::image "ubuntu:latest" uses the "latest" tag; pin a specific version
`
	if sb.String() != want {
		t.Errorf("WriteReport github = %q, want %q", sb.String(), want)
	}
}

func TestGitHubWriteReport_WriteError(t *testing.T) {
	t.Parallel()

	assertWriteErrors(t, func(w io.Writer) error {
		return (GitHubFormat{}).WriteReport(w, testReport())
	})
}

func TestEscapeGitHubData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello world", "hello world"},
		{"percent", "100% done", "100%25 done"},
		{"cr", "a\rb", "a%0Db"},
		{"lf", "a\nb", "a%0Ab"},
		{"crlf", "a\r\nb", "a%0D%0Ab"},
		{"colon and comma preserved", "a:b,c", "a:b,c"},
		{"percent must be escaped first", "%0A", "%250A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := escapeGitHubData(tt.in); got != tt.want {
				t.Errorf("escapeGitHubData(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEscapeGitHubProperty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello world", "hello world"},
		{"percent", "100% done", "100%25 done"},
		{"cr", "a\rb", "a%0Db"},
		{"lf", "a\nb", "a%0Ab"},
		{"colon", "a:b", "a%3Ab"},
		{"comma", "a,b", "a%2Cb"},
		{"all special chars", "%\r\n:,", "%25%0D%0A%3A%2C"},
		{"percent must be escaped first", "%3A", "%253A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := escapeGitHubProperty(tt.in); got != tt.want {
				t.Errorf("escapeGitHubProperty(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
