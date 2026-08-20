// Package ocitest provides test helpers for publishing Dev Container Features to an ephemeral,
// in-memory OCI registry, so tests across packages can exercise the Feature fetch and merge paths
// against real registry round-trips without a network dependency.
package ocitest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
)

// The media types the Features distribution specification mandates for a Feature artifact's config
// blob and its single tar layer.
const (
	featureConfigMediaType = "application/vnd.devcontainers"
	featureLayerMediaType  = "application/vnd.devcontainers.layer.v1+tar"
)

// metadataFileName is the name of the metadata file a Feature archive carries.
const metadataFileName = "devcontainer-feature.json"

// Registry starts an ephemeral, in-memory OCI registry on a loopback address and returns its host
// (host:port). Loopback hosts are reached over plain HTTP, which the Feature fetcher permits. The
// registry is shut down when the test ends.
func Registry(t *testing.T) string {
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

// FeatureArchive builds an in-memory tar archive containing a devcontainer-feature.json with the
// given metadata, the layer payload of a published Feature. When compress is true the archive is
// gzip-compressed, exercising the fetcher's decompression path.
func FeatureArchive(t *testing.T, metadata string, compress bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	files := []struct{ name, content string }{
		{"install.sh", "#!/bin/sh\n"},
		{metadataFileName, metadata},
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

// PushFeature publishes archive as a Feature artifact at host/repo:tag. See [pushArtifact] for the
// meaning of asIndex.
func PushFeature(t *testing.T, host, repo, tag string, archive []byte, asIndex bool) {
	t.Helper()
	pushArtifact(t, host, repo, tag,
		featureConfigMediaType, []byte("{}"),
		featureLayerMediaType, archive, asIndex)
}

// PushImage publishes a minimal container image at host/repo:tag whose config carries the given
// labels, for exercising the base-image metadata path. See [pushArtifact] for the meaning of asIndex.
func PushImage(t *testing.T, host, repo, tag string, labels map[string]string, asIndex bool) {
	t.Helper()
	pushArtifact(t, host, repo, tag,
		ocispec.MediaTypeImageConfig, marshal(t, ocispec.Image{Config: ocispec.ImageConfig{Labels: labels}}),
		ocispec.MediaTypeImageLayer, []byte("layer"), asIndex)
}

// pushArtifact publishes a single-layer manifest built from the given config and layer blobs at
// host/repo:tag. When asIndex is true the tag resolves to an image index that points at the
// manifest, exercising the index-following path.
func pushArtifact(t *testing.T, host, repo, tag string,
	configMediaType string, config []byte,
	layerMediaType string, layer []byte, asIndex bool) {
	t.Helper()
	ctx := t.Context()
	repository, err := remote.NewRepository(host + "/" + repo)
	if err != nil {
		t.Fatal(err)
	}
	repository.PlainHTTP = true

	push := func(mediaType string, data []byte) ocispec.Descriptor {
		desc := ocispec.Descriptor{
			MediaType: mediaType,
			Digest:    digest.FromBytes(data),
			Size:      int64(len(data)),
		}
		if err := repository.Push(ctx, desc, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		return desc
	}

	configDesc := push(configMediaType, config)
	layerDesc := push(layerMediaType, layer)

	manBytes := marshal(t, ocispec.Manifest{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageManifest,
		Config:        configDesc,
		Layers:        []ocispec.Descriptor{layerDesc},
	})
	manDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manBytes),
		Size:      int64(len(manBytes)),
	}

	if !asIndex {
		if err := repository.PushReference(ctx, manDesc, bytes.NewReader(manBytes), tag); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Store the manifest by digest and publish an index pointing at it under the tag.
	if err := repository.Push(ctx, manDesc, bytes.NewReader(manBytes)); err != nil {
		t.Fatal(err)
	}
	indexBytes := marshal(t, ocispec.Index{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageIndex,
		Manifests:     []ocispec.Descriptor{manDesc},
	})
	indexDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromBytes(indexBytes),
		Size:      int64(len(indexBytes)),
	}
	if err := repository.PushReference(ctx, indexDesc, bytes.NewReader(indexBytes), tag); err != nil {
		t.Fatal(err)
	}
}

// marshal JSON-encodes v, failing the test on error.
func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
