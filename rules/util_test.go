package rules

import (
	"slices"
	"testing"

	"github.com/bare-devcontainer/decolint/feature"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// TestFeatureRefs covers each form the specification defines and the references that are none of
// them, which belong to no kind and are left out.
func TestFeatureRefs(t *testing.T) {
	t.Parallel()

	// One object carrying every form, so each kind is seen alongside the ones it is told apart from.
	// The references left out are none of the three forms:
	//   - "http://...": the tarball form is an HTTPS URI;
	//   - "/absolute/feature": the local form is a relative path;
	//   - "no-slash": an OCI reference needs a registry to be fetched from;
	//   - "GHCR.IO/UPPER/CASE": an OCI reference is lower-case.
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

	type kindOf struct {
		ref  string
		kind feature.RefKind
	}
	want := []kindOf{
		{"ghcr.io/devcontainers/features/go:1.3.2", feature.KindOCI},
		{"localhost:5000/features/foo", feature.KindOCI},
		{"./local-feature", feature.KindLocal},
		{"../sibling-feature", feature.KindLocal},
		{"https://example.invalid/devcontainer-feature.tgz", feature.KindTarball},
	}

	value, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var got []kindOf
	for _, ref := range featureRefs(&value) {
		got = append(got, kindOf{ref.ref, ref.kind})
	}
	if !slices.Equal(got, want) {
		t.Errorf("featureRefs = %+v, want %+v", got, want)
	}
}

// TestFeatureRefs_NotAnObject covers a property whose value is not an object of references, which a
// rule is offered as readily as a well-formed one.
func TestFeatureRefs_NotAnObject(t *testing.T) {
	t.Parallel()

	value, err := hujson.Parse([]byte(`"not an object"`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := featureRefs(&value); got != nil {
		t.Errorf("featureRefs = %+v, want none", got)
	}
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
