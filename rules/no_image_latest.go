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
	Category:    linter.CategoryReproducibility,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{"/image"},
	Check:       checkNoImageLatest,
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
