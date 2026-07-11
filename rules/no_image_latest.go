package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoImageLatest reports the "image" property when it references a container image without an
// explicit tag or with the "latest" tag. Such references are not reproducible: the image they
// resolve to changes over time.
type NoImageLatest struct{}

// ID implements [linter.Rule].
func (NoImageLatest) ID() string { return "no-image-latest" }

// Description implements [linter.Rule].
func (NoImageLatest) Description() string {
	return `disallow container images without an explicit tag or with the "latest" tag`
}

// FileTypes implements [linter.Rule].
func (NoImageLatest) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (NoImageLatest) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (NoImageLatest) Paths() []string { return []string{"/image"} }

// Check implements [linter.Rule].
func (r NoImageLatest) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	image := lit.String()

	tag, hasTag := refTag(image)
	switch {
	case !hasTag:
		return []linter.Finding{{
			RuleID:  r.ID(),
			Message: fmt.Sprintf("image %q has no explicit tag; pin a specific version", image),
			Offset:  node.Value.StartOffset,
		}}
	case tag == "latest":
		return []linter.Finding{{
			RuleID:  r.ID(),
			Message: fmt.Sprintf("image %q uses the \"latest\" tag; pin a specific version", image),
			Offset:  node.Value.StartOffset,
		}}
	}
	return nil
}
