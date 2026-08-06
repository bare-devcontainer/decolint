package rules

import (
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
)

// TestComposeServiceSource covers what a service is read as — an image, a build, or neither — since
// which one it is decides whether the Compose rule or the Dockerfile rules report on it.
func TestComposeServiceSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		files     map[string]string
		paths     []string
		wantOK    bool
		wantImage string
		wantBuild *composeBuild
	}{
		{
			name:      "an image",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    image: ubuntu:24.04\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantImage: "ubuntu:24.04",
		},
		{
			name:      "a build in the long form",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    build:\n      context: .\n      dockerfile: Dockerfile\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: &composeBuild{dockerfile: "Dockerfile"},
		},
		{
			name:      "a build in the short form defaults the Dockerfile",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    build: .\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: &composeBuild{dockerfile: "Dockerfile"},
		},
		{
			name:      "a build resolves against the Compose file's own directory",
			files:     map[string]string{"compose/docker-compose.yml": "services:\n  app:\n    build:\n      context: ..\n      dockerfile: build/Dockerfile\n"},
			paths:     []string{"compose/docker-compose.yml"},
			wantOK:    true,
			wantBuild: &composeBuild{dockerfile: "build/Dockerfile"},
		},
		{
			name:      "a build carries its target",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    build:\n      context: .\n      target: dev\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: &composeBuild{dockerfile: "Dockerfile", target: "dev"},
		},
		{
			name:      "an inline Dockerfile is its own content",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    build:\n      dockerfile_inline: |\n        FROM ubuntu:latest\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: &composeBuild{inline: "FROM ubuntu:latest\n"},
		},
		{
			name:      "a build overrides an image",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    image: built:latest\n    build: .\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: &composeBuild{dockerfile: "Dockerfile"},
		},
		{
			name: "a later file overriding the image wins",
			files: map[string]string{
				"a.yml": "services:\n  app:\n    image: ubuntu:latest\n",
				"b.yml": "services:\n  app:\n    image: ubuntu:24.04\n",
			},
			paths:     []string{"a.yml", "b.yml"},
			wantOK:    true,
			wantImage: "ubuntu:24.04",
		},
		{
			// Compose merges a build option by option across files, which this does not model.
			name: "a build declared by two files is not resolved",
			files: map[string]string{
				"a.yml": "services:\n  app:\n    build: .\n",
				"b.yml": "services:\n  app:\n    build:\n      target: dev\n",
			},
			paths:  []string{"a.yml", "b.yml"},
			wantOK: false,
		},
		{
			name:      "a context naming a repository is no path",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    build: https://example.invalid/repo.git\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: nil,
		},
		{
			name:      "a context written as a variable is not resolved",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    build: ${CONTEXT}\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: nil,
		},
		{
			name:      "a Dockerfile written as a variable is not resolved",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    build:\n      context: .\n      dockerfile: ${DOCKERFILE}\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: nil,
		},
		{
			// Compose writes a build as its context or as an object of options, and as neither of
			// those a build says nothing about a Dockerfile.
			name:      "a build that is neither form is not resolved",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    build:\n      - .\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantBuild: nil,
		},
		{
			name:      "an image written as a variable is not resolved",
			files:     map[string]string{"docker-compose.yml": "services:\n  app:\n    image: ubuntu:$TAG\n"},
			paths:     []string{"docker-compose.yml"},
			wantOK:    true,
			wantImage: "",
		},
		{
			name:   "a service none of the files defines",
			files:  map[string]string{"docker-compose.yml": "services:\n  web:\n    image: ubuntu:24.04\n"},
			paths:  []string{"docker-compose.yml"},
			wantOK: false,
		},
		{
			name:   "a service extending another",
			files:  map[string]string{"docker-compose.yml": "services:\n  app:\n    extends:\n      service: base\n"},
			paths:  []string{"docker-compose.yml"},
			wantOK: false,
		},
		{
			name:   "a file pulling in others",
			files:  map[string]string{"docker-compose.yml": "include:\n  - other.yml\nservices:\n  app:\n    image: ubuntu:24.04\n"},
			paths:  []string{"docker-compose.yml"},
			wantOK: false,
		},
		{
			name:   "a file that does not parse",
			files:  map[string]string{"docker-compose.yml": "services:\n  app:\n   image: [\n"},
			paths:  []string{"docker-compose.yml"},
			wantOK: false,
		},
		{
			name:   "a missing file",
			files:  map[string]string{},
			paths:  []string{"docker-compose.yml"},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fsys := fstest.MapFS{}
			for name, content := range tt.files {
				fsys[name] = &fstest.MapFile{Data: []byte(content)}
			}
			got, ok := composeServiceSource(linter.Dir{FS: fsys}, tt.paths, "app")
			if ok != tt.wantOK {
				t.Fatalf("composeServiceSource ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.image != tt.wantImage {
				t.Errorf("image = %q, want %q", got.image, tt.wantImage)
			}
			switch {
			case tt.wantBuild == nil && got.build != nil:
				t.Errorf("build = %+v, want none", *got.build)
			case tt.wantBuild != nil && got.build == nil:
				t.Errorf("build = none, want %+v", *tt.wantBuild)
			case tt.wantBuild != nil && *got.build != *tt.wantBuild:
				t.Errorf("build = %+v, want %+v", *got.build, *tt.wantBuild)
			}
		})
	}
}
