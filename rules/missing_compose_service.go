package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingComposeService reports a devcontainer.json that sets "dockerComposeFile" without also
// setting "service", leaving the tool no way to know which compose service to attach to.
var MissingComposeService = &linter.Rule{
	ID:          "missing-compose-service",
	Description: `disallow a devcontainer.json that sets "dockerComposeFile" without "service"`,
	Category:    linter.CategoryCorrectness,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{""},
	Check:       checkMissingComposeService,
}

func checkMissingComposeService(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok || !hasMember(obj, "dockerComposeFile") || hasMember(obj, "service") {
		return nil
	}
	return []linter.Finding{{
		Message: `devcontainer.json sets "dockerComposeFile" but is missing "service"`,
		Offset:  node.Value.StartOffset,
	}}
}
