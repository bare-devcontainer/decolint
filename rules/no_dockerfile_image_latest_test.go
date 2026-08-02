package rules_test

import (
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoDockerfileImageLatest(t *testing.T) {
	t.Parallel()

	// Every case declares the Dockerfile at "build.dockerfile", whose value starts at column 26, so
	// the findings all anchor there.
	const src = `{"build": {"dockerfile": "Dockerfile"}}`
	issue := func(message string) []linter.Issue {
		return []linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "no-dockerfile-image-latest", Message: message}}
	}

	tests := []struct {
		name       string
		dockerfile string
		want       []linter.Issue
	}{
		{
			"untagged base image",
			"FROM ubuntu\n",
			issue(`Dockerfile "Dockerfile" builds from image "ubuntu", which has no explicit tag; pin a specific version`),
		},
		{
			"latest base image",
			"FROM ubuntu:latest\n",
			issue(`Dockerfile "Dockerfile" builds from image "ubuntu:latest", which uses the "latest" tag; pin a specific version`),
		},
		{"pinned tag", "FROM ubuntu:24.04\n", nil},
		{"pinned digest", "FROM ubuntu@sha256:abc123\n", nil},
		{"scratch is not an image", "FROM scratch\nCOPY app /app\n", nil},
		{
			"a later stage building on an earlier one is not an image",
			"FROM golang:1.24 AS builder\nRUN go build\n\nFROM builder AS final\n",
			nil,
		},
		{
			"a stage name is matched case-insensitively",
			"FROM golang:1.24 AS Builder\n\nFROM builder\n",
			nil,
		},
		{
			"an image reached through a variable is not resolved",
			"ARG VARIANT=24.04\nFROM ubuntu:${VARIANT}\n",
			nil,
		},
		{
			"each unpinned stage is reported",
			"FROM golang:latest AS builder\nRUN go build\n\nFROM ubuntu\nCOPY --from=builder /app /app\n",
			[]linter.Issue{
				{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "no-dockerfile-image-latest", Message: `Dockerfile "Dockerfile" builds from image "golang:latest", which uses the "latest" tag; pin a specific version`},
				{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "no-dockerfile-image-latest", Message: `Dockerfile "Dockerfile" builds from image "ubuntu", which has no explicit tag; pin a specific version`},
			},
		},
		{
			"the same unpinned image in several stages is reported once",
			"FROM ubuntu:latest AS a\n\nFROM ubuntu:latest AS b\n",
			issue(`Dockerfile "Dockerfile" builds from image "ubuntu:latest", which uses the "latest" tag; pin a specific version`),
		},
		{"a Dockerfile whose instructions do not parse reports nothing", "FROM\n", nil},
		{"a Dockerfile that does not tokenize reports nothing", "FROM ubuntu\nRUN <<EOF\necho hi\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := linter.Dir{FS: fstest.MapFS{"Dockerfile": {Data: []byte(tt.dockerfile)}}}
			assertIssuesInDir(t, rules.NoDockerfileImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, src, dir, tt.want)
		})
	}
}

func TestNoDockerfileImageLatest_DockerfileLocation(t *testing.T) {
	t.Parallel()

	dir := linter.Dir{FS: fstest.MapFS{
		"Dockerfile":       {Data: []byte("FROM ubuntu:24.04\n")},
		"build/Dockerfile": {Data: []byte("FROM ubuntu:latest\n")},
	}}

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no Dockerfile declared", `{"image": "ubuntu:latest"}`, nil},
		{
			"the legacy top-level property is read",
			`{"dockerFile": "build/Dockerfile"}`,
			[]linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 16, RuleID: "no-dockerfile-image-latest", Message: `Dockerfile "build/Dockerfile" builds from image "ubuntu:latest", which uses the "latest" tag; pin a specific version`}},
		},
		{
			// The reference implementation prefers the top-level property, so the pinned Dockerfile
			// it names is the one built and the "build.dockerfile" beside it is never read.
			"the top-level property wins over build.dockerfile",
			`{"dockerFile": "Dockerfile", "build": {"dockerfile": "build/Dockerfile"}}`,
			nil,
		},
		{"a subdirectory path is read", `{"build": {"dockerfile": "build/Dockerfile"}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "no-dockerfile-image-latest", Message: `Dockerfile "build/Dockerfile" builds from image "ubuntu:latest", which uses the "latest" tag; pin a specific version`},
		}},
		{"a missing Dockerfile reports nothing", `{"build": {"dockerfile": "absent/Dockerfile"}}`, nil},
		{
			// Configuration under .devcontainer is read through a root confined to it, so a
			// Dockerfile above that directory is not decolint's to open.
			"a path leading outside the directory reports nothing",
			`{"build": {"dockerfile": "../Dockerfile"}}`,
			nil,
		},
		{"an absolute path reports nothing", `{"build": {"dockerfile": "/Dockerfile"}}`, nil},
		{"a path naming a directory reports nothing", `{"build": {"dockerfile": "build"}}`, nil},
		{"a non-string dockerfile reports nothing", `{"build": {"dockerfile": 42}}`, nil},
		{"a non-object build reports nothing", `{"build": "Dockerfile"}`, nil},
		{"a document that is not an object reports nothing", `["Dockerfile"]`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesInDir(t, rules.NoDockerfileImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, tt.src, dir, tt.want)
		})
	}

	t.Run("unreadable directory reports nothing", func(t *testing.T) {
		t.Parallel()
		src := `{"build": {"dockerfile": "Dockerfile"}}`
		assertIssuesInDir(t, rules.NoDockerfileImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, src, linter.Dir{FS: errFS{}}, nil)
	})

	t.Run("nil directory reports nothing", func(t *testing.T) {
		t.Parallel()
		assertIssues(t, rules.NoDockerfileImageLatest, linter.SeverityError, `{"build": {"dockerfile": "Dockerfile"}}`, nil)
	})
}
