package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// NoComposeImageLatest reports the Compose service a devcontainer.json attaches to when it runs an
// image without an explicit tag or with the "latest" tag. It is [NoImageLatest] for the
// Compose-based form, where the container's image is named in a Compose file rather than in the
// "image" property.
var NoComposeImageLatest = &linter.Rule{
	ID:          "no-compose-image-latest",
	Description: `disallow a Compose service that runs an image without an explicit tag or with the "latest" tag`,
	LongDescription: `The service named by "service" is the dev container: it is the one editors attach to and lifecycle
scripts run in. Its "image:" is therefore the environment the project works in, and an entry with no tag,
or with "latest", pulls whatever the publisher last released — a container that changes from one
"docker compose up" to the next while the repository stays the same.`,
	References: []string{
		`https://containers.dev/implementors/spec/#docker-compose-based`,
		`https://containers.dev/implementors/json_reference/#compose-specific`,
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "name": "api",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}
`},
				{Path: `docker-compose.yml`, Content: `services:
  app:
    image: mcr.microsoft.com/devcontainers/base:latest
    command: sleep infinity
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "name": "api",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}
`},
				{Path: `docker-compose.yml`, Content: `services:
  app:
    image: mcr.microsoft.com/devcontainers/base:ubuntu-24.04
    command: sleep infinity
`},
			},
		},
		Note: "Only the service the dev container runs in is checked, and only when it runs a\n" +
			"published image; a service that builds its own image is covered by\n" +
			"[`no-dockerfile-image-latest`](../no-dockerfile-image-latest/), which reads the\n" +
			"Dockerfile its `build` names. An image written as a `${...}` variable is not checked,\n" +
			"its value not being in the configuration.",
	},
	Check: checkNoComposeImageLatest,
}

func checkNoComposeImageLatest(ctx *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	paths, offset, ok := composeFilePaths(obj)
	if !ok || len(paths) == 0 {
		return nil
	}
	service, ok := stringMember(obj, "service")
	if !ok {
		return nil
	}
	source, ok := composeServiceSource(ctx.Dir, paths, service)
	if !ok || source.image == "" {
		return nil
	}
	image := source.image

	tag, hasTag := refTag(image)
	switch {
	case !hasTag:
		return []linter.Finding{{
			Message: fmt.Sprintf("compose service %q runs image %q, which has no explicit tag; pin a specific version", service, image),
			Offset:  offset,
		}}
	case tag == "latest":
		return []linter.Finding{{
			Message: fmt.Sprintf("compose service %q runs image %q, which uses the \"latest\" tag; pin a specific version", service, image),
			Offset:  offset,
		}}
	}
	return nil
}
