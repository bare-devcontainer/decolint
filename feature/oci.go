package feature

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// featureLayerMediaType is the media type of the single tar layer a Feature is packaged as, per the
// Features distribution specification.
const featureLayerMediaType = "application/vnd.devcontainers.layer.v1+tar"

// dockerManifestListMediaType is the Docker (schema 2) equivalent of an OCI image index; ghcr.io
// serves Features behind either media type.
const dockerManifestListMediaType = "application/vnd.docker.distribution.manifest.list.v2+json"

// fetchOCI retrieves a Feature distributed as an OCI artifact, using anonymous pull access. oras-go
// handles the registry protocol (manifest resolution and the token handshake). Every manifest and
// the layer blob are read through content.FetchAll, which verifies the bytes against the digest in
// their descriptor; repo.Fetch on its own does not.
func (f *Fetcher) fetchOCI(ctx context.Context, feat Ref) (*Metadata, error) {
	repo, err := remote.NewRepository(feat.OCI.Registry + "/" + feat.OCI.Repository)
	if err != nil {
		return nil, fmt.Errorf("new repository %s/%s: %w", feat.OCI.Registry, feat.OCI.Repository, err)
	}
	// Loopback registries (local test registries) are reached over plain HTTP; all others use HTTPS.
	repo.PlainHTTP = isLoopback(feat.OCI.Registry)
	repo.Client = &auth.Client{
		Client: f.client,
		Cache:  auth.NewCache(),
		Header: http.Header{"User-Agent": []string{"decolint"}},
	}

	// ReferenceOrDefault resolves an unversioned reference to the "latest" tag.
	target := feat.OCI.ReferenceOrDefault()
	desc, err := repo.Resolve(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", target, err)
	}

	layer, err := featureLayer(ctx, repo, desc)
	if err != nil {
		return nil, err
	}

	// Reject an oversized layer before reading it into memory. Digest verification requires hashing
	// the whole blob, so this path cannot stream and stop early the way the tarball path does; the
	// declared size is the only bound available up front.
	if layer.Size > maxArchiveBytes {
		return nil, fmt.Errorf("layer %s size %d exceeds %d bytes", layer.Digest, layer.Size, maxArchiveBytes)
	}
	// content.FetchAll verifies the fetched bytes against the layer descriptor's size and digest.
	// repo.Fetch alone does not hash the body (it only checks the optional Docker-Content-Digest
	// header), so a tampered layer from a malicious or man-in-the-middle registry would otherwise be
	// linted as if authentic, defeating digest-pinned references.
	blob, err := content.FetchAll(ctx, repo, layer)
	if err != nil {
		return nil, fmt.Errorf("fetch layer %s: %w", layer.Digest, err)
	}
	src, err := metadataFromArchive(bytes.NewReader(blob))
	if err != nil {
		return nil, fmt.Errorf("read layer %s: %w", layer.Digest, err)
	}
	return parseMetadata(src)
}

// featureLayer resolves desc to an image manifest and returns the descriptor of the layer carrying
// the Feature archive: the layer with the Features distribution media type, or the sole layer if
// none declares it.
func featureLayer(ctx context.Context, repo *remote.Repository, desc ocispec.Descriptor) (ocispec.Descriptor, error) {
	man, err := imageManifest(ctx, repo, desc)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	for _, layer := range man.Layers {
		if layer.MediaType == featureLayerMediaType {
			return layer, nil
		}
	}
	if len(man.Layers) == 1 {
		return man.Layers[0], nil
	}
	return ocispec.Descriptor{}, fmt.Errorf("manifest %s has no %s layer", desc.Digest, featureLayerMediaType)
}

// imageManifest fetches and parses the image manifest for desc. When desc is an image index (Features
// are single-platform), it follows the first entry.
func imageManifest(ctx context.Context, repo *remote.Repository, desc ocispec.Descriptor) (ocispec.Manifest, error) {
	if desc.MediaType == ocispec.MediaTypeImageIndex || desc.MediaType == dockerManifestListMediaType {
		raw, err := content.FetchAll(ctx, repo, desc)
		if err != nil {
			return ocispec.Manifest{}, fmt.Errorf("fetch index %s: %w", desc.Digest, err)
		}
		var index ocispec.Index
		if err := json.Unmarshal(raw, &index); err != nil {
			return ocispec.Manifest{}, fmt.Errorf("decode index %s: %w", desc.Digest, err)
		}
		if len(index.Manifests) == 0 {
			return ocispec.Manifest{}, fmt.Errorf("index %s has no manifests", desc.Digest)
		}
		desc = index.Manifests[0]
	}

	raw, err := content.FetchAll(ctx, repo, desc)
	if err != nil {
		return ocispec.Manifest{}, fmt.Errorf("fetch manifest %s: %w", desc.Digest, err)
	}
	var man ocispec.Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("decode manifest %s: %w", desc.Digest, err)
	}
	return man, nil
}

// isLoopback reports whether host (optionally with a port) names the local machine, in which case
// the registry is reached over plain HTTP.
func isLoopback(host string) bool {
	if colon := strings.LastIndex(host, ":"); colon >= 0 {
		host = host[:colon]
	}
	switch host {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	default:
		return false
	}
}
