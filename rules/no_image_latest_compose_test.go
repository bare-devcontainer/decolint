package rules_test

import (
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoImageLatest_Compose(t *testing.T) {
	t.Parallel()

	// Every case declares one Compose file, whose path starts at column 23, so the findings all
	// anchor there.
	const src = `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
	issue := func(message string) []linter.Issue {
		return []linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 23, RuleID: "no-image-latest", Message: message}}
	}

	tests := []struct {
		name    string
		compose string
		want    []linter.Issue
	}{
		{
			"untagged image",
			"services:\n  app:\n    image: ubuntu\n",
			issue(`compose service "app": image "ubuntu" has no explicit tag; pin a specific version`),
		},
		{
			"latest image",
			"services:\n  app:\n    image: ubuntu:latest\n",
			issue(`compose service "app": image "ubuntu:latest" uses the "latest" tag; pin a specific version`),
		},
		{"pinned tag", "services:\n  app:\n    image: ubuntu:24.04\n", nil},
		{"pinned digest", "services:\n  app:\n    image: ubuntu@sha256:abc123\n", nil},
		{
			// Only the service the dev container runs in is the container's image.
			"another service is not the dev container",
			"services:\n  app:\n    image: ubuntu:24.04\n  db:\n    image: postgres:latest\n",
			nil,
		},
		{
			// A service that builds names in "image" what the build produces, not what it starts
			// from; the Dockerfile rules cover the base image.
			"a service that builds its own image reports nothing",
			"services:\n  app:\n    build: .\n    image: myapp:latest\n",
			nil,
		},
		{
			"an image written as a variable is not resolved",
			"services:\n  app:\n    image: ubuntu:${TAG}\n",
			nil,
		},
		{
			// Compose accepts the bare form as readily as "${VAR}".
			"an image written as a bare variable is not resolved",
			"services:\n  app:\n    image: $IMAGE\n",
			nil,
		},
		{
			// The definition continues in a file this does not read, so what is here may not be the
			// image the service ends up running.
			"a service extending another reports nothing",
			"services:\n  app:\n    extends:\n      file: base.yml\n      service: base\n    image: ubuntu:latest\n",
			nil,
		},
		{
			"a file pulling in others reports nothing",
			"include:\n  - other.yml\nservices:\n  app:\n    image: ubuntu:latest\n",
			nil,
		},
		{"a service defined in no file reports nothing", "services:\n  web:\n    image: ubuntu:latest\n", nil},
		{"a service without an image reports nothing", "services:\n  app:\n    command: sleep infinity\n", nil},
		{"a file that does not parse reports nothing", "services:\n  app:\n   image: [\n", nil},
		{"an empty file reports nothing", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := linter.Dir{FS: fstest.MapFS{"docker-compose.yml": {Data: []byte(tt.compose)}}}
			assertIssuesInDir(t, rules.NoImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, src, dir, tt.want)
		})
	}
}

func TestNoImageLatest_Compose_ComposeFileList(t *testing.T) {
	t.Parallel()

	dir := linter.Dir{FS: fstest.MapFS{
		"docker-compose.yml":          {Data: []byte("services:\n  app:\n    image: ubuntu:latest\n")},
		"docker-compose.override.yml": {Data: []byte("services:\n  app:\n    image: ubuntu:24.04\n")},
		"command.yml":                 {Data: []byte("services:\n  app:\n    command: sleep infinity\n")},
	}}

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{
			// Compose applies the files in order, so the last one to name an image wins.
			"a later file overriding the image is the one read",
			`{"dockerComposeFile": ["docker-compose.yml", "docker-compose.override.yml"], "service": "app"}`,
			nil,
		},
		{
			"a later file leaving the image alone does not clear it",
			`{"dockerComposeFile": ["docker-compose.yml", "command.yml"], "service": "app"}`,
			[]linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 23, RuleID: "no-image-latest", Message: `compose service "app": image "ubuntu:latest" uses the "latest" tag; pin a specific version`}},
		},
		{
			"an earlier file overridden by a later one is not reported",
			`{"dockerComposeFile": ["docker-compose.override.yml", "docker-compose.yml"], "service": "app"}`,
			[]linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 23, RuleID: "no-image-latest", Message: `compose service "app": image "ubuntu:latest" uses the "latest" tag; pin a specific version`}},
		},
		{
			"no dockerComposeFile property",
			`{"image": "ubuntu:latest", "service": "app"}`,
			[]linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "no-image-latest", Message: `image "ubuntu:latest" uses the "latest" tag; pin a specific version`}},
		},
		{"no service property", `{"dockerComposeFile": "docker-compose.yml"}`, nil},
		{"an empty file list reports nothing", `{"dockerComposeFile": [], "service": "app"}`, nil},
		{"a non-string entry reports nothing", `{"dockerComposeFile": [42], "service": "app"}`, nil},
		{"a non-string dockerComposeFile reports nothing", `{"dockerComposeFile": 42, "service": "app"}`, nil},
		{"an object dockerComposeFile reports nothing", `{"dockerComposeFile": {}, "service": "app"}`, nil},
		{"a non-string service reports nothing", `{"dockerComposeFile": "docker-compose.yml", "service": 42}`, nil},
		{"a document that is not an object reports nothing", `["docker-compose.yml"]`, nil},
		{"a missing Compose file reports nothing", `{"dockerComposeFile": "absent.yml", "service": "app"}`, nil},
		{
			// Configuration under .devcontainer is read through a root confined to it, so a Compose
			// file above that directory is not decolint's to open.
			"a path leading outside the directory reports nothing",
			`{"dockerComposeFile": "../docker-compose.yml", "service": "app"}`,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesInDir(t, rules.NoImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, tt.src, dir, tt.want)
		})
	}

	t.Run("unreadable directory reports nothing", func(t *testing.T) {
		t.Parallel()
		src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
		assertIssuesInDir(t, rules.NoImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, src, linter.Dir{FS: errFS{}}, nil)
	})

	t.Run("nil directory reports nothing", func(t *testing.T) {
		t.Parallel()
		assertIssues(t, rules.NoImageLatest, linter.SeverityError, `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`, nil)
	})
}
