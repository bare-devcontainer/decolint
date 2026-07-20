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

// featureConfigMediaType is the media type the Features distribution specification requires on a
// Feature manifest's config descriptor. The reference implementation treats a manifest whose config
// media type differs as not being a Feature; matching that keeps decolint from linting artifacts the
// toolchain would reject.
const featureConfigMediaType = "application/vnd.devcontainers"

// dockerManifestListMediaType is the Docker (schema 2) equivalent of an OCI image index; ghcr.io
// serves Features behind either media type.
const dockerManifestListMediaType = "application/vnd.docker.distribution.manifest.list.v2+json"

// repository returns a client for the repository named repoName on the registry reg, using
// anonymous pull access. A loopback registry (a local test registry) is reached over plain HTTP;
// all others use HTTPS.
func (f *Fetcher) repository(reg, repoName string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(reg + "/" + repoName)
	if err != nil {
		return nil, fmt.Errorf("new repository %s/%s: %w", reg, repoName, err)
	}
	repo.PlainHTTP = isLoopback(reg)
	repo.Client = &auth.Client{
		Client: f.client,
		Cache:  auth.NewCache(),
		Header: http.Header{"User-Agent": []string{"decolint"}},
	}
	return repo, nil
}

// fetchOCI retrieves a Feature distributed as an OCI artifact, using anonymous pull access. oras-go
// handles the registry protocol (manifest resolution and the token handshake). Every manifest and
// the layer blob are read through content.FetchAll, which verifies the bytes against the digest in
// their descriptor; repo.Fetch on its own does not.
func (f *Fetcher) fetchOCI(ctx context.Context, feat Ref) (*Metadata, error) {
	repo, err := f.repository(feat.OCI.Registry, feat.OCI.Repository)
	if err != nil {
		return nil, err
	}

	desc, err := repo.Resolve(ctx, feat.OCI.Reference)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", feat.OCI.Reference, err)
	}

	man, manifestDigest, err := imageManifest(ctx, repo, desc)
	if err != nil {
		return nil, err
	}
	if man.Config.MediaType != featureConfigMediaType {
		return nil, fmt.Errorf("manifest %s config media type %q is not %s", manifestDigest, man.Config.MediaType, featureConfigMediaType)
	}
	layer, err := featureLayer(man, manifestDigest)
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
	md, err := parseMetadata(src)
	if err != nil {
		return nil, err
	}
	md.Digest = manifestDigest
	return md, nil
}

// featureLayer returns the descriptor of the layer carrying the Feature archive: the layer with the
// Features distribution media type, or the sole layer if none declares it. manifestDigest names the
// manifest for diagnostics.
func featureLayer(man ocispec.Manifest, manifestDigest string) (ocispec.Descriptor, error) {
	for _, layer := range man.Layers {
		if layer.MediaType == featureLayerMediaType {
			return layer, nil
		}
	}
	if len(man.Layers) == 1 {
		return man.Layers[0], nil
	}
	return ocispec.Descriptor{}, fmt.Errorf("manifest %s has no %s layer", manifestDigest, featureLayerMediaType)
}

// imageManifest fetches and parses the image manifest for desc and returns it with its digest. When
// desc is an image index, it follows the first entry, and the returned digest is that of the
// followed manifest. Features are single-platform, and for a multi-platform container image any
// entry serves: the "devcontainer.metadata" label is identical across platforms, and buildx places
// attestation entries after the real platform manifests.
func imageManifest(ctx context.Context, repo *remote.Repository, desc ocispec.Descriptor) (ocispec.Manifest, string, error) {
	if desc.MediaType == ocispec.MediaTypeImageIndex || desc.MediaType == dockerManifestListMediaType {
		raw, err := content.FetchAll(ctx, repo, desc)
		if err != nil {
			return ocispec.Manifest{}, "", fmt.Errorf("fetch index %s: %w", desc.Digest, err)
		}
		var index ocispec.Index
		if err := json.Unmarshal(raw, &index); err != nil {
			return ocispec.Manifest{}, "", fmt.Errorf("decode index %s: %w", desc.Digest, err)
		}
		if len(index.Manifests) == 0 {
			return ocispec.Manifest{}, "", fmt.Errorf("index %s has no manifests", desc.Digest)
		}
		desc = index.Manifests[0]
	}

	raw, err := content.FetchAll(ctx, repo, desc)
	if err != nil {
		return ocispec.Manifest{}, "", fmt.Errorf("fetch manifest %s: %w", desc.Digest, err)
	}
	var man ocispec.Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return ocispec.Manifest{}, "", fmt.Errorf("decode manifest %s: %w", desc.Digest, err)
	}
	return man, desc.Digest.String(), nil
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
