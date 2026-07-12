package feature

import "testing"

func TestParseRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw     string
		want    Ref
		wantErr bool
	}{
		{
			raw:  "ghcr.io/devcontainers/features/node:1",
			want: Ref{Raw: "ghcr.io/devcontainers/features/node:1", Kind: KindOCI, Registry: "ghcr.io", Repository: "devcontainers/features/node", Tag: "1"},
		},
		{
			raw:  "ghcr.io/devcontainers/features/node",
			want: Ref{Raw: "ghcr.io/devcontainers/features/node", Kind: KindOCI, Registry: "ghcr.io", Repository: "devcontainers/features/node", Tag: "latest"},
		},
		{
			raw: "ghcr.io/devcontainers/features/node@sha256:0123456789abcdef",
			want: Ref{
				Raw: "ghcr.io/devcontainers/features/node@sha256:0123456789abcdef", Kind: KindOCI,
				Registry: "ghcr.io", Repository: "devcontainers/features/node", Digest: "sha256:0123456789abcdef",
			},
		},
		{
			raw:  "localhost:5000/features/go:2",
			want: Ref{Raw: "localhost:5000/features/go:2", Kind: KindOCI, Registry: "localhost:5000", Repository: "features/go", Tag: "2"},
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

func TestRefWithoutVersion(t *testing.T) {
	t.Parallel()

	tests := []struct{ ref, want string }{
		{"ghcr.io/devcontainers/features/node:1", "ghcr.io/devcontainers/features/node"},
		{"ghcr.io/devcontainers/features/node", "ghcr.io/devcontainers/features/node"},
		{"ghcr.io/devcontainers/features/node@sha256:abc", "ghcr.io/devcontainers/features/node"},
		{"localhost:5000/features/go:2", "localhost:5000/features/go"},
		{"localhost:5000/features/go", "localhost:5000/features/go"},
	}
	for _, tt := range tests {
		if got := refWithoutVersion(tt.ref); got != tt.want {
			t.Errorf("refWithoutVersion(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}
