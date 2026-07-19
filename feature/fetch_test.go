package feature

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
)

// writeLocalFeature creates dir/<name>/devcontainer-feature.json with the given content.
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

// openRoot opens dir as an os.Root, closed when the test ends, to fetch local Features through.
func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestFetchLocal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLocalFeature(t, dir, "my-feature", `{"id": "my-feature", "version": "1.0.0"}`)

	f := NewFetcher()
	md, err := f.Fetch(t.Context(), "./my-feature", openRoot(t, dir), ".")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "my-feature" || md.Version != "1.0.0" {
		t.Errorf("got ID %q version %q, want my-feature 1.0.0", md.ID, md.Version)
	}
}

func TestFetchLocalEscapingRootIsRejected(t *testing.T) {
	t.Parallel()

	// A Feature outside the confining root (here, dir's parent) must not be reachable via "..",
	// even though it exists on disk.
	dir := t.TempDir()
	writeLocalFeature(t, filepath.Dir(dir), "escaped-feature", `{"id": "escaped-feature"}`)

	f := NewFetcher()
	if _, err := f.Fetch(t.Context(), "../escaped-feature", openRoot(t, dir), "."); err == nil {
		t.Error("Fetch of a feature escaping the root: got nil error")
	}
}

func TestFetchLocalMissing(t *testing.T) {
	t.Parallel()

	f := NewFetcher()
	if _, err := f.Fetch(t.Context(), "./nope", openRoot(t, t.TempDir()), "."); err == nil {
		t.Error("Fetch of a missing local feature: got nil error")
	}
}

func TestFetchLocalTooLarge(t *testing.T) {
	t.Parallel()

	// A metadata file just over the size cap must be rejected on its declared size, before it is
	// read into memory or parsed.
	dir := t.TempDir()
	writeLocalFeature(t, dir, "big", "{"+strings.Repeat(" ", maxMetadataBytes)+"}")

	_, err := NewFetcher().Fetch(t.Context(), "./big", openRoot(t, dir), ".")
	if err == nil {
		t.Fatal("Fetch of an oversized local feature: got nil error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want it to mention exceeding the size limit", err)
	}
}

func TestFetchCachesResults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLocalFeature(t, dir, "f", `{"id": "before"}`)
	root := openRoot(t, dir)

	f := NewFetcher()
	md, err := f.Fetch(t.Context(), "./f", root, ".")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "before" {
		t.Fatalf("ID = %q, want before", md.ID)
	}

	// A second fetch must hit the cache and not observe the changed file.
	writeLocalFeature(t, dir, "f", `{"id": "after"}`)
	md, err = f.Fetch(t.Context(), "./f", root, ".")
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

	md, err := NewFetcher().Fetch(t.Context(), "./f", openRoot(t, dir), ".")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var deps []string
	for _, d := range md.DependsOn {
		deps = append(deps, d.Ref)
	}
	if got, want := strings.Join(deps, ","), "./dep1,./dep2"; got != want {
		t.Errorf("DependsOn = %q, want %q", got, want)
	}
	// The options each dependency is requested with are captured, distinguishing otherwise identical
	// dependencies for install ordering.
	if got, want := md.DependsOn[1].Options, (optionValue{kind: 'o', obj: map[string]optScalar{"opt": {kind: 'b', b: true}}}); !reflect.DeepEqual(got, want) {
		t.Errorf("DependsOn[1].Options = %+v, want %+v", got, want)
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feature.tgz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	f := NewFetcher()
	f.client = srv.Client()
	md, err := f.Fetch(t.Context(), srv.URL+"/feature.tgz", nil, "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "tarred" {
		t.Errorf("ID = %q, want tarred", md.ID)
	}
}

func TestFetchTarballNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()

	f := NewFetcher()
	f.client = srv.Client()
	if _, err := f.Fetch(t.Context(), srv.URL+"/feature.tgz", nil, ""); err == nil {
		t.Error("Fetch of a missing tarball: got nil error")
	}
}

func TestFetchInvalidReference(t *testing.T) {
	t.Parallel()

	// A reference that is neither a local path nor an HTTPS URI is parsed as an OCI reference; a
	// malformed one is rejected before any fetch is attempted.
	if _, err := NewFetcher().Fetch(t.Context(), "not a valid reference", nil, ""); err == nil {
		t.Error("Fetch of an invalid feature reference: got nil error")
	}
}

func TestFetchTarballFollowsSecureRedirect(t *testing.T) {
	t.Parallel()

	// A redirect that keeps the request on HTTPS preserves the transport guarantee, so it is
	// followed (unlike the downgrade rejected by TestFetchTarballRefusesInsecureRedirect).
	archive := archiveWithMetadata(t, `{"id": "redirected"}`, true)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/real.tgz" {
			_, _ = w.Write(archive)
			return
		}
		http.Redirect(w, r, "/real.tgz", http.StatusFound)
	}))
	defer srv.Close()

	f := NewFetcher()
	client := srv.Client()
	client.CheckRedirect = refuseInsecureRedirect
	f.client = client

	md, err := f.Fetch(t.Context(), srv.URL+"/feature.tgz", nil, "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "redirected" {
		t.Errorf("ID = %q, want redirected", md.ID)
	}
}

func TestFetchTarballRefusesInsecureRedirect(t *testing.T) {
	t.Parallel()

	// A plain-HTTP endpoint that would serve a valid archive if the redirect were followed.
	archive := archiveWithMetadata(t, `{"id": "downgraded"}`, true)
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer plain.Close()

	// An HTTPS reference that redirects to the plain-HTTP endpoint, downgrading the transport.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/feature.tgz", http.StatusFound)
	}))
	defer srv.Close()

	f := NewFetcher()
	client := srv.Client()
	client.CheckRedirect = refuseInsecureRedirect
	f.client = client

	_, err := f.Fetch(t.Context(), srv.URL+"/feature.tgz", nil, "")
	if err == nil {
		t.Fatal("Fetch following an HTTPS to HTTP redirect: got nil error")
	}
	if !strings.Contains(err.Error(), "insecure redirect") {
		t.Errorf("error = %v, want it to mention an insecure redirect", err)
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

	md, err := NewFetcher().Fetch(t.Context(), host+"/devcontainers/features/node:1", nil, "")
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

	md, err := NewFetcher().Fetch(t.Context(), host+"/devcontainers/features/go:1", nil, "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if md.ID != "go" {
		t.Errorf("ID = %q, want go", md.ID)
	}
}

// tamperingProxy fronts a registry, forwarding every request but replacing each layer blob it serves
// with replacement. The blob no longer matches the digest declared in the (untampered) manifest,
// while still being a well-formed archive: a caller that skips digest verification would parse it as
// authentic. Config blobs and manifests are JSON and pass through unchanged.
func tamperingProxy(t *testing.T, host string, replacement []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://"+host+r.URL.RequestURI(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Forward request headers, notably Accept, which the registry uses for manifest content
		// negotiation; without it manifest resolution fails before the layer is ever fetched.
		maps.Copy(req.Header, r.Header)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		tampered := strings.Contains(r.URL.Path, "/blobs/") && len(body) > 0 && !json.Valid(body)
		maps.Copy(w.Header(), resp.Header)
		if tampered {
			// The replacement length differs from the manifest's declared size; drop the upstream
			// Content-Length so the response frames the substituted body correctly.
			w.Header().Del("Content-Length")
			body = replacement
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestFetchOCIRejectsTamperedLayer(t *testing.T) {
	t.Parallel()

	host := startOCIRegistry(t)
	pushOCIFeature(t, host, "devcontainers/features/node", "1",
		archiveWithMetadata(t, `{"id": "node"}`, false), false)

	// A valid archive with different metadata: parseable, but not the layer the manifest commits to.
	forged := archiveWithMetadata(t, `{"id": "evil"}`, false)
	proxy := tamperingProxy(t, host, forged)
	if _, err := NewFetcher().Fetch(t.Context(), proxy+"/devcontainers/features/node:1", nil, ""); err == nil {
		t.Error("Fetch of a Feature whose layer bytes do not match the manifest digest: got nil error")
	}
}

func TestFetchOCIUnknownRepository(t *testing.T) {
	t.Parallel()

	host := startOCIRegistry(t)
	pushOCIFeature(t, host, "devcontainers/features/node", "1",
		archiveWithMetadata(t, `{"id": "node"}`, false), false)

	if _, err := NewFetcher().Fetch(t.Context(), host+"/devcontainers/features/nope:1", nil, ""); err == nil {
		t.Error("Fetch of an unknown repository: got nil error")
	}
}
