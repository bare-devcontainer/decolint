package rules

import "github.com/bare-devcontainer/decolint/linter"

// NoAppPort reports the legacy "appPort" property. It only supports statically publishing ports at
// container-creation time; "forwardPorts" is the modern replacement and forwards ports dynamically
// without requiring the container to be recreated.
var NoAppPort = &linter.Rule{
	ID:          "no-app-port",
	Description: `disallow the legacy "appPort" property in favor of "forwardPorts"`,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{"/appPort"},
	Check:       checkNoAppPort,
}

func checkNoAppPort(_ *linter.Context, node *linter.Node) []linter.Finding {
	return []linter.Finding{{
		Message: `"appPort" is a legacy property; use "forwardPorts" instead to forward ports dynamically`,
		Offset:  node.Value.StartOffset,
	}}
}
