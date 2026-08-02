package rules

import (
	"fmt"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
	"go.yaml.in/yaml/v3"
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
		Note: "Only the service the dev container runs in is checked. A service that builds its own\n" +
			"image is left to the Dockerfile rules, and a service whose image is written as a\n" +
			"`${...}` variable is not reported: the value is not in the configuration.",
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
	image, ok := composeServiceImage(ctx.Dir, paths, service)
	if !ok {
		return nil
	}

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

// composeFilePaths returns the Compose file paths obj declares, with the byte offset of the value
// declaring them. The property is a single path or an array of paths, later ones overriding earlier
// ones; the merge reads the same property in feature's composeFilePaths.
func composeFilePaths(obj *hujson.Object) (paths []string, offset int, ok bool) {
	m := memberNamed(obj, "dockerComposeFile")
	if m == nil {
		return nil, 0, false
	}
	switch v := m.Value.Value.(type) {
	case hujson.Literal:
		if v.Kind() != '"' {
			return nil, 0, false
		}
		paths = []string{v.String()}
	case *hujson.Array:
		for _, e := range v.Elements {
			lit, isLit := e.Value.(hujson.Literal)
			if !isLit || lit.Kind() != '"' {
				return nil, 0, false
			}
			paths = append(paths, lit.String())
		}
	default:
		return nil, 0, false
	}
	return paths, m.Value.StartOffset, true
}

// composeService is the part of a Compose service definition that says which image the service
// runs.
type composeService struct {
	Image string `yaml:"image"`
	Build any    `yaml:"build"`
}

// composeServiceImage returns the image the named Compose service runs, reading the files at paths
// in the order they are declared, each later one overriding the earlier ones as Compose merges them.
//
// ok is false whenever the answer is not in the files themselves, so that the caller reports
// nothing rather than reporting on a service it has only partly resolved:
//   - a file that cannot be read (see [readConfigFile]) or does not parse;
//   - a service none of the files defines, which "extends" or "include" may bring in from a file
//     decolint does not follow;
//   - a service that declares "build", whose "image" names what the build produces rather than what
//     it starts from;
//   - an image written with a "${...}" variable, whose value comes from the environment.
func composeServiceImage(dir linter.Dir, paths []string, service string) (string, bool) {
	var image string
	var found bool
	for _, p := range paths {
		src, ok := readConfigFile(dir, p)
		if !ok {
			return "", false
		}
		var doc struct {
			Services map[string]composeService `yaml:"services"`
		}
		if err := yaml.Unmarshal(src, &doc); err != nil {
			return "", false
		}
		svc, ok := doc.Services[service]
		if !ok {
			continue
		}
		found = true
		if svc.Build != nil {
			return "", false
		}
		if svc.Image != "" {
			image = svc.Image
		}
	}
	if !found || image == "" || strings.Contains(image, "${") {
		return "", false
	}
	return image, true
}
