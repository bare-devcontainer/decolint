package rules

import "github.com/bare-devcontainer/decolint/linter"

// NoAppPort reports the legacy "appPort" property. It only supports statically publishing ports at
// container-creation time; "forwardPorts" is the modern replacement and forwards ports dynamically
// without requiring the container to be recreated.
var NoAppPort = &linter.Rule{
	ID:          "no-app-port",
	Description: `disallow the legacy "appPort" property in favor of "forwardPorts"`,
	LongDescription: `"appPort" publishes the port the way Docker does: it is fixed when the container is created, and the
application has to listen on all interfaces rather than just "localhost" to be reachable. A forwarded
port instead looks like "localhost" to the application and can be changed without recreating the
container, which is why the reference recommends "forwardPorts" in most cases.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#image-specific`,
		`https://containers.dev/implementors/json_reference/#publishing-vs-forwarding-ports`,
	},
	Category:  linter.CategoryStyle,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{"/appPort"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "appPort": [3000]
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "forwardPorts": [3000]
}
`},
			},
		},
	},
	Check: checkNoAppPort,
}

func checkNoAppPort(_ *linter.Context, node *linter.Node) []linter.Finding {
	return []linter.Finding{{
		Message: `"appPort" is a legacy property; use "forwardPorts" instead to forward ports dynamically`,
		Offset:  node.Value.StartOffset,
	}}
}
