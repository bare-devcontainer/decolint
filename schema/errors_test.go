package schema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

func TestNearest(t *testing.T) {
	t.Parallel()

	candidates := map[string]bool{"forwardPorts": true, "workspaceFolder": true, "image": true}
	tests := []struct {
		name string
		want string
	}{
		{"forwardPort", "forwardPorts"}, // one edit away
		{"workspaceFolde", "workspaceFolder"},
		{"image", ""},               // equals a candidate: never suggests itself
		{"completelyunrelated", ""}, // too far from any candidate
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := nearest(tt.name, candidates); got != tt.want {
				t.Errorf("nearest(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestValueMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		leaf leaf
		want string
	}{
		{
			name: "type",
			leaf: leaf{loc: []string{"forwardPorts"}, kind: &kind.Type{Got: "string", Want: []string{"array"}}},
			want: `property "/forwardPorts" must be array, but is string`,
		},
		{
			name: "enum",
			leaf: leaf{loc: []string{"shutdownAction"}, kind: &kind.Enum{Got: "x", Want: []any{"none"}}},
			want: `property "/shutdownAction" has an unsupported value`,
		},
		{
			name: "single missing required",
			leaf: leaf{loc: nil, kind: &kind.Required{Missing: []string{"image"}}},
			want: `missing required property "image"`,
		},
		{
			name: "multiple missing required",
			leaf: leaf{loc: nil, kind: &kind.Required{Missing: []string{"dockerComposeFile", "service"}}},
			want: `missing required properties "dockerComposeFile", "service"`,
		},
		{
			name: "fallback uses the library wording",
			leaf: leaf{loc: []string{"runArgs"}, kind: &kind.MinItems{Got: 0, Want: 1}},
			want: `property "/runArgs": minItems: got 0, want 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := valueMessage(tt.leaf); got != tt.want {
				t.Errorf("valueMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubject(t *testing.T) {
	t.Parallel()

	if got := subject(nil); got != "the document" {
		t.Errorf("subject(nil) = %q, want %q", got, "the document")
	}
	if got := subject([]string{"a", "0"}); got != `property "/a/0"` {
		t.Errorf("subject = %q, want %q", got, `property "/a/0"`)
	}
}

func TestDedupe(t *testing.T) {
	t.Parallel()

	in := []Diagnostic{
		{Message: "b", Offset: 10},
		{Message: "a", Offset: 5},
		{Message: "a", Offset: 5}, // exact duplicate, dropped
		{Message: "a", Offset: 10},
	}
	want := []Diagnostic{
		{Message: "a", Offset: 5},
		{Message: "a", Offset: 10},
		{Message: "b", Offset: 10},
	}
	if diff := cmp.Diff(want, dedupe(in)); diff != "" {
		t.Errorf("dedupe mismatch (-want +got):\n%s", diff)
	}
}
