package rules

import (
	"fmt"
	"regexp"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// digestSuffix matches a trailing "@algorithm:encoded" content digest, per the OCI image reference
// grammar (e.g. "@sha256:<hex>"). It is anchored to the end of the string since a digest is always
// the last component of an image reference.
var digestSuffix = regexp.MustCompile(`@[a-z0-9]+(?:[+._-][a-z0-9]+)*:[a-zA-Z0-9=_-]+$`)

// PinImageDigest reports an image the configuration pulls without a content digest (e.g.
// "ubuntu@sha256:..."), wherever it is named (see [configImages]). Unlike [NoImageLatest], which
// only flags a missing or "latest" tag, this rule flags any reference that isn't pinned by digest,
// since even a fixed tag can later be reassigned to point at a different image.
var PinImageDigest = &linter.Rule{
	ID:          "pin-image-digest",
	Description: `disallow an "image" property that does not pin the image by content digest (e.g. "image@sha256:...")`,
	LongDescription: `A tag is a mutable pointer: the publisher can move even a fully specified one to different bits, and a
registry can serve a different image for the same tag on a different day. A digest names the content
itself, so "image@sha256:..." always resolves to the exact image the project was tested with, and the
client verifies what it pulled against it.

Every image a container of this configuration pulls is checked, whichever way the configuration names
it: the "image" property, the "FROM" and "COPY --from" of the Dockerfile it builds from, and, for a
Compose-based configuration, the image its service runs or the Dockerfile that service builds from.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#image-specific`,
		`https://github.com/opencontainers/image-spec/blob/main/descriptor.md#digests`,
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04"
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04@sha256:2a1d1e1a4b0c3f8e5c8a1e0a6d3b7c9f4e2d1a0b9c8d7e6f5a4b3c2d1e0f9a8b"
}
`},
			},
		},
		Note: "An image written with a `$` or `${...}` variable is not checked: its value comes from\n" +
			"the environment or from `build.args`, not from the configuration.",
	},
	Check: checkPinImageDigest,
}

func checkPinImageDigest(ctx *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	var findings []linter.Finding
	for _, image := range configImages(ctx.Dir, obj) {
		if digestSuffix.MatchString(image.ref) {
			continue
		}
		findings = append(findings, linter.Finding{
			Message: fmt.Sprintf("%simage %q is not pinned by digest; add an \"@sha256:...\" digest", image.source, image.ref),
			Offset:  image.offset,
		})
	}
	return findings
}
