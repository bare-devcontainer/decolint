package feature

import (
	"testing"

	"oras.land/oras-go/v2/registry"
)

func TestParseRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw     string
		want    Ref
		wantErr bool
	}{
		{
			raw:  "ghcr.io/devcontainers/features/node:1",
			want: Ref{Raw: "ghcr.io/devcontainers/features/node:1", Kind: KindOCI, OCI: registry.Reference{Registry: "ghcr.io", Repository: "devcontainers/features/node", Reference: "1"}},
		},
		{
			raw:  "ghcr.io/devcontainers/features/node",
			want: Ref{Raw: "ghcr.io/devcontainers/features/node", Kind: KindOCI, OCI: registry.Reference{Registry: "ghcr.io", Repository: "devcontainers/features/node"}},
		},
		{
			raw: "ghcr.io/devcontainers/features/node@sha256:0000000000000000000000000000000000000000000000000000000000000000",
			want: Ref{
				Raw: "ghcr.io/devcontainers/features/node@sha256:0000000000000000000000000000000000000000000000000000000000000000", Kind: KindOCI,
				OCI: registry.Reference{Registry: "ghcr.io", Repository: "devcontainers/features/node", Reference: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
			},
		},
		{
			raw:  "localhost:5000/features/go:2",
			want: Ref{Raw: "localhost:5000/features/go:2", Kind: KindOCI, OCI: registry.Reference{Registry: "localhost:5000", Repository: "features/go", Reference: "2"}},
		},
		{
			raw:  "./local-feature",
			want: Ref{Raw: "./local-feature", Kind: KindLocal},
		},
		{
			raw:  "../sibling-feature",
			want: Ref{Raw: "../sibling-feature", Kind: KindLocal},
		},
		{
			raw:  "https://example.com/features/foo.tgz",
			want: Ref{Raw: "https://example.com/features/foo.tgz", Kind: KindTarball},
		},
		{raw: "no-slash", wantErr: true},
		{raw: "ghcr.io/features/node@md5:abc", wantErr: true},
		// Only https:// is a tarball Feature; a plain-HTTP URI is neither a tarball nor a valid OCI
		// reference and must be rejected.
		{raw: "http://example.com/features/foo.tgz", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRef(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRef(%q) = %+v, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRef(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ParseRef(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}
