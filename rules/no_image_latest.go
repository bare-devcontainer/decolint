package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoImageLatest reports an image the configuration pulls without an explicit tag or with the
// "latest" tag, wherever it is named (see [configImages]). Such references are not reproducible: the
// image they resolve to changes over time.
var NoImageLatest = &linter.Rule{
	ID:          "no-image-latest",
	Description: `disallow container images without an explicit tag or with the "latest" tag`,
	LongDescription: `A reference with no tag resolves to "latest", and "latest" is just the tag a publisher moves as they
release. Either way the configuration says "whatever is current", so the same devcontainer.json builds a
different environment next month, and a build that broke cannot be reproduced from the file alone. Name
the version the project was tested against.

Every image a container of this configuration pulls is checked, whichever way the configuration names
it: the "image" property, the "FROM" and "COPY --from" of the Dockerfile it builds from, and, for a
Compose-based configuration, the image its service runs or the Dockerfile that service builds from.`,
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
  "image": "mcr.microsoft.com/devcontainers/base:latest"
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04"
}
`},
			},
		},
		Note: "An image written with a `$` or `${...}` variable is not checked: its value comes from\n" +
			"the environment or from `build.args`, not from the configuration.",
	},
	Check: checkNoImageLatest,
}

func checkNoImageLatest(ctx *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	var findings []linter.Finding
	for _, image := range configImages(ctx.Dir, obj) {
		tag, hasTag := refTag(image.ref)
		switch {
		case !hasTag:
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf("%simage %q has no explicit tag; pin a specific version", image.source, image.ref),
				Offset:  image.offset,
			})
		case tag == "latest":
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf("%simage %q uses the \"latest\" tag; pin a specific version", image.source, image.ref),
				Offset:  image.offset,
			})
		}
	}
	return findings
}
