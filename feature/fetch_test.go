package feature

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
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

// startOCIRegistry starts an ephemeral, in-memory OCI registry on a loopback address and returns
// its host (host:port).
func startOCIRegistry(t *testing.T) string {
	t.Helper()
	handler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{StoreType: oConfig.StoreMem},
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(func() {
		srv.Close()
		_ = handler.Close()
	})
	return strings.TrimPrefix(srv.URL, "http://")
}

// pushOCIFeature publishes archive as a Feature artifact at host/repo:tag. When asIndex is true the
// tag resolves to an image index that points at the manifest, exercising the index-following path.
func pushOCIFeature(t *testing.T, host, repoName, tag string, archive []byte, asIndex bool) {
	t.Helper()
	ctx := t.Context()
	repo, err := remote.NewRepository(host + "/" + repoName)
	if err != nil {
		t.Fatal(err)
	}
	repo.PlainHTTP = true

	push := func(mediaType string, data []byte) ocispec.Descriptor {
		desc := ocispec.Descriptor{
			MediaType: mediaType,
			Digest:    digest.FromBytes(data),
			Size:      int64(len(data)),
		}
		if err := repo.Push(ctx, desc, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		return desc
	}

	configDesc := push("application/vnd.devcontainers", []byte("{}"))
	layerDesc := push(featureLayerMediaType, archive)

	manBytes := mustMarshal(t, ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
	})
	manDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manBytes),
		Size:      int64(len(manBytes)),
	}

	if !asIndex {
		if err := repo.PushReference(ctx, manDesc, bytes.NewReader(manBytes), tag); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Store the manifest by digest and publish an index pointing at it under the tag.
	if err := repo.Push(ctx, manDesc, bytes.NewReader(manBytes)); err != nil {
		t.Fatal(err)
	}
	indexBytes := mustMarshal(t, ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{manDesc},
	})
	indexDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromBytes(indexBytes),
		Size:      int64(len(indexBytes)),
	}
	if err := repo.PushReference(ctx, indexDesc, bytes.NewReader(indexBytes), tag); err != nil {
		t.Fatal(err)
	}
}

// mustMarshal JSON-encodes v, failing the test on error.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFetchOCI(t *testing.T) {
	t.Parallel()

	host := startOCIRegistry(t)
	pushOCIFeature(t, host, "devcontainers/features/node", "1",
		archiveWithMetadata(t, `{"id": "node", "version": "1.2.3"}`, false), false)

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

	host := startOCIRegistry(t)
	pushOCIFeature(t, host, "devcontainers/features/go", "1",
		archiveWithMetadata(t, `{"id": "go"}`, false), true)

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

	host := startOCIRegistry(t)
	pushOCIFeature(t, host, "devcontainers/features/node", "1",
		archiveWithMetadata(t, `{"id": "node"}`, false), false)

	if _, err := NewFetcher().Fetch(t.Context(), host+"/devcontainers/features/nope:1", ""); err == nil {
		t.Error("Fetch of an unknown repository: got nil error")
	}
}
