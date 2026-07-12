package feature

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Media types of the OCI artifacts a published Feature consists of. Docker registries may rewrite
// the manifest media types, so both the OCI and Docker variants are accepted.
const (
	ociManifestMediaType    = "application/vnd.oci.image.manifest.v1+json"
	ociIndexMediaType       = "application/vnd.oci.image.index.v1+json"
	dockerManifestMediaType = "application/vnd.docker.distribution.manifest.v2+json"
	dockerListMediaType     = "application/vnd.docker.distribution.manifest.list.v2+json"
	// featureLayerMediaType is the media type of the single tar layer a Feature is packaged as, per
	// the Features distribution specification.
	featureLayerMediaType = "application/vnd.devcontainers.layer.v1+tar"
)

// manifestAccept is the Accept header for manifest requests.
var manifestAccept = strings.Join([]string{
	ociManifestMediaType,
	ociIndexMediaType,
	dockerManifestMediaType,
	dockerListMediaType,
}, ", ")

// ociDescriptor is a content descriptor within a manifest or index.
type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
}

// ociManifest is the subset of an OCI image manifest or index needed to locate the Feature layer.
// A manifest carries Layers; an index carries Manifests.
type ociManifest struct {
	Layers    []ociDescriptor `json:"layers"`
	Manifests []ociDescriptor `json:"manifests"`
}

// fetchOCI retrieves a Feature distributed as an OCI artifact, using anonymous pull access.
func (f *Fetcher) fetchOCI(ctx context.Context, ref Ref) (*Metadata, error) {
	reference := ref.Tag
	if ref.Digest != "" {
		reference = ref.Digest
	}

	// The token, obtained lazily on the first 401 challenge, is shared by the subsequent requests
	// against the same repository.
	var token string
	manifest, err := f.fetchManifest(ctx, ref, reference, &token)
	if err != nil {
		return nil, err
	}
	if len(manifest.Manifests) > 0 {
		// The reference resolved to an index; Features are single-platform, so follow its first
		// entry.
		manifest, err = f.fetchManifest(ctx, ref, manifest.Manifests[0].Digest, &token)
		if err != nil {
			return nil, err
		}
	}

	layer, err := featureLayer(manifest)
	if err != nil {
		return nil, err
	}
	blobURL := fmt.Sprintf("%s://%s/v2/%s/blobs/%s", registryScheme(ref.Registry), ref.Registry, ref.Repository, layer.Digest)
	resp, err := f.registryGet(ctx, ref, blobURL, "", &token)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	src, err := metadataFromArchive(io.LimitReader(resp.Body, maxArchiveBytes))
	if err != nil {
		return nil, fmt.Errorf("read layer %s: %w", layer.Digest, err)
	}
	return parseMetadata(src)
}

// featureLayer picks the layer carrying the Feature archive out of manifest: the layer with the
// Features distribution media type, or the sole layer if none declares it.
func featureLayer(manifest *ociManifest) (ociDescriptor, error) {
	for _, layer := range manifest.Layers {
		if layer.MediaType == featureLayerMediaType {
			return layer, nil
		}
	}
	if len(manifest.Layers) == 1 {
		return manifest.Layers[0], nil
	}
	return ociDescriptor{}, fmt.Errorf("manifest has no %s layer", featureLayerMediaType)
}

// fetchManifest retrieves and decodes the manifest (or index) at reference, a tag or digest.
func (f *Fetcher) fetchManifest(ctx context.Context, ref Ref, reference string, token *string) (*ociManifest, error) {
	manifestURL := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", registryScheme(ref.Registry), ref.Registry, ref.Repository, reference)
	resp, err := f.registryGet(ctx, ref, manifestURL, manifestAccept, token)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return nil, err
	}
	var manifest ociManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode manifest %s: %w", reference, err)
	}
	return &manifest, nil
}

// registryGet performs an authenticated GET against a registry endpoint. On a 401 challenge it
// obtains an anonymous bearer token per the OCI distribution auth flow, stores it in *token for
// reuse, and retries once.
func (f *Fetcher) registryGet(ctx context.Context, ref Ref, url, accept string, token *string) (*http.Response, error) {
	do := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if *token != "" {
			req.Header.Set("Authorization", "Bearer "+*token)
		}
		return f.client.Do(req)
	}

	resp, err := do()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && *token == "" {
		challenge := resp.Header.Get("Www-Authenticate")
		_ = resp.Body.Close()
		*token, err = f.fetchToken(ctx, ref, challenge)
		if err != nil {
			return nil, err
		}
		resp, err = do()
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode != http.StatusOK {
		status := resp.Status
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, status)
	}
	return resp, nil
}

// fetchToken obtains an anonymous pull token from the token endpoint named by a Bearer challenge
// (RFC 6750): `Bearer realm="...",service="...",scope="..."`.
func (f *Fetcher) fetchToken(ctx context.Context, ref Ref, challenge string) (string, error) {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(challenge), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("registry %s requires unsupported authentication %q", ref.Registry, challenge)
	}
	params := parseChallengeParams(rest)
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("registry %s sent a Bearer challenge without a realm", ref.Registry)
	}

	query := url.Values{}
	if s := params["service"]; s != "" {
		query.Set("service", s)
	}
	scope := params["scope"]
	if scope == "" {
		scope = fmt.Sprintf("repository:%s:pull", ref.Repository)
	}
	query.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", realm, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return "", err
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", fmt.Errorf("decode token response from %s: %w", realm, err)
	}
	if body.Token != "" {
		return body.Token, nil
	}
	if body.AccessToken != "" {
		return body.AccessToken, nil
	}
	return "", fmt.Errorf("token response from %s contains no token", realm)
}

// parseChallengeParams parses the comma-separated key="value" parameters of an auth challenge.
func parseChallengeParams(s string) map[string]string {
	params := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		params[strings.ToLower(key)] = strings.Trim(value, `"`)
	}
	return params
}

// registryScheme returns the URL scheme for reaching a registry host: plain HTTP for loopback
// hosts (local test registries), HTTPS otherwise.
func registryScheme(host string) string {
	h := host
	if colon := strings.LastIndex(h, ":"); colon >= 0 {
		h = h[:colon]
	}
	if h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "[::1]" {
		return "http"
	}
	return "https"
}
