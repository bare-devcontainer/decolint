package format

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubWriteIssues(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	if err := (GitHubFormat{}).WriteIssues(&sb, testIssues()); err != nil {
		t.Fatalf("WriteIssues: %v", err)
	}

	want := `::warning file=.devcontainer/devcontainer.json,line=4,col=12,title=no-image-latest::image "ubuntu:latest" uses the "latest" tag; pin a specific version
::error file=.devcontainer/devcontainer.json,line=8,col=3,title=some-error-rule::something is broken
`
	if sb.String() != want {
		t.Errorf("WriteIssues github = %q, want %q", sb.String(), want)
	}
}

// TestGitHubWriteIssues_PathSeparators checks that a path built with the host's separators is
// written with "/" ones, which is what GitHub matches an annotation against.
func TestGitHubWriteIssues_PathSeparators(t *testing.T) {
	t.Parallel()

	issues := testIssues()[:1]
	issues[0].Path = filepath.Join(".devcontainer", "go", "devcontainer.json")

	var sb strings.Builder
	if err := (GitHubFormat{}).WriteIssues(&sb, issues); err != nil {
		t.Fatalf("WriteIssues: %v", err)
	}

	want := `::warning file=.devcontainer/go/devcontainer.json,line=4,col=12,title=no-image-latest::image "ubuntu:latest" uses the "latest" tag; pin a specific version
`
	if sb.String() != want {
		t.Errorf("WriteIssues github = %q, want %q", sb.String(), want)
	}
}

func TestGitHubWriteIssues_WriteError(t *testing.T) {
	t.Parallel()

	if err := (GitHubFormat{}).WriteIssues(errWriter{}, testIssues()); !errors.Is(err, errWrite) {
		t.Errorf("WriteIssues error = %v, want %v", err, errWrite)
	}
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
