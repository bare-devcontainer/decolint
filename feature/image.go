package feature

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/tailscale/hujson"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
)

// imageMetadataLabel is the OCI image label carrying Dev Container metadata entries, per the Dev
// Container specification's image metadata section.
const imageMetadataLabel = "devcontainer.metadata"

// parseImageRef parses a container image reference as written in the "image" property, accepting
// the shorthand forms the Docker engine does: a bare name resolves to the "library" namespace on
// Docker Hub, and a missing tag defaults to "latest".
func parseImageRef(raw string) (registry.Reference, error) {
	domain, remainder, ok := strings.Cut(raw, "/")
	// Docker treats the first path component as a registry host only when it can be one (it
	// contains a dot or a port, or is "localhost"); anything else resolves against Docker Hub.
	if !ok || (!strings.ContainsAny(domain, ".:") && domain != "localhost") {
		domain, remainder = "docker.io", raw
	}
	if domain == "index.docker.io" {
		domain = "docker.io"
	}
	if domain == "docker.io" && !strings.Contains(remainder, "/") {
		remainder = "library/" + remainder
	}
	parsed, err := registry.ParseReference(domain + "/" + remainder)
	if err != nil {
		return registry.Reference{}, fmt.Errorf("invalid image reference %q: %w", raw, err)
	}
	// Resolve an untagged reference to "latest", as the container runtime would.
	parsed.Reference = parsed.ReferenceOrDefault()
	return parsed, nil
}

// FetchImageMetadata retrieves the Dev Container metadata entries carried by the
// "devcontainer.metadata" label of the container image referenced by raw, in label order. It
// returns no entries when the image carries no such label; a label that is not valid JSON is
// ignored, matching the reference implementation. Every result, including a failure, is cached
// for the lifetime of the Fetcher.
func (f *Fetcher) FetchImageMetadata(ctx context.Context, raw string) ([]*Metadata, error) {
	f.mu.Lock()
	res, ok := f.imageCache[raw]
	f.mu.Unlock()
	if !ok {
		_, _ = fmt.Fprintf(f.log, "Downloading image metadata(%s)\n", raw)
		res.entries, res.err = f.fetchImageMetadata(ctx, raw)
		if res.err != nil {
			res.err = fmt.Errorf("fetch image %q: %w", raw, res.err)
		}
		f.mu.Lock()
		f.imageCache[raw] = res
		f.mu.Unlock()
	}
	return res.entries, res.err
}

func (f *Fetcher) fetchImageMetadata(ctx context.Context, raw string) ([]*Metadata, error) {
	ref, err := parseImageRef(raw)
	if err != nil {
		return nil, err
	}
	repo, err := f.repository(ref.Registry, ref.Repository)
	if err != nil {
		return nil, err
	}
	desc, err := repo.Resolve(ctx, ref.Reference)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", ref.Reference, err)
	}
	man, _, err := imageManifest(ctx, repo, desc)
	if err != nil {
		return nil, err
	}
	// Reject an oversized config before reading it into memory, for the same reason the Feature
	// layer fetch bounds its blob.
	if man.Config.Size > maxImageConfigBytes {
		return nil, fmt.Errorf("config %s size %d exceeds %d bytes", man.Config.Digest, man.Config.Size, maxImageConfigBytes)
	}
	// content.FetchAll verifies the fetched bytes against the config descriptor's size and digest;
	// repo.Fetch alone does not hash the body.
	blob, err := content.FetchAll(ctx, repo, man.Config)
	if err != nil {
		return nil, fmt.Errorf("fetch config %s: %w", man.Config.Digest, err)
	}
	// Both OCI and Docker image configs carry labels at the same path, so the config is decoded
	// without a media-type check; a non-image config simply has no labels and degrades to a no-op.
	var img ocispec.Image
	if err := json.Unmarshal(blob, &img); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", man.Config.Digest, err)
	}
	label, ok := img.Config.Labels[imageMetadataLabel]
	if !ok {
		return nil, nil
	}
	return imageMetadataEntries([]byte(label)), nil
}

// imageMetadataEntries parses the value of a "devcontainer.metadata" image label: an array of
// metadata entries, or a single entry object, which is wrapped in a one-entry result. Anything
// else, including malformed JSON, yields no entries, as the reference implementation ignores a
// broken label rather than failing. Only the ID and Root of each returned Metadata are populated;
// that is all the merge consumes for an image contributor.
func imageMetadataEntries(label []byte) []*Metadata {
	root, err := hujson.Parse(label)
	if err != nil {
		return nil
	}
	var elems []hujson.Value
	switch t := root.Value.(type) {
	case *hujson.Array:
		elems = t.Elements
	case *hujson.Object:
		elems = []hujson.Value{root}
	default:
		return nil
	}
	var entries []*Metadata
	for _, e := range elems {
		if _, ok := e.Value.(*hujson.Object); !ok {
			continue
		}
		md := &Metadata{Root: e}
		if v := e.Find("/id"); v != nil {
			if lit, ok := v.Value.(hujson.Literal); ok && lit.Kind() == '"' {
				md.ID = lit.String()
			}
		}
		entries = append(entries, md)
	}
	return entries
}
