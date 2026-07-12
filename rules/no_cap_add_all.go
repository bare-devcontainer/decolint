package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoCapAddAll reports a devcontainer.json or devcontainer-feature.json that grants every Linux
// capability to the container, either via an "ALL" entry in the "capAdd" property or, in a
// devcontainer.json, a "--cap-add=ALL" entry in "runArgs". Granting all capabilities gives the
// container far more privilege than most workloads need, which is a significant security risk.
var NoCapAddAll = &linter.Rule{
	ID:          "no-cap-add-all",
	Description: `disallow granting all Linux capabilities via an "ALL" entry in the "capAdd" property, or a "--cap-add=ALL" entry in a devcontainer.json's "runArgs"`,
	FileTypes:   []linter.FileType{linter.Devcontainer, linter.Feature},
	Paths:       []string{"/capAdd/*", "/runArgs"},
	Check:       checkNoCapAddAll,
}

func checkNoCapAddAll(ctx *linter.Context, node *linter.Node) []linter.Finding {
	if node.Pointer == "/runArgs" {
		arr, ok := node.Value.Value.(*hujson.Array)
		if !ok || !runArgsApplicable(ctx) {
			return nil
		}
		v := runArgsFindFlagValue(arr, "--cap-add", func(s string) bool { return s == "ALL" })
		if v == nil {
			return nil
		}
		return []linter.Finding{{
			Message: `"runArgs" contains "--cap-add=ALL", granting every Linux capability to the container`,
			Offset:  v.StartOffset,
		}}
	}

	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' || lit.String() != "ALL" {
		return nil
	}
	return []linter.Finding{{
		Message: `"capAdd" contains "ALL", granting every Linux capability to the container`,
		Offset:  node.Value.StartOffset,
	}}
}
