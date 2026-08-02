package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// PinDockerfileImageDigest reports a FROM instruction of the Dockerfile a devcontainer.json builds
// from that names an image without a content digest. It is [PinImageDigest] for the
// Dockerfile-based form, and stands to [NoDockerfileImageLatest] as that rule stands to
// [NoImageLatest]: any unpinned reference is reported, not only a missing or "latest" tag.
var PinDockerfileImageDigest = &linter.Rule{
	ID:          "pin-dockerfile-image-digest",
	Description: `disallow a Dockerfile that builds from an image not pinned by content digest (e.g. "FROM image@sha256:...")`,
	LongDescription: `A "FROM" with a fixed tag still resolves through a mutable pointer: the publisher can move the tag to
different bits, so two builds of the same Dockerfile can start from different images. Writing the digest
("FROM image:tag@sha256:...") names the content itself, and the build verifies what it pulled against it.
Keeping the tag alongside the digest leaves the reference readable.`,
	References: []string{
		`https://containers.dev/implementors/spec/#dockerfile-based`,
		`https://github.com/opencontainers/image-spec/blob/main/descriptor.md#digests`,
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "name": "api",
  "build": {
    "dockerfile": "Dockerfile"
  }
}
`},
				{Path: `Dockerfile`, Content: `FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update && apt-get install -y --no-install-recommends jq
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "name": "api",
  "build": {
    "dockerfile": "Dockerfile"
  }
}
`},
				{Path: `Dockerfile`, Content: `FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04@sha256:2a1d1e1a4b0c3f8e5c8a1e0a6d3b7c9f4e2d1a0b9c8d7e6f5a4b3c2d1e0f9a8b

RUN apt-get update && apt-get install -y --no-install-recommends jq
`},
			},
		},
	},
	Check: checkPinDockerfileImageDigest,
}

func checkPinDockerfileImageDigest(ctx *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	images, path, offset, ok := dockerfileBuildImages(ctx.Dir, obj)
	if !ok {
		return nil
	}

	var findings []linter.Finding
	for _, image := range images {
		if digestSuffix.MatchString(image) {
			continue
		}
		findings = append(findings, linter.Finding{
			Message: fmt.Sprintf("Dockerfile %q builds from image %q, which is not pinned by digest; add an \"@sha256:...\" digest", path, image),
			Offset:  offset,
		})
	}
	return findings
}
