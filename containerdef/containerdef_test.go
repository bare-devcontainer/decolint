package containerdef_test

import (
	"fmt"
	"testing"

	"github.com/bare-devcontainer/decolint/containerdef"
	"github.com/google/go-cmp/cmp"
	"github.com/tailscale/hujson"
)

// object parses src and returns its root object, failing the test if it is not one.
func object(t *testing.T, src string) *hujson.Object {
	t.Helper()

	value, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	obj, ok := value.Value.(*hujson.Object)
	if !ok {
		t.Fatalf("parsed %s, want an object", src)
	}
	return obj
}

// defOf returns the definition of type T among the ones src declares, or the zero value when it
// declares none of that form.
func defOf[T containerdef.Def](t *testing.T, src string) T {
	t.Helper()

	var found T
	for def := range containerdef.Defs(object(t, src)) {
		if typed, ok := def.(T); ok {
			found = typed
		}
	}
	return found
}

func TestDefs_Image(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
		ok   bool
	}{
		{"a named image", `{"image": "ubuntu:24.04"}`, "ubuntu:24.04", true},
		{"no image", `{"name": "x"}`, "", false},
		{"a non-string image", `{"image": 42}`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := defOf[*containerdef.ImageDef](t, tt.src)
			if (got != nil) != tt.ok {
				t.Fatalf("image def = %v, want one: %v", got, tt.ok)
			}
			if got != nil && got.Ref != tt.want {
				t.Errorf("Ref = %q, want %q", got.Ref, tt.want)
			}
		})
	}
}

func TestDefs_Build(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		wantFile string
		wantArgs map[string]string
		wantTgt  string
		ok       bool
	}{
		{name: "build.dockerfile", src: `{"build": {"dockerfile": "Dockerfile"}}`, wantFile: "Dockerfile", ok: true},
		{name: "the legacy top-level property", src: `{"dockerFile": "Dockerfile"}`, wantFile: "Dockerfile", ok: true},
		{
			// The reference implementation reads the top-level property first, so it is the one
			// built when a configuration carries both.
			name:     "the top-level property wins",
			src:      `{"dockerFile": "top", "build": {"dockerfile": "nested"}}`,
			wantFile: "top", ok: true,
		},
		{
			// The legacy form names the Dockerfile while "build" still shapes what it produces.
			name:     "the legacy property carries the options beside it",
			src:      `{"dockerFile": "Dockerfile", "build": {"args": {"A": "1"}, "target": "dev"}}`,
			wantFile: "Dockerfile", wantArgs: map[string]string{"A": "1"}, wantTgt: "dev", ok: true,
		},
		{
			name:     "args and target",
			src:      `{"build": {"dockerfile": "Dockerfile", "args": {"A": "1"}, "target": "dev"}}`,
			wantFile: "Dockerfile", wantArgs: map[string]string{"A": "1"}, wantTgt: "dev", ok: true,
		},
		{
			name:     "a non-string arg is left out",
			src:      `{"build": {"dockerfile": "Dockerfile", "args": {"A": 1, "B": "2"}}}`,
			wantFile: "Dockerfile", wantArgs: map[string]string{"B": "2"}, ok: true,
		},
		{
			name:     "a non-string target is no target",
			src:      `{"build": {"dockerfile": "Dockerfile", "target": 42}}`,
			wantFile: "Dockerfile", ok: true,
		},
		{name: "no Dockerfile", src: `{"image": "ubuntu:24.04"}`},
		{name: "a non-object build", src: `{"build": "Dockerfile"}`},
		{name: "a non-string dockerfile", src: `{"build": {"dockerfile": 42}}`},
		{name: "options without a Dockerfile build nothing", src: `{"build": {"target": "dev"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := defOf[*containerdef.BuildDef](t, tt.src)
			if (got != nil) != tt.ok {
				t.Fatalf("build def = %v, want one: %v", got, tt.ok)
			}
			if got == nil {
				return
			}
			if got.Dockerfile != tt.wantFile {
				t.Errorf("Dockerfile = %q, want %q", got.Dockerfile, tt.wantFile)
			}
			if diff := cmp.Diff(tt.wantArgs, got.Args); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
			if got.Target != tt.wantTgt {
				t.Errorf("target = %q, want %q", got.Target, tt.wantTgt)
			}
		})
	}
}

func TestDefs_BuildOffsets(t *testing.T) {
	t.Parallel()

	// `{"build": {"dockerfile": "Dockerfile"}}` — the key opens at 11 and the value at 25.
	got := defOf[*containerdef.BuildDef](t, `{"build": {"dockerfile": "Dockerfile"}}`)
	if got == nil {
		t.Fatal("no build declared")
	}
	if got.DockerfileDecl.KeyOffset != 11 || got.DockerfileDecl.ValueOffset != 25 {
		t.Errorf("decl = %+v, want {KeyOffset:11 ValueOffset:25}", got.DockerfileDecl)
	}
}

func TestDefs_Compose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		src          string
		wantFiles    []string
		wantService  string
		wantDeclared bool
		wantUsable   bool
	}{
		{
			name:         "a single file and a service",
			src:          `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			wantFiles:    []string{"docker-compose.yml"},
			wantService:  "app",
			wantDeclared: true,
			wantUsable:   true,
		},
		{
			name:         "a list of files",
			src:          `{"dockerComposeFile": ["a.yml", "b.yml"], "service": "app"}`,
			wantFiles:    []string{"a.yml", "b.yml"},
			wantService:  "app",
			wantDeclared: true,
			wantUsable:   true,
		},
		{name: "not declared", src: `{"image": "ubuntu:24.04"}`},
		{
			// The configuration says it is Compose-based, so a caller must not fall back to another
			// form, even though there is nothing to attach to.
			name:         "declared without a service",
			src:          `{"dockerComposeFile": "docker-compose.yml"}`,
			wantFiles:    []string{"docker-compose.yml"},
			wantDeclared: true,
		},
		{
			name:         "declared with no readable path",
			src:          `{"dockerComposeFile": 42, "service": "app"}`,
			wantService:  "app",
			wantDeclared: true,
		},
		{
			name:         "a non-string element is left out",
			src:          `{"dockerComposeFile": ["a.yml", 42], "service": "app"}`,
			wantFiles:    []string{"a.yml"},
			wantService:  "app",
			wantDeclared: true,
			wantUsable:   true,
		},
		{
			name:         "a non-string service is no service",
			src:          `{"dockerComposeFile": "docker-compose.yml", "service": 42}`,
			wantFiles:    []string{"docker-compose.yml"},
			wantDeclared: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := defOf[*containerdef.ComposeDef](t, tt.src)
			if (got != nil) != tt.wantDeclared {
				t.Fatalf("compose def = %v, want one: %v", got, tt.wantDeclared)
			}
			if got == nil {
				return
			}
			if diff := cmp.Diff(tt.wantFiles, got.Files); diff != "" {
				t.Errorf("files mismatch (-want +got):\n%s", diff)
			}
			if got.Service != tt.wantService {
				t.Errorf("service = %q, want %q", got.Service, tt.wantService)
			}
			if got.Usable() != tt.wantUsable {
				t.Errorf("Usable = %v, want %v", got.Usable(), tt.wantUsable)
			}
		})
	}
}

// TestCompose_Offsets checks that both properties are located, since the merge anchors its findings
// at the key and a rule at the value.
func TestDefs_ComposeOffsets(t *testing.T) {
	t.Parallel()

	// `{"dockerComposeFile": "c.yml", "service": "app"}` — the key opens at 1 and the value at 22;
	// "service" opens at 31 with its value at 42.
	got := defOf[*containerdef.ComposeDef](t, `{"dockerComposeFile": "c.yml", "service": "app"}`)
	if got == nil {
		t.Fatal("no compose declared")
	}
	if got.FilesDecl.KeyOffset != 1 || got.FilesDecl.ValueOffset != 22 {
		t.Errorf("FilesDecl = %+v, want {KeyOffset:1 ValueOffset:22}", got.FilesDecl)
	}
	if got.ServiceDecl.KeyOffset != 31 || got.ServiceDecl.ValueOffset != 42 {
		t.Errorf("ServiceDecl = %+v, want {KeyOffset:31 ValueOffset:42}", got.ServiceDecl)
	}
}

// TestDefs_Order checks the order the forms come in, which is the order the reference implementation
// resolves them: a caller taking the first takes the one the tooling would build.
func TestDefs_Order(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"nothing declared", `{"name": "x"}`, nil},
		{"an image alone", `{"image": "ubuntu:24.04"}`, []string{"image"}},
		{
			"compose comes before a build beside it",
			`{"dockerComposeFile": "c.yml", "service": "app", "build": {"dockerfile": "Dockerfile"}}`,
			[]string{"compose", "build"},
		},
		{
			"a build comes before an image beside it",
			`{"build": {"dockerfile": "Dockerfile"}, "image": "ubuntu:24.04"}`,
			[]string{"build", "image"},
		},
		{
			"all three",
			`{"dockerComposeFile": "c.yml", "service": "app", "build": {"dockerfile": "Dockerfile"}, "image": "ubuntu:24.04"}`,
			[]string{"compose", "build", "image"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got []string
			for def := range containerdef.Defs(object(t, tt.src)) {
				switch def.(type) {
				case *containerdef.ComposeDef:
					got = append(got, "compose")
				case *containerdef.BuildDef:
					got = append(got, "build")
				case *containerdef.ImageDef:
					got = append(got, "image")
				}
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("forms mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDefs_StopsEarly checks that a caller stops where it says it does: the forms after the one it
// stops at are read as they are reached, not before, so a caller taking the first reads only it.
func TestDefs_StopsEarly(t *testing.T) {
	t.Parallel()

	// A configuration declaring all three, so there is always a form left to stop before.
	const src = `{"dockerComposeFile": "c.yml", "service": "app", "build": {"dockerfile": "Dockerfile"}, "image": "ubuntu:24.04"}`

	for _, stopAfter := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("stopping after %d", stopAfter), func(t *testing.T) {
			t.Parallel()

			var read int
			for range containerdef.Defs(object(t, src)) {
				read++
				if read == stopAfter {
					break
				}
			}
			if read != stopAfter {
				t.Errorf("read %d definitions, want %d", read, stopAfter)
			}
		})
	}
}
