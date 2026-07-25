package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoImageLatest reports the "image" property when it references a container image without an
// explicit tag or with the "latest" tag. Such references are not reproducible: the image they
// resolve to changes over time.
var NoImageLatest = &linter.Rule{
	ID:          "no-image-latest",
	Description: `disallow container images without an explicit tag or with the "latest" tag`,
	LongDescription: `A reference with no tag resolves to "latest", and "latest" is just the tag a publisher moves as they
release. Either way the configuration says "whatever is current", so the same devcontainer.json builds a
different environment next month, and a build that broke cannot be reproduced from the file alone. Name
the version the project was tested against.`,
	References: []string{
		"https://containers.dev/implementors/json_reference/#image-or-dockerfile-specific-properties",
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{"/image"},
	Check:     checkNoImageLatest,
}

func checkNoImageLatest(_ *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	image := lit.String()

	tag, hasTag := refTag(image)
	switch {
	case !hasTag:
		return []linter.Finding{{
			Message: fmt.Sprintf("image %q has no explicit tag; pin a specific version", image),
			Offset:  node.Value.StartOffset,
		}}
	case tag == "latest":
		return []linter.Finding{{
			Message: fmt.Sprintf("image %q uses the \"latest\" tag; pin a specific version", image),
			Offset:  node.Value.StartOffset,
		}}
	}
	return nil
}
