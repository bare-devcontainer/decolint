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
			// The parser lower-cases every stage name, so a "FROM" reaches one only in lower case.
			"a stage name is reached in the case the parser gives it",
			"FROM golang:1.24 AS Builder\n\nFROM builder\n",
			nil,
		},
		{
			// BuildKit reads a base name it cannot match as an image, which is why "FROM BUILDER"
			// fails with "repository name must be lowercase" rather than building on the stage.
			"a base name in another case is an image",
			"FROM golang:1.24 AS builder\n\nFROM BUILDER\n",
			issue(`Dockerfile "Dockerfile" builds from image "BUILDER", which has no explicit tag; pin a specific version`),
		},
		{
			// A stage name cannot begin with a digit, so a "FROM" naming a position names an image;
			// the stage at that position is not built and its own base never pulled.
			"a base name written as a position is an image",
			"FROM golang:latest\n\nFROM 0\n",
			issue(`Dockerfile "Dockerfile" builds from image "0", which has no explicit tag; pin a specific version`),
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
			"FROM ubuntu:latest AS a\n\nFROM ubuntu:latest AS b\nCOPY --from=a /x /x\n",
			issue(`Dockerfile "Dockerfile" builds from image "ubuntu:latest", which uses the "latest" tag; pin a specific version`),
		},
		{"a Dockerfile whose instructions do not parse reports nothing", "FROM\n", nil},
		{"a Dockerfile that does not tokenize reports nothing", "FROM ubuntu\nRUN <<EOF\necho hi\n", nil},
		{
			// A "# check=" comment configures buildkit's own linter, which the parse must be handed
			// one of; the rule reads the stages as usual rather than failing on it.
			"a Dockerfile configuring buildkit's linter is read as usual",
			"FROM ubuntu:latest\n# check=skip=all\nRUN echo hi\n",
			issue(`Dockerfile "Dockerfile" builds from image "ubuntu:latest", which uses the "latest" tag; pin a specific version`),
		},
		{
			// BuildKit builds the last stage and what it depends on, so a stage nothing reaches is
			// never built and its base image never pulled.
			"a stage the build never reaches is not reported",
			"FROM ubuntu:latest AS unused\n\nFROM ubuntu:24.04\nRUN echo hi\n",
			nil,
		},
		{
			"a stage the last one copies from is reported",
			"FROM golang:latest AS builder\n\nFROM ubuntu:24.04\nCOPY --from=builder /app /app\n",
			issue(`Dockerfile "Dockerfile" builds from image "golang:latest", which uses the "latest" tag; pin a specific version`),
		},
		{
			"a stage the last one mounts from is reported",
			"FROM golang:latest AS builder\n\nFROM ubuntu:24.04\nRUN --mount=from=builder,target=/app echo hi\n",
			issue(`Dockerfile "Dockerfile" builds from image "golang:latest", which uses the "latest" tag; pin a specific version`),
		},
		{
			"a stage copied from by its position is reported",
			"FROM golang:latest\n\nFROM ubuntu:24.04\nCOPY --from=0 /app /app\n",
			issue(`Dockerfile "Dockerfile" builds from image "golang:latest", which uses the "latest" tag; pin a specific version`),
		},
		{
			"an image copied from is reported",
			"FROM ubuntu:24.04\nCOPY --from=ghcr.io/astral-sh/uv:latest /uv /bin/\n",
			issue(`Dockerfile "Dockerfile" pulls image "ghcr.io/astral-sh/uv:latest", which uses the "latest" tag; pin a specific version`),
		},
		{
			"an image mounted from is reported",
			"FROM ubuntu:24.04\nRUN --mount=from=busybox,target=/b /b/bin/echo hi\n",
			issue(`Dockerfile "Dockerfile" pulls image "busybox", which has no explicit tag; pin a specific version`),
		},
		{
			// A mount naming no source mounts the build context, and one naming no stage is matched
			// by name alone, so neither reaches the stage at that position.
			"a mount is not a position and needs no source",
			"FROM golang:latest\n\nFROM ubuntu:24.04\nRUN --mount=target=/b --mount=from=0,target=/c true\n",
			issue(`Dockerfile "Dockerfile" pulls image "0", which has no explicit tag; pin a specific version`),
		},
		{
			// Out of range, the position names no stage at all and the build fails on it, so there
			// is no image to report either.
			"a position no stage occupies is not an image",
			"FROM ubuntu:24.04\nCOPY --from=9 /app /app\n",
			nil,
		},
		{
			"an ordinary copy pulls nothing",
			"FROM ubuntu:24.04\nCOPY app /app\nRUN --mount=type=cache,target=/c true\n",
			nil,
		},
		{
			"a copy from a variable is not resolved",
			"FROM ubuntu:24.04\nCOPY --from=$BUILDER /app /app\n",
			nil,
		},
		{
			"a stage reached twice is read once",
			"FROM ubuntu:latest AS base\n\nFROM base AS mid\nCOPY --from=base /x /x\n",
			issue(`Dockerfile "Dockerfile" builds from image "ubuntu:latest", which uses the "latest" tag; pin a specific version`),
		},
		{"a Dockerfile with no stage at all reports nothing", "# nothing to build here\n", nil},
		{"a Dockerfile of global ARGs alone reports nothing", "ARG VERSION=1.0\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := linter.Dir{FS: fstest.MapFS{"Dockerfile": {Data: []byte(tt.dockerfile)}}}
			assertIssuesInDir(t, rules.NoDockerfileImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, src, dir, tt.want)
		})
	}
}

// TestNoDockerfileImageLatest_BuildTarget checks that only the stages a build of "build.target"
// reaches are read, since the others are never built.
func TestNoDockerfileImageLatest_BuildTarget(t *testing.T) {
	t.Parallel()

	const dockerfile = `FROM ubuntu:latest AS test
RUN echo test

FROM golang:latest AS tools

FROM ubuntu:24.04 AS dev
COPY --from=tools /go /go
`
	dir := linter.Dir{FS: fstest.MapFS{"Dockerfile": {Data: []byte(dockerfile)}}}
	// "build.dockerfile" comes first in each case below, so its value starts at column 26.
	toolsIssue := []linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "no-dockerfile-image-latest", Message: `Dockerfile "Dockerfile" builds from image "golang:latest", which uses the "latest" tag; pin a specific version`}}

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{
			"a stage outside the target is not reported",
			`{"build": {"dockerfile": "Dockerfile", "target": "dev"}}`,
			toolsIssue,
		},
		{
			"the target's own unpinned base is reported",
			`{"build": {"dockerfile": "Dockerfile", "target": "test"}}`,
			[]linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "no-dockerfile-image-latest", Message: `Dockerfile "Dockerfile" builds from image "ubuntu:latest", which uses the "latest" tag; pin a specific version`}},
		},
		{
			"no target builds the last stage and what it reaches",
			`{"build": {"dockerfile": "Dockerfile"}}`,
			toolsIssue,
		},
		{
			"a target is matched case-insensitively",
			`{"build": {"dockerfile": "Dockerfile", "target": "DEV"}}`,
			toolsIssue,
		},
		{"a target naming no stage reports nothing", `{"build": {"dockerfile": "Dockerfile", "target": "absent"}}`, nil},
		{
			// A target names a stage and never a position, and a stage name cannot begin with a
			// digit, so the build fails rather than building the stage at that position.
			"a target written as a position reports nothing",
			`{"build": {"dockerfile": "Dockerfile", "target": "0"}}`,
			nil,
		},
		{"a non-string target is no target", `{"build": {"dockerfile": "Dockerfile", "target": 42}}`, toolsIssue},
		{
			// The legacy top-level property names the Dockerfile; a "build" beside it that is not
			// an object names no target.
			"a non-object build alongside the legacy property is no target",
			`{"dockerFile": "Dockerfile", "build": "nonsense"}`,
			[]linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 16, RuleID: "no-dockerfile-image-latest", Message: `Dockerfile "Dockerfile" builds from image "golang:latest", which uses the "latest" tag; pin a specific version`}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesInDir(t, rules.NoDockerfileImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, tt.src, dir, tt.want)
		})
	}
}

// TestNoDockerfileImageLatest_ForwardStageReference checks that a stage copied from before it is
// declared is read as the stage it names rather than as an image: BuildKit resolves a "--from"
// once the whole Dockerfile is parsed, so the order the two stages are written in does not matter.
func TestNoDockerfileImageLatest_ForwardStageReference(t *testing.T) {
	t.Parallel()

	const dockerfile = `FROM ubuntu:24.04 AS dev
COPY --from=tools /go /go

FROM golang:latest AS tools
`
	dir := linter.Dir{FS: fstest.MapFS{"Dockerfile": {Data: []byte(dockerfile)}}}
	src := `{"build": {"dockerfile": "Dockerfile", "target": "dev"}}`
	want := []linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "no-dockerfile-image-latest", Message: `Dockerfile "Dockerfile" builds from image "golang:latest", which uses the "latest" tag; pin a specific version`}}
	assertIssuesInDir(t, rules.NoDockerfileImageLatest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, src, dir, want)
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
