package rules_test

import (
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestPinDockerfileImageDigest(t *testing.T) {
	t.Parallel()

	const src = `{"build": {"dockerfile": "Dockerfile"}}`
	issue := func(message string) []linter.Issue {
		return []linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "pin-dockerfile-image-digest", Message: message}}
	}

	tests := []struct {
		name       string
		dockerfile string
		want       []linter.Issue
	}{
		{
			"a fixed tag is not a digest",
			"FROM ubuntu:24.04\n",
			issue(`Dockerfile "Dockerfile" builds from image "ubuntu:24.04", which is not pinned by digest; add an "@sha256:..." digest`),
		},
		{
			"an untagged image is reported too",
			"FROM ubuntu\n",
			issue(`Dockerfile "Dockerfile" builds from image "ubuntu", which is not pinned by digest; add an "@sha256:..." digest`),
		},
		{"a digest alone is pinned", "FROM ubuntu@sha256:abc123\n", nil},
		{"a tag alongside a digest is pinned", "FROM ubuntu:24.04@sha256:abc123\n", nil},
		{"scratch is not an image", "FROM scratch\n", nil},
		{
			"a later stage building on an earlier one is not an image",
			"FROM ubuntu:24.04@sha256:abc123 AS builder\n\nFROM builder\n",
			nil,
		},
		{"an image reached through a variable is not resolved", "FROM ubuntu:${VARIANT}\n", nil},
		{
			"each unpinned stage is reported",
			"FROM golang:1.24 AS builder\n\nFROM ubuntu:24.04\nCOPY --from=builder /app /app\n",
			[]linter.Issue{
				{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "pin-dockerfile-image-digest", Message: `Dockerfile "Dockerfile" builds from image "golang:1.24", which is not pinned by digest; add an "@sha256:..." digest`},
				{Path: "devcontainer.json", Line: 1, Col: 26, RuleID: "pin-dockerfile-image-digest", Message: `Dockerfile "Dockerfile" builds from image "ubuntu:24.04", which is not pinned by digest; add an "@sha256:..." digest`},
			},
		},
		{
			"an image copied from is reported",
			"FROM ubuntu:24.04@sha256:abc123\nCOPY --from=ghcr.io/astral-sh/uv:0.9.7 /uv /bin/\n",
			issue(`Dockerfile "Dockerfile" pulls image "ghcr.io/astral-sh/uv:0.9.7", which is not pinned by digest; add an "@sha256:..." digest`),
		},
		{
			"an image mounted from is reported",
			"FROM ubuntu:24.04@sha256:abc123\nRUN --mount=from=busybox:1.37,target=/b /b/bin/echo hi\n",
			issue(`Dockerfile "Dockerfile" pulls image "busybox:1.37", which is not pinned by digest; add an "@sha256:..." digest`),
		},
		{
			"an image copied from by digest is pinned",
			"FROM ubuntu:24.04@sha256:abc123\nCOPY --from=busybox@sha256:def456 /bin/busybox /bin/\n",
			nil,
		},
		{"a Dockerfile that does not parse reports nothing", "FROM\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := linter.Dir{FS: fstest.MapFS{"Dockerfile": {Data: []byte(tt.dockerfile)}}}
			assertIssuesInDir(t, rules.PinDockerfileImageDigest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, src, dir, tt.want)
		})
	}

	t.Run("no Dockerfile declared", func(t *testing.T) {
		t.Parallel()
		dir := linter.Dir{FS: fstest.MapFS{"Dockerfile": {Data: []byte("FROM ubuntu:24.04\n")}}}
		assertIssuesInDir(t, rules.PinDockerfileImageDigest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, `{"image": "ubuntu:24.04"}`, dir, nil)
	})

	t.Run("a missing Dockerfile reports nothing", func(t *testing.T) {
		t.Parallel()
		assertIssuesInDir(t, rules.PinDockerfileImageDigest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, src, linter.Dir{FS: fstest.MapFS{}}, nil)
	})

	t.Run("a document that is not an object reports nothing", func(t *testing.T) {
		t.Parallel()
		dir := linter.Dir{FS: fstest.MapFS{"Dockerfile": {Data: []byte("FROM ubuntu\n")}}}
		assertIssuesInDir(t, rules.PinDockerfileImageDigest, linter.SeverityError, "devcontainer.json", linter.Devcontainer, `["Dockerfile"]`, dir, nil)
	})
}
