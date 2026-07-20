package linter

import (
	"slices"
	"testing"

	"github.com/tailscale/hujson"
)

func parseValue(t *testing.T, src string) hujson.Value {
	t.Helper()
	v, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return v
}

// pathSpy returns a stub Rule that subscribes to the given paths. Check is a no-op; tests observe
// what walk visits via their own visit callback instead.
func pathSpy(id string, paths []string) *Rule {
	return &Rule{
		ID:          id,
		Description: "records visited paths",
		FileTypes:   []FileType{Devcontainer},
		Paths:       paths,
		Check:       func(*Context, *Node) []Finding { return nil },
	}
}

func TestWalk_Dispatch(t *testing.T) {
	t.Parallel()

	src := `{
  "image": "ubuntu:24.04",
  "mounts": [
    { "source": "a", "target": "/a", "type": "volume" },
    { "source": "b", "target": "/b", "type": "volume" }
  ],
  "customizations": {
    "vscode": {
      "extensions": ["golang.go", "foo.bar"]
    }
  },
  "a/b": { "image": "nested" }
}`
	tests := []struct {
		name  string
		paths []string
		want  []string // pointers walk must visit, in order
	}{
		{"exact path", []string{"/image"}, []string{"/image"}},
		{"wildcard over array", []string{"/mounts/*"}, []string{"/mounts/0", "/mounts/1"}},
		{"nested wildcard", []string{"/customizations/vscode/extensions/*"},
			[]string{"/customizations/vscode/extensions/0", "/customizations/vscode/extensions/1"}},
		{"document root", []string{""}, []string{""}},
		{"wildcard over object members", []string{"/mounts/0/*"},
			[]string{"/mounts/0/source", "/mounts/0/target", "/mounts/0/type"}},
		{"pattern anchors at root", []string{"/vscode"}, nil},
		{"escaped segment", []string{"/a~1b/image"}, []string{"/a~1b/image"}},
		{"overlapping patterns visit once", []string{"/image", "/*"},
			[]string{"/image", "/mounts", "/customizations", "/a~1b"}},
		{"no match", []string{"/build/dockerfile"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls []string
			root := parseValue(t, src)
			patterns := compilePatterns(pathSpy("spy", tt.paths))
			walk(&root, "", nil, patterns, func(_ *Rule, node *Node) {
				calls = append(calls, node.Pointer)
			})
			if !slices.Equal(calls, tt.want) {
				t.Errorf("visited %v, want %v", calls, tt.want)
			}
		})
	}
}

// TestWalk_SingleTraversal checks that adding rules does not cause the tree to be traversed again:
// each of two rules subscribing to the same path is visited exactly once at that path.
func TestWalk_SingleTraversal(t *testing.T) {
	t.Parallel()

	src := `{ "image": "ubuntu:24.04" }`
	root := parseValue(t, src)
	patterns := append(
		compilePatterns(pathSpy("a", []string{"/image"})),
		compilePatterns(pathSpy("b", []string{"/image"}))...,
	)
	var callsA, callsB []string
	walk(&root, "", nil, patterns, func(r *Rule, node *Node) {
		switch r.ID {
		case "a":
			callsA = append(callsA, node.Pointer)
		case "b":
			callsB = append(callsB, node.Pointer)
		}
	})
	if !slices.Equal(callsA, []string{"/image"}) || !slices.Equal(callsB, []string{"/image"}) {
		t.Errorf("calls = %v / %v, want a single /image call for each rule", callsA, callsB)
	}
}

func TestSplitPointer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ptr  string
		want []string
	}{
		{"", nil},
		{"/image", []string{"image"}},
		{"/mounts/*", []string{"mounts", "*"}},
		{"/a~1b/c~0d", []string{"a/b", "c~d"}},
	}
	for _, tt := range tests {
		if got := splitPointer(tt.ptr); !slices.Equal(got, tt.want) {
			t.Errorf("splitPointer(%q) = %v, want %v", tt.ptr, got, tt.want)
		}
	}
}
