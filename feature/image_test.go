package feature

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/ocitest"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
)

func TestParseImageRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string // registry/repository:reference
		wantErr bool
	}{
		{"bare name defaults to the library namespace and latest", "ubuntu", "docker.io/library/ubuntu:latest", false},
		{"tagged bare name", "ubuntu:24.04", "docker.io/library/ubuntu:24.04", false},
		{"user repository on docker hub", "user/repo", "docker.io/user/repo:latest", false},
		{"explicit registry and tag", "mcr.microsoft.com/devcontainers/base:jammy", "mcr.microsoft.com/devcontainers/base:jammy", false},
		{"digest reference", "ghcr.io/x/y@sha256:" + strings.Repeat("a", 64), "ghcr.io/x/y@sha256:" + strings.Repeat("a", 64), false},
		{"localhost registry", "localhost:5000/x", "localhost:5000/x:latest", false},
		{"loopback registry with port", "127.0.0.1:5000/x:1", "127.0.0.1:5000/x:1", false},
		{"index.docker.io normalizes to docker.io", "index.docker.io/library/ubuntu", "docker.io/library/ubuntu:latest", false},
		{"invalid uppercase repository", "Ubuntu", "", true},
		{"empty tag from an unset variable", "registry.invalid/app:", "", true},
		{"empty tag on a registry with a port", "localhost:5000/app:", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ref, err := parseImageRef(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseImageRef(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got := ref.Registry + "/" + ref.Repository + refSeparator(ref.Reference) + ref.Reference
			if got != tt.want {
				t.Errorf("parseImageRef(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// refSeparator returns the separator that joins a repository to a digest ("@") or a tag (":").
func refSeparator(reference string) string {
	if strings.Contains(reference, ":") {
		return "@"
	}
	return ":"
}

func TestImageMetadataEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		label   string
		wantLen int
		wantIDs []string
	}{
		{"array of entries", `[{"id": "a", "privileged": true}, {"id": "b"}]`, 2, []string{"a", "b"}},
		{"single object is wrapped", `{"id": "solo", "init": true}`, 1, []string{"solo"}},
		{"non-object array elements are skipped", `[{"id": "a"}, "x", 3]`, 1, []string{"a"}},
		{"empty array", `[]`, 0, nil},
		{"malformed JSON is ignored", `[{"id":`, 0, nil},
		{"scalar is ignored", `"metadata"`, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries := imageMetadataEntries([]byte(tt.label))
			if len(entries) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(entries), tt.wantLen)
			}
			for i, id := range tt.wantIDs {
				if entries[i].ID != id {
					t.Errorf("entry %d ID = %q, want %q", i, entries[i].ID, id)
				}
			}
		})
	}
}

func TestFetchImageMetadata(t *testing.T) {
	t.Parallel()

	t.Run("reads array label entries in order", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "app", "1", map[string]string{
			imageMetadataLabel: `[{"id": "a", "privileged": true}, {"id": "b", "remoteUser": "vscode"}]`,
		}, false)

		entries, err := NewFetcher().FetchImageMetadata(t.Context(), host+"/app:1")
		if err != nil {
			t.Fatalf("FetchImageMetadata: %v", err)
		}
		if len(entries) != 2 || entries[0].ID != "a" || entries[1].ID != "b" {
			t.Fatalf("entries = %+v, want ids [a b]", entries)
		}
		if entries[0].Root.Find("/privileged") == nil {
			t.Error("first entry lost its /privileged property")
		}
	})

	t.Run("wraps a single object label", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "app", "1", map[string]string{
			imageMetadataLabel: `{"id": "solo", "init": true}`,
		}, false)

		entries, err := NewFetcher().FetchImageMetadata(t.Context(), host+"/app:1")
		if err != nil {
			t.Fatalf("FetchImageMetadata: %v", err)
		}
		if len(entries) != 1 || entries[0].ID != "solo" {
			t.Fatalf("entries = %+v, want one entry solo", entries)
		}
	})

	t.Run("image without the label yields nothing", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "app", "1", map[string]string{"other": "value"}, false)

		entries, err := NewFetcher().FetchImageMetadata(t.Context(), host+"/app:1")
		if err != nil {
			t.Fatalf("FetchImageMetadata: %v", err)
		}
		if entries != nil {
			t.Errorf("entries = %+v, want nil", entries)
		}
	})

	t.Run("malformed label is ignored", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "app", "1", map[string]string{imageMetadataLabel: `[{"id":`}, false)

		entries, err := NewFetcher().FetchImageMetadata(t.Context(), host+"/app:1")
		if err != nil {
			t.Fatalf("FetchImageMetadata: %v", err)
		}
		if entries != nil {
			t.Errorf("entries = %+v, want nil for a malformed label", entries)
		}
	})

	t.Run("follows an image index", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "app", "1", map[string]string{
			imageMetadataLabel: `[{"id": "a"}]`,
		}, true)

		entries, err := NewFetcher().FetchImageMetadata(t.Context(), host+"/app:1")
		if err != nil {
			t.Fatalf("FetchImageMetadata: %v", err)
		}
		if len(entries) != 1 || entries[0].ID != "a" {
			t.Fatalf("entries = %+v, want one entry a", entries)
		}
	})

	t.Run("unknown repository is an error", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		_, err := NewFetcher().FetchImageMetadata(t.Context(), host+"/absent:1")
		if err == nil {
			t.Fatal("FetchImageMetadata of an absent image: got nil error")
		}
	})

	t.Run("caches the underlying image config", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "app", "1", map[string]string{imageMetadataLabel: `[{"id": "a"}]`}, false)

		var log strings.Builder
		f := NewFetcher(WithLogWriter(&log))
		for range 2 {
			entries, err := f.FetchImageMetadata(t.Context(), host+"/app:1")
			if err != nil {
				t.Fatalf("FetchImageMetadata: %v", err)
			}
			if len(entries) != 1 || entries[0].ID != "a" {
				t.Fatalf("entries = %+v, want one entry a", entries)
			}
		}
		// The config is fetched once and reused; the label parse repeats but no second download does.
		if got := strings.Count(log.String(), "Downloading image metadata"); got != 1 {
			t.Errorf("downloads = %d, want 1 (the config must be cached):\n%s", got, log.String())
		}
	})
}

func TestFetchImageMetadata_RejectsOversizedConfig(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ctx := t.Context()
	repo, err := remote.NewRepository(host + "/big")
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

	configDesc := push(ocispec.MediaTypeImageConfig, []byte("{}"))
	layerDesc := push(ocispec.MediaTypeImageLayer, []byte("layer"))
	// Declare a config larger than the cap while the stored blob is unchanged: the size guard must
	// reject it before the blob is fetched.
	configDesc.Size = maxImageConfigBytes + 1

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

	_, err = NewFetcher().FetchImageMetadata(ctx, host+"/big:1")
	if err == nil {
		t.Fatal("FetchImageMetadata of an oversized config: got nil error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want it to mention exceeding the size limit", err)
	}
}
