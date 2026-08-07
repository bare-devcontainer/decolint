package containerdef_test

import (
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

func TestImage(t *testing.T) {
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
			got, _, ok := containerdef.Image(object(t, tt.src))
			if got != tt.want || ok != tt.ok {
				t.Errorf("Image = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDockerfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
		ok   bool
	}{
		{"build.dockerfile", `{"build": {"dockerfile": "Dockerfile"}}`, "Dockerfile", true},
		{"the legacy top-level property", `{"dockerFile": "Dockerfile"}`, "Dockerfile", true},
		{
			// The reference implementation reads the top-level property first, so it is the one
			// built when a configuration carries both.
			"the top-level property wins",
			`{"dockerFile": "top", "build": {"dockerfile": "nested"}}`,
			"top", true,
		},
		{"neither", `{"image": "ubuntu:24.04"}`, "", false},
		{"a non-object build", `{"build": "Dockerfile"}`, "", false},
		{"a non-string dockerfile", `{"build": {"dockerfile": 42}}`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _, ok := containerdef.Dockerfile(object(t, tt.src))
			if got != tt.want || ok != tt.ok {
				t.Errorf("Dockerfile = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDockerfile_Offsets(t *testing.T) {
	t.Parallel()

	// `{"build": {"dockerfile": "Dockerfile"}}` — the key opens at 11 and the value at 25.
	_, decl, ok := containerdef.Dockerfile(object(t, `{"build": {"dockerfile": "Dockerfile"}}`))
	if !ok {
		t.Fatal("Dockerfile: not found")
	}
	if decl.KeyOffset != 11 || decl.ValueOffset != 25 {
		t.Errorf("decl = %+v, want {KeyOffset:11 ValueOffset:25}", decl)
	}
}

func TestBuildOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		wantArgs map[string]string
		wantTgt  string
	}{
		{"args and target", `{"build": {"args": {"A": "1"}, "target": "dev"}}`, map[string]string{"A": "1"}, "dev"},
		{"neither", `{"build": {"dockerfile": "Dockerfile"}}`, nil, ""},
		{"no build", `{"image": "ubuntu:24.04"}`, nil, ""},
		{"a non-object build", `{"build": 42}`, nil, ""},
		{"a non-string arg is left out", `{"build": {"args": {"A": 1, "B": "2"}}}`, map[string]string{"B": "2"}, ""},
		{"a non-string target is no target", `{"build": {"target": 42}}`, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args, target := containerdef.BuildOptions(object(t, tt.src))
			if diff := cmp.Diff(tt.wantArgs, args); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
			if target != tt.wantTgt {
				t.Errorf("target = %q, want %q", target, tt.wantTgt)
			}
		})
	}
}

func TestComposeFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		src          string
		want         []string
		wantDeclared bool
	}{
		{"a single path", `{"dockerComposeFile": "docker-compose.yml"}`, []string{"docker-compose.yml"}, true},
		{"a list of paths", `{"dockerComposeFile": ["a.yml", "b.yml"]}`, []string{"a.yml", "b.yml"}, true},
		{"not declared", `{"image": "ubuntu:24.04"}`, nil, false},
		{
			// The configuration says it is Compose-based, so a caller must not fall back to another
			// form, even though there is no path to read.
			"declared but not a path",
			`{"dockerComposeFile": 42}`,
			nil, true,
		},
		{"a non-string element is left out", `{"dockerComposeFile": ["a.yml", 42]}`, []string{"a.yml"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _, declared := containerdef.ComposeFiles(object(t, tt.src))
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("paths mismatch (-want +got):\n%s", diff)
			}
			if declared != tt.wantDeclared {
				t.Errorf("declared = %v, want %v", declared, tt.wantDeclared)
			}
		})
	}
}

func TestComposeService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
		ok   bool
	}{
		{"a named service", `{"service": "app"}`, "app", true},
		{"no service", `{"dockerComposeFile": "docker-compose.yml"}`, "", false},
		{"a non-string service", `{"service": 42}`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _, ok := containerdef.ComposeService(object(t, tt.src))
			if got != tt.want || ok != tt.ok {
				t.Errorf("ComposeService = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
