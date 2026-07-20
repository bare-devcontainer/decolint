package ocitest_test

import (
	"bytes"
	"testing"

	"github.com/bare-devcontainer/decolint/feature"
	"github.com/bare-devcontainer/decolint/ocitest"
)

func TestFeatureArchive(t *testing.T) {
	t.Parallel()

	plain := ocitest.FeatureArchive(t, `{"id": "example"}`, false)
	compressed := ocitest.FeatureArchive(t, `{"id": "example"}`, true)

	// An uncompressed archive is a raw tar; a compressed one is gzip, identified by its magic bytes.
	if bytes.HasPrefix(plain, []byte{0x1f, 0x8b}) {
		t.Error("uncompressed archive unexpectedly starts with the gzip magic bytes")
	}
	if !bytes.HasPrefix(compressed, []byte{0x1f, 0x8b}) {
		t.Error("compressed archive does not start with the gzip magic bytes")
	}
}

func TestPushFeature(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)

	tests := []struct {
		name    string
		asIndex bool
	}{
		{"manifest", false},
		{"index", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := "features/" + tt.name
			// The OCI layer is an uncompressed tar, so the archive is published uncompressed.
			ocitest.PushFeature(t, host, repo, "1",
				ocitest.FeatureArchive(t, `{"id": "example", "version": "1.0.0"}`, false), tt.asIndex)

			// The published artifact must be fetchable by the real fetcher, confirming the helper
			// produces a spec-conformant Feature (through the index when asIndex is set).
			md, err := feature.NewFetcher().Fetch(t.Context(), host+"/"+repo+":1", nil, "")
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if md.ID != "example" || md.Version != "1.0.0" {
				t.Errorf("got ID %q version %q, want example 1.0.0", md.ID, md.Version)
			}
		})
	}
}
