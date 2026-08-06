package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// PinDockerfileImageDigest reports an image the Dockerfile a devcontainer.json builds from pulls
// without a content digest. It is [PinImageDigest] for the Dockerfile-based form, and stands to
// [NoDockerfileImageLatest] as that rule stands to [NoImageLatest]: any unpinned reference is
// reported, not only a missing or "latest" tag.
var PinDockerfileImageDigest = &linter.Rule{
	ID:          "pin-dockerfile-image-digest",
	Description: `disallow a Dockerfile that pulls an image not pinned by content digest (e.g. "FROM image@sha256:...")`,
	LongDescription: `A "FROM" with a fixed tag still resolves through a mutable pointer: the publisher can move the tag to
different bits, so two builds of the same Dockerfile can start from different images. Writing the digest
("FROM image:tag@sha256:...") names the content itself, and the build verifies what it pulled against it.
Keeping the tag alongside the digest leaves the reference readable.

An image a "COPY --from" or a "RUN --mount=from" names is pulled through the same mutable pointer, so
it takes a digest too.

The Dockerfile is the one the configuration names, or, for a Compose-based configuration, the one the
service it runs is built by.`,
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
		Note: "The images a build of the Dockerfile pulls are checked: the base image of each stage the\n" +
			"build reaches, and the images its `COPY --from` and `RUN --mount=from` instructions name.\n" +
			"An image written with a `$` variable is not checked, since its value can come from\n" +
			"`build.args`. A Compose service that builds its own image is read the same way,\n" +
			"through the Dockerfile its `build` names.",
	},
	Check: checkPinDockerfileImageDigest,
}

func checkPinDockerfileImageDigest(ctx *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	images, subject, offset, ok := dockerfileBuildImages(ctx.Dir, obj)
	if !ok {
		return nil
	}

	var findings []linter.Finding
	for _, image := range images {
		if digestSuffix.MatchString(image.ref) {
			continue
		}
		findings = append(findings, linter.Finding{
			Message: fmt.Sprintf("%s %s image %q, which is not pinned by digest; add an \"@sha256:...\" digest", subject, image.verb(), image.ref),
			Offset:  offset,
		})
	}
	return findings
}
