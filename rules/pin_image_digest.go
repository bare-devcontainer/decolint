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

// PinImageDigest reports the "image" property when it references a container image without a
// content digest (e.g. "ubuntu@sha256:..."). Unlike [NoImageLatest], which only flags a missing or
// "latest" tag, this rule flags any reference that isn't pinned by digest, since even a fixed tag
// can later be reassigned to point at a different image. It is off by default because
// digest-pinning every image is a heavier requirement than most projects want.
var PinImageDigest = &linter.Rule{
	ID:          "pin-image-digest",
	Description: `disallow an "image" property that does not pin the image by content digest (e.g. "image@sha256:...")`,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{"/image"},
	Check:       checkPinImageDigest,
}

func checkPinImageDigest(_ *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	image := lit.String()
	if digestSuffix.MatchString(image) {
		return nil
	}
	return []linter.Finding{{
		Message: fmt.Sprintf("image %q is not pinned by digest; add an \"@sha256:...\" digest", image),
		Offset:  node.Value.StartOffset,
	}}
}
