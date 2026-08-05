package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoDockerfileImageLatest reports an image the Dockerfile a devcontainer.json builds from pulls
// without an explicit tag or with the "latest" tag. It is [NoImageLatest] for the Dockerfile-based
// form, where the images are named in the Dockerfile rather than in the "image" property.
var NoDockerfileImageLatest = &linter.Rule{
	ID:          "no-dockerfile-image-latest",
	Description: `disallow a Dockerfile that pulls an image without an explicit tag or with the "latest" tag`,
	LongDescription: `A configuration that builds from a Dockerfile still starts from a base image, and pinning the
devcontainer.json says nothing about what that image is: a "FROM" with no tag, or with "latest", resolves
to whatever the publisher last released. The container then changes from one rebuild to the next while
every file in the repository stays the same. Name the version in the "FROM" the way you would in "image".

A "COPY --from" or a "RUN --mount=from" naming an image pulls one just as a "FROM" does, and what it
brings into the container moves under an unpinned reference the same way, so those are named too.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#image-specific`,
		`https://containers.dev/implementors/spec/#dockerfile-based`,
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
				{Path: `Dockerfile`, Content: `FROM mcr.microsoft.com/devcontainers/base:latest

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
				{Path: `Dockerfile`, Content: `FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update && apt-get install -y --no-install-recommends jq
`},
			},
		},
		Note: "The images a build of the Dockerfile pulls are checked: the base image of each stage the\n" +
			"build reaches, and the images its `COPY --from` and `RUN --mount=from` instructions name.\n" +
			"An image written with a `$` variable is not checked, since its value can come from\n" +
			"`build.args`.\n" +
			"The finding is reported at the property naming the Dockerfile, since that is what the\n" +
			"devcontainer.json says about the image; the fix belongs in the Dockerfile.",
	},
	Check: checkNoDockerfileImageLatest,
}

func checkNoDockerfileImageLatest(ctx *linter.Context, node *linter.Node) []linter.Finding {
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
		tag, hasTag := refTag(image.ref)
		switch {
		case !hasTag:
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf("Dockerfile %q %s image %q, which has no explicit tag; pin a specific version", path, image.verb(), image.ref),
				Offset:  offset,
			})
		case tag == "latest":
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf("Dockerfile %q %s image %q, which uses the \"latest\" tag; pin a specific version", path, image.verb(), image.ref),
				Offset:  offset,
			})
		}
	}
	return findings
}
