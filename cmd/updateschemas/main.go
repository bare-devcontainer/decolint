// Command updateschemas refreshes the vendored Dev Container schemas in schema/data from upstream.
//
// It resolves the current tip of devcontainers/spec@main and microsoft/vscode@main, downloads the
// schema files at those commits, and rewrites schema/data/*.json and schema/data/REVISIONS.json. The
// schema-sync workflow runs it and fails when the result differs from what is committed, signalling
// that the vendored copies are stale.
//
// Run it from the repository root: go run ./cmd/updateschemas.
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	specRepo   = "https://github.com/devcontainers/spec.git"
	vscodeRepo = "https://github.com/microsoft/vscode.git"
	dataDir    = "schema/data"
)

// file describes one vendored schema: its local name and the upstream path, relative to a commit, it
// is fetched from.
type file struct {
	name   string
	repo   string // "spec" or "vscode"
	upPath string
}

var files = []file{
	{"devContainer.base.schema.json", "spec", "schemas/devContainer.base.schema.json"},
	{"devContainer.schema.json", "spec", "schemas/devContainer.schema.json"},
	{"devContainerFeature.schema.json", "spec", "schemas/devContainerFeature.schema.json"},
	{"devContainer.codespaces.schema.json", "vscode", "extensions/configuration-editing/schemas/devContainer.codespaces.schema.json"},
	{"devContainer.vscode.schema.json", "vscode", "extensions/configuration-editing/schemas/devContainer.vscode.schema.json"},
}

// revisions mirrors schema/data/REVISIONS.json.
type revisions struct {
	Spec    string            `json:"spec"`
	VSCode  string            `json:"vscode"`
	Sources map[string]string `json:"sources"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "updateschemas:", err)
		os.Exit(1)
	}
}

func run() error {
	specSHA, err := headSHA(specRepo)
	if err != nil {
		return err
	}
	vscodeSHA, err := headSHA(vscodeRepo)
	if err != nil {
		return err
	}
	sha := map[string]string{"spec": specSHA, "vscode": vscodeSHA}

	rev := revisions{Spec: specSHA, VSCode: vscodeSHA, Sources: map[string]string{}}
	for _, f := range files {
		url := rawURL(f.repo, sha[f.repo], f.upPath)
		body, err := download(url)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dataDir, f.name), body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.name, err)
		}
		rev.Sources[f.name] = url
	}
	return writeRevisions(dataDir, rev)
}

// headSHA returns the commit hash at the tip of the repository's main branch.
func headSHA(repo string) (string, error) {
	out, err := exec.Command("git", "ls-remote", repo, "refs/heads/main").Output()
	if err != nil {
		return "", fmt.Errorf("resolve %s main: %w", repo, err)
	}
	sha, _, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !ok || sha == "" {
		return "", fmt.Errorf("resolve %s main: unexpected ls-remote output %q", repo, out)
	}
	return sha, nil
}

func rawURL(repo, sha, path string) string {
	owner := "devcontainers/spec"
	if repo == "vscode" {
		owner = "microsoft/vscode"
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", owner, sha, path)
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: status %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if len(body) > 0 && body[len(body)-1] != '\n' {
		body = append(body, '\n')
	}
	return body, nil
}

// writeRevisions writes rev to REVISIONS.json under dir with two-space indentation and a trailing
// newline, matching the committed format.
func writeRevisions(dir string, rev revisions) error {
	b, err := json.Marshal(rev, jsontext.Multiline(true), jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("marshal revisions: %w", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(filepath.Join(dir, "REVISIONS.json"), b, 0o644); err != nil {
		return fmt.Errorf("write REVISIONS.json: %w", err)
	}
	return nil
}
