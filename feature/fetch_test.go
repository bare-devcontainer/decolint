package feature

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLocalFeature creates dir/<name>/devcontainer-feature.json with the given content and
// returns the base directory.
func writeLocalFeature(t *testing.T, dir, name, src string) {
	t.Helper()
	featureDir := filepath.Join(dir, name)
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, metadataFileName), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFetchLocal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLocalFeature(t, dir, "my-feature", `{"id": "my-feature", "version": "1.0.0"}`)

	f := NewFetcher()
	md, err := f.Fetch(t.Context(), "./my-feature", dir)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "my-feature" || md.Version != "1.0.0" {
		t.Errorf("got ID %q version %q, want my-feature 1.0.0", md.ID, md.Version)
	}
}

func TestFetchLocalMissing(t *testing.T) {
	t.Parallel()

	f := NewFetcher()
	if _, err := f.Fetch(t.Context(), "./nope", t.TempDir()); err == nil {
		t.Error("Fetch of a missing local feature: got nil error")
	}
}

func TestFetchCachesResults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLocalFeature(t, dir, "f", `{"id": "before"}`)

	f := NewFetcher()
	md, err := f.Fetch(t.Context(), "./f", dir)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "before" {
		t.Fatalf("ID = %q, want before", md.ID)
	}

	// A second fetch must hit the cache and not observe the changed file.
	writeLocalFeature(t, dir, "f", `{"id": "after"}`)
	md, err = f.Fetch(t.Context(), "./f", dir)
	if err != nil {
		t.Fatalf("Fetch (cached): %v", err)
	}
	if md.ID != "before" {
		t.Errorf("cached ID = %q, want before", md.ID)
	}
}

func TestFetchMetadataParse(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLocalFeature(t, dir, "f", `{
	  // JSONC comments are allowed in feature metadata.
	  "id": "f",
	  "dependsOn": {"./dep1": {}, "./dep2": {"opt": true}},
	  "installsAfter": ["ghcr.io/devcontainers/features/common-utils"],
	}`)

	md, err := NewFetcher().Fetch(t.Context(), "./f", dir)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := strings.Join(md.DependsOn, ","), "./dep1,./dep2"; got != want {
		t.Errorf("DependsOn = %q, want %q", got, want)
	}
	if got, want := strings.Join(md.InstallsAfter, ","), "ghcr.io/devcontainers/features/common-utils"; got != want {
		t.Errorf("InstallsAfter = %q, want %q", got, want)
	}
}

// archiveWithMetadata builds an in-memory tar archive containing a devcontainer-feature.json,
// optionally gzip-compressed.
func archiveWithMetadata(t *testing.T, src string, compress bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	files := []struct{ name, content string }{
		{"install.sh", "#!/bin/sh\n"},
		{metadataFileName, src},
	}
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "./" + f.name, Mode: 0o644, Size: int64(len(f.content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if !compress {
		return buf.Bytes()
	}
	var gzBuf bytes.Buffer
	gz := gzip.NewWriter(&gzBuf)
	if _, err := gz.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return gzBuf.Bytes()
}

func TestFetchTarball(t *testing.T) {
	t.Parallel()

	archive := archiveWithMetadata(t, `{"id": "tarred", "version": "2.0.0"}`, true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feature.tgz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	md, err := NewFetcher().Fetch(t.Context(), srv.URL+"/feature.tgz", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "tarred" {
		t.Errorf("ID = %q, want tarred", md.ID)
	}
}

func TestFetchTarballNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	if _, err := NewFetcher().Fetch(t.Context(), srv.URL+"/feature.tgz", ""); err == nil {
		t.Error("Fetch of a missing tarball: got nil error")
	}
}

// fakeRegistry serves a minimal OCI distribution API for a single feature artifact, requiring the
// anonymous bearer token flow.
type fakeRegistry struct {
	repository string
	blob       []byte
	blobDigest string
	// useIndex makes the tag reference resolve to an image index pointing at the manifest.
	useIndex bool
	// manifestDigest is the digest the index points at.
	manifestDigest string
	token          string
}

func (fr *fakeRegistry) handler(registryHost func() string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("scope") != "repository:"+fr.repository+":pull" {
			http.Error(w, "bad scope", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprintf(w, `{"token": %q}`, fr.token)
	})
	authorized := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") == "Bearer "+fr.token {
			return true
		}
		w.Header().Set("Www-Authenticate", fmt.Sprintf(
			`Bearer realm="http://%s/token",service="registry",scope="repository:%s:pull"`,
			registryHost(), fr.repository))
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	mux.HandleFunc("/v2/"+fr.repository+"/manifests/", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		reference := strings.TrimPrefix(r.URL.Path, "/v2/"+fr.repository+"/manifests/")
		if fr.useIndex && reference != fr.manifestDigest {
			w.Header().Set("Content-Type", ociIndexMediaType)
			_, _ = fmt.Fprintf(w, `{"mediaType": %q, "manifests": [{"mediaType": %q, "digest": %q}]}`,
				ociIndexMediaType, ociManifestMediaType, fr.manifestDigest)
			return
		}
		w.Header().Set("Content-Type", ociManifestMediaType)
		_, _ = fmt.Fprintf(w, `{"mediaType": %q, "layers": [{"mediaType": %q, "digest": %q}]}`,
			ociManifestMediaType, featureLayerMediaType, fr.blobDigest)
	})
	mux.HandleFunc("/v2/"+fr.repository+"/blobs/"+fr.blobDigest, func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		_, _ = w.Write(fr.blob)
	})
	return mux
}

// startRegistry serves fr on a loopback address and returns the registry host (host:port).
func startRegistry(t *testing.T, fr *fakeRegistry) string {
	t.Helper()
	var host string
	srv := httptest.NewServer(fr.handler(func() string { return host }))
	t.Cleanup(srv.Close)
	host = strings.TrimPrefix(srv.URL, "http://")
	return host
}

func TestFetchOCI(t *testing.T) {
	t.Parallel()

	fr := &fakeRegistry{
		repository: "devcontainers/features/node",
		blob:       archiveWithMetadata(t, `{"id": "node", "version": "1.2.3"}`, false),
		blobDigest: "sha256:feedface",
		token:      "anonymous-token",
	}
	host := startRegistry(t, fr)

	md, err := NewFetcher().Fetch(t.Context(), host+"/devcontainers/features/node:1", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "node" || md.Version != "1.2.3" {
		t.Errorf("got ID %q version %q, want node 1.2.3", md.ID, md.Version)
	}
}

func TestFetchOCIThroughIndex(t *testing.T) {
	t.Parallel()

	fr := &fakeRegistry{
		repository:     "devcontainers/features/go",
		blob:           archiveWithMetadata(t, `{"id": "go"}`, false),
		blobDigest:     "sha256:cafebabe",
		useIndex:       true,
		manifestDigest: "sha256:deadbeef",
		token:          "anonymous-token",
	}
	host := startRegistry(t, fr)

	md, err := NewFetcher().Fetch(t.Context(), host+"/devcontainers/features/go:1", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "go" {
		t.Errorf("ID = %q, want go", md.ID)
	}
}

func TestFetchOCIUnknownRepository(t *testing.T) {
	t.Parallel()

	fr := &fakeRegistry{
		repository: "devcontainers/features/node",
		blob:       archiveWithMetadata(t, `{"id": "node"}`, false),
		blobDigest: "sha256:feedface",
		token:      "anonymous-token",
	}
	host := startRegistry(t, fr)

	if _, err := NewFetcher().Fetch(t.Context(), host+"/devcontainers/features/nope:1", ""); err == nil {
		t.Error("Fetch of an unknown repository: got nil error")
	}
}
