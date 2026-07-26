package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingWorkspaceMountFolder reports a devcontainer.json that uses "image" or "build" and sets
// only one of "workspaceMount" or "workspaceFolder", leaving the tool unable to tell where the
// overridden mount lands inside the container.
var MissingWorkspaceMountFolder = &linter.Rule{
	ID:          "missing-workspace-mount-folder",
	Description: `disallow a devcontainer.json using "image" or "build" that sets only one of "workspaceMount" or "workspaceFolder"`,
	LongDescription: `The two properties describe opposite ends of the same override: "workspaceMount" says where the source
code is mounted, "workspaceFolder" says which path inside the container the tooling opens. The reference
documents each as requiring the other, because setting one alone either mounts the source somewhere
nothing opens, or opens a path nothing is mounted at.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#image-or-dockerfile-specific-properties`,
		`https://containers.dev/implementors/spec/#workspacefolder-and-workspacemount`,
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "workspaceMount": "source=${localWorkspaceFolder},target=/srv/app,type=bind"
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "workspaceMount": "source=${localWorkspaceFolder},target=/srv/app,type=bind",
  "workspaceFolder": "/srv/app"
}
`},
			},
		},
	},
	Check: checkMissingWorkspaceMountFolder,
}

func checkMissingWorkspaceMountFolder(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok || (!hasMember(obj, "image") && !hasMember(obj, "build")) {
		return nil
	}

	hasMount := hasMember(obj, "workspaceMount")
	hasFolder := hasMember(obj, "workspaceFolder")
	if hasMount == hasFolder {
		return nil
	}

	present, missing := "workspaceMount", "workspaceFolder"
	if hasFolder {
		present, missing = "workspaceFolder", "workspaceMount"
	}
	return []linter.Finding{{
		Message: fmt.Sprintf("devcontainer.json sets %q but is missing %q", present, missing),
		Offset:  node.Value.StartOffset,
	}}
}
