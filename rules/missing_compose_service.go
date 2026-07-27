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
	LongDescription: `A Compose project usually defines several services, so naming the Compose file does not say which
container the tooling should attach to. The specification requires "service" to name that main container:
it is the one lifecycle scripts run in and the one editors connect to.`,
	References: []string{
		`https://containers.dev/implementors/spec/#docker-compose-based`,
		`https://containers.dev/implementors/json_reference/#compose-specific`,
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "name": "my project",
  "dockerComposeFile": "docker-compose.yml",
  "workspaceFolder": "/workspace"
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "name": "my project",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}
`},
			},
		},
	},
	Check: checkMissingComposeService,
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
