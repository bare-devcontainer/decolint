package feature

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// fetchOCI retrieves a Feature distributed as an OCI artifact, using anonymous pull access. The
// registry protocol (manifest resolution, the token handshake, blob download, and digest
// verification) is handled by oras-go.
func (f *Fetcher) fetchOCI(ctx context.Context, feat Ref) (*Metadata, error) {
	repo, err := remote.NewRepository(feat.Registry + "/" + feat.Repository)
	if err != nil {
		return nil, err
	}
	// Loopback registries (local test registries) are reached over plain HTTP; all others use HTTPS.
	repo.PlainHTTP = isLoopback(feat.Registry)
	repo.Client = &auth.Client{
		Client: f.client,
		Cache:  auth.NewCache(),
		Header: http.Header{"User-Agent": []string{"decolint"}},
	}

	target := feat.Tag
	if feat.Digest != "" {
		target = feat.Digest
	}
	desc, err := repo.Resolve(ctx, target)
	if err != nil {
		return nil, err
	}

	layer, err := featureLayer(ctx, repo, desc)
	if err != nil {
		return nil, err
	}

	blob, err := repo.Fetch(ctx, layer)
	if err != nil {
		return nil, err
	}
	defer func() { _ = blob.Close() }()
	src, err := metadataFromArchive(io.LimitReader(blob, maxArchiveBytes))
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
			return ocispec.Manifest{}, err
		}
		var index ocispec.Index
		if err := json.Unmarshal(raw, &index); err != nil {
			return ocispec.Manifest{}, err
		}
		if len(index.Manifests) == 0 {
			return ocispec.Manifest{}, fmt.Errorf("index %s has no manifests", desc.Digest)
		}
		desc = index.Manifests[0]
	}

	raw, err := content.FetchAll(ctx, repo, desc)
	if err != nil {
		return ocispec.Manifest{}, err
	}
	var man ocispec.Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return ocispec.Manifest{}, err
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
