package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// RequireCapDropAll reports a devcontainer.json that does not drop all Linux capabilities via a
// "--cap-drop=ALL" entry in "runArgs". Dropping every capability and adding back only what's needed
// (e.g. via "capAdd") follows the principle of least privilege. It is off by default because most
// configs don't set it and enabling it by default would be noisy.
var RequireCapDropAll = &linter.Rule{
	ID:          "require-cap-drop-all",
	Description: `require a "--cap-drop=ALL" entry in a devcontainer.json's "runArgs", dropping every Linux capability`,
	Category:    linter.CategorySecurity,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{""},
	Check:       checkRequireCapDropAll,
}

func checkRequireCapDropAll(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	if arr, ok := arrayMember(obj, "runArgs"); ok {
		if runArgsFindFlagValue(arr, "--cap-drop", func(s string) bool { return s == "ALL" }) != nil {
			return nil
		}
	}

	return []linter.Finding{{
		Message: `"ALL" is not set via "runArgs", leaving the container with its default Linux capabilities`,
		Offset:  node.Value.StartOffset,
	}}
}
