package linter

import (
	"slices"
	"testing"

	"github.com/bare-devcontainer/decolint/dockerargs"
	"github.com/google/go-cmp/cmp"
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
			walk(&root, Devcontainer, patterns, func(_ *Rule, node *Node) {
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
	walk(&root, Devcontainer, patterns, func(r *Rule, node *Node) {
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

func TestRunArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []dockerargs.Arg
	}{
		{"empty array", `[]`, nil},
		{"flag and value in one element", `["--cap-drop=ALL"]`, []dockerargs.Arg{
			{Flag: "cap-drop", Value: "ALL", Index: 0},
		}},
		{"value in the following element", `["--cap-drop", "ALL"]`, []dockerargs.Arg{
			{Flag: "cap-drop", Value: "ALL", Index: 1},
		}},
		// A non-string element keeps its position so that the ones after it keep theirs.
		{"non-string element", `[123, "--privileged"]`, []dockerargs.Arg{
			{Flag: "privileged", Value: "true", Index: 1},
		}},
		{"non-string element consumed as a value", `["--label", 123, "--privileged"]`, []dockerargs.Arg{
			{Flag: "label", Value: "", Index: 1},
			{Flag: "privileged", Value: "true", Index: 2},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := parseValue(t, tt.src)
			arr, ok := v.Value.(*hujson.Array)
			if !ok {
				t.Fatalf("%s is not an array", tt.src)
			}
			if diff := cmp.Diff(tt.want, RunArgs(arr)); diff != "" {
				t.Errorf("RunArgs(%s) mismatch (-want +got):\n%s", tt.src, diff)
			}
		})
	}
}

// TestWalk_RunArgs checks the traversal of a devcontainer.json's "runArgs" as the "docker run" argv
// it becomes: its elements are reached by flag rather than by index, and each of them at most once.
func TestWalk_RunArgs(t *testing.T) {
	t.Parallel()

	// visit is one (rule, value) pair walk produced: where the value is, its source text, and the
	// flag occurrence it was reached as, if any.
	type visit struct {
		pointer string
		element string // the visited value, as written in the source
		flag    string
		value   string
	}
	tests := []struct {
		name  string
		paths []string
		src   string
		want  []visit
	}{
		{"long flag holding its value", []string{"/runArgs/--cap-add"}, `{"runArgs": ["--cap-add=ALL"]}`,
			[]visit{{"/runArgs/0", `"--cap-add=ALL"`, "cap-add", "ALL"}}},
		{"long flag consuming the next element", []string{"/runArgs/--cap-add"}, `{"runArgs": ["--cap-add", "ALL"]}`,
			[]visit{{"/runArgs/1", `"ALL"`, "cap-add", "ALL"}}},
		{"shorthand reaches the long spelling", []string{"/runArgs/--volume"}, `{"runArgs": ["-v", "/a:/b"]}`,
			[]visit{{"/runArgs/1", `"/a:/b"`, "volume", "/a:/b"}}},
		{"one element naming several flags", []string{"/runArgs/--interactive", "/runArgs/--tty"}, `{"runArgs": ["-it"]}`,
			[]visit{{"/runArgs/0", `"-it"`, "interactive", "true"}, {"/runArgs/0", `"-it"`, "tty", "true"}}},
		{"every occurrence of a flag", []string{"/runArgs/--cap-add"}, `{"runArgs": ["--cap-add=ALL", "--cap-add=NET_ADMIN"]}`,
			[]visit{{"/runArgs/0", `"--cap-add=ALL"`, "cap-add", "ALL"}, {"/runArgs/1", `"--cap-add=NET_ADMIN"`, "cap-add", "NET_ADMIN"}}},
		{"another flag's value names no flag", []string{"/runArgs/--cap-add"}, `{"runArgs": ["--label", "--cap-add=ALL"]}`, nil},
		{"non-string element", []string{"/runArgs/--cap-add"}, `{"runArgs": [123, "--cap-add=ALL"]}`,
			[]visit{{"/runArgs/1", `"--cap-add=ALL"`, "cap-add", "ALL"}}},
		{"every copy of a duplicated member", []string{"/runArgs/--cap-add"},
			`{"runArgs": ["--cap-add=ALL"], "runArgs": ["--cap-add=NET_ADMIN"]}`,
			[]visit{{"/runArgs/0", `"--cap-add=ALL"`, "cap-add", "ALL"}, {"/runArgs/0", `"--cap-add=NET_ADMIN"`, "cap-add", "NET_ADMIN"}}},

		// The elements are addressed by flag only, so a wildcard reaches an element once per flag it
		// names — and only the elements a flag's value is written in.
		{"wildcard over flags holding their values", []string{"/runArgs/*"}, `{"runArgs": ["--privileged", "--init"]}`,
			[]visit{{"/runArgs/0", `"--privileged"`, "privileged", "true"}, {"/runArgs/1", `"--init"`, "init", "true"}}},
		{"wildcard over a flag consuming the next element", []string{"/runArgs/*"}, `{"runArgs": ["--cap-add", "ALL"]}`,
			[]visit{{"/runArgs/1", `"ALL"`, "cap-add", "ALL"}}},
		{"wildcard over one element naming several flags", []string{"/runArgs/*"}, `{"runArgs": ["-it"]}`,
			[]visit{{"/runArgs/0", `"-it"`, "interactive", "true"}, {"/runArgs/0", `"-it"`, "tty", "true"}}},

		{"the array itself", []string{"/runArgs"}, `{"runArgs": ["--cap-add=ALL"]}`,
			[]visit{{"/runArgs", `["--cap-add=ALL"]`, "", ""}}},
		{"a runArgs that is not an array", []string{"/runArgs/--cap-add"}, `{"runArgs": "--cap-add=ALL"}`, nil},
		{"a runArgs that is not the document's", []string{"/build/runArgs/*"}, `{"build": {"runArgs": ["--cap-add=ALL"]}}`,
			[]visit{{"/build/runArgs/0", `"--cap-add=ALL"`, "", ""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []visit
			root := parseValue(t, tt.src)
			patterns := compilePatterns(pathSpy("spy", tt.paths))
			walk(&root, Devcontainer, patterns, func(_ *Rule, node *Node) {
				v := visit{pointer: node.Pointer, element: tt.src[node.Value.StartOffset:node.Value.EndOffset]}
				if node.Arg != nil {
					v.flag, v.value = node.Arg.Flag, node.Arg.Value
				}
				got = append(got, v)
			})
			if !slices.Equal(got, tt.want) {
				t.Errorf("visited %v, want %v", got, tt.want)
			}
		})
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
