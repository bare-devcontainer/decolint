package feature

import (
	"bytes"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
)

func TestFeatureLayer(t *testing.T) {
	t.Parallel()

	feat := ocispec.Descriptor{MediaType: featureLayerMediaType, Digest: "sha256:feature"}
	other := ocispec.Descriptor{MediaType: "application/vnd.oci.image.layer.v1.tar", Digest: "sha256:other"}

	tests := []struct {
		name    string
		layers  []ocispec.Descriptor
		wantDig string
		wantErr bool
	}{
		{"feature media type among others", []ocispec.Descriptor{other, feat}, "sha256:feature", false},
		// A single layer is taken as the Feature archive even without the distribution media type.
		{"sole layer without the media type", []ocispec.Descriptor{other}, "sha256:other", false},
		{"no layers", nil, "", true},
		{"multiple layers, none matching", []ocispec.Descriptor{other, other}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := featureLayer(ocispec.Manifest{Layers: tt.layers}, "sha256:manifest")
			if (err != nil) != tt.wantErr {
				t.Fatalf("featureLayer error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got.Digest.String() != tt.wantDig {
				t.Errorf("layer digest = %q, want %q", got.Digest, tt.wantDig)
			}
		})
	}
}

// TestFetchOCIEmptyIndex covers the robustness branch that rejects an image index carrying no
// manifests, rather than dereferencing a nonexistent first entry.
func TestFetchOCIEmptyIndex(t *testing.T) {
	t.Parallel()

	host := startOCIRegistry(t)
	ctx := t.Context()
	repo, err := remote.NewRepository(host + "/devcontainers/features/empty")
	if err != nil {
		t.Fatal(err)
	}
	repo.PlainHTTP = true

	indexBytes := mustMarshal(t, ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
	})
	indexDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromBytes(indexBytes),
		Size:      int64(len(indexBytes)),
	}
	if err := repo.PushReference(ctx, indexDesc, bytes.NewReader(indexBytes), "1"); err != nil {
		t.Fatal(err)
	}

	_, err = NewFetcher().Fetch(ctx, host+"/devcontainers/features/empty:1", nil, "")
	if err == nil {
		t.Fatal("Fetch of an index with no manifests: got nil error")
	}
	if !strings.Contains(err.Error(), "no manifests") {
		t.Errorf("error = %v, want it to mention the empty index", err)
	}
}

func TestFetchOCIRejectsOversizedLayer(t *testing.T) {
	t.Parallel()

	host := startOCIRegistry(t)
	ctx := t.Context()
	repo, err := remote.NewRepository(host + "/devcontainers/features/big")
	if err != nil {
		t.Fatal(err)
	}
	repo.PlainHTTP = true

	push := func(mediaType string, data []byte) ocispec.Descriptor {
		desc := ocispec.Descriptor{MediaType: mediaType, Digest: digest.FromBytes(data), Size: int64(len(data))}
		if err := repo.Push(ctx, desc, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		return desc
	}

	configDesc := push("application/vnd.devcontainers", []byte("{}"))
	layerDesc := push(featureLayerMediaType, archiveWithMetadata(t, `{"id": "big"}`, false))
	// Declare a layer larger than the cap while the stored blob is unchanged: the OCI path cannot
	// stream (it must hash the whole blob to verify the digest), so the declared size is the only
	// bound and must be enforced before the blob is fetched.
	layerDesc.Size = maxArchiveBytes + 1

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
	if err := repo.PushReference(ctx, manDesc, bytes.NewReader(manBytes), "1"); err != nil {
		t.Fatal(err)
	}

	_, err = NewFetcher().Fetch(ctx, host+"/devcontainers/features/big:1", nil, "")
	if err == nil {
		t.Fatal("Fetch of an oversized layer: got nil error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want it to mention exceeding the size limit", err)
	}
}
