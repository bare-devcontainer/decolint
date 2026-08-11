package rules

import (
	"slices"
	"testing"

	"github.com/bare-devcontainer/decolint/feature"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// TestFeatureRefsOfKind covers each form the specification defines, and the references that are none
// of them: a rule asks for the kind it can report on and must be handed nothing else.
func TestFeatureRefsOfKind(t *testing.T) {
	t.Parallel()

	// One object carrying every form, so each case sees the ones it must leave behind.
	const src = `{
  "ghcr.io/devcontainers/features/go:1.3.2": {},
  "localhost:5000/features/foo": {},
  "./local-feature": {},
  "../sibling-feature": {},
  "https://example.invalid/devcontainer-feature.tgz": {},
  "http://example.invalid/devcontainer-feature.tgz": {},
  "/absolute/feature": {},
  "no-slash": {},
  "GHCR.IO/UPPER/CASE": {}
}`

	tests := []struct {
		name string
		kind feature.RefKind
		want []string
	}{
		{
			// An absolute path, a bare name and an upper-case registry are none of the three forms:
			// the specification's local form is a relative path, and an OCI reference needs a
			// registry it can be fetched from.
			name: "OCI",
			kind: feature.KindOCI,
			want: []string{"ghcr.io/devcontainers/features/go:1.3.2", "localhost:5000/features/foo"},
		},
		{
			name: "local",
			kind: feature.KindLocal,
			want: []string{"./local-feature", "../sibling-feature"},
		},
		{
			// The tarball form is an HTTPS URI; the "http://" spelling is not one.
			name: "tarball",
			kind: feature.KindTarball,
			want: []string{"https://example.invalid/devcontainer-feature.tgz"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			value, err := hujson.Parse([]byte(src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var got []string
			for _, ref := range featureRefsOfKind(&value, tt.kind) {
				got = append(got, ref.ref)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("featureRefsOfKind(%v) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}

	t.Run("a value that is not an object", func(t *testing.T) {
		t.Parallel()

		value, err := hujson.Parse([]byte(`"not an object"`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := featureRefsOfKind(&value, feature.KindOCI); got != nil {
			t.Errorf("featureRefsOfKind = %v, want none", got)
		}
	})
}

// TestHoldsFeatureRefs covers every file type, including the one no rule declaring these paths
// applies to, since the answer for it is part of the contract rather than a case that cannot arise.
func TestHoldsFeatureRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileType linter.FileType
		pointer  string
		want     bool
	}{
		{"devcontainer features", linter.Devcontainer, "/features", true},
		{"devcontainer dependsOn", linter.Devcontainer, "/dependsOn", false},
		{"feature dependsOn", linter.Feature, "/dependsOn", true},
		{"feature features", linter.Feature, "/features", false},
		{"template features", linter.Template, "/features", false},
		{"template dependsOn", linter.Template, "/dependsOn", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := holdsFeatureRefs(tt.fileType, tt.pointer); got != tt.want {
				t.Errorf("holdsFeatureRefs(%q, %q) = %v, want %v", tt.fileType, tt.pointer, got, tt.want)
			}
		})
	}
}
