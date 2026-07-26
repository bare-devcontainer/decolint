package rules

import (
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// ConflictingContainerDef reports a devcontainer.json that defines more than one of "image",
// "build", or "dockerComposeFile". The schema treats these container-definition variants as mutually
// exclusive, so exactly one may be set.
var ConflictingContainerDef = &linter.Rule{
	ID:          "conflicting-container-def",
	Description: `disallow a devcontainer.json that defines more than one of "image", "build", or "dockerComposeFile"`,
	LongDescription: `The specification defines three mutually exclusive ways to create the container: from an image, from a
Dockerfile, or from a Docker Compose project. Which one wins when several are set is unspecified, so the
container that gets built depends on the tool rather than on the configuration. Keep the variant the
project actually uses and remove the others.`,
	References: []string{
		`https://containers.dev/implementors/spec/#orchestration-options`,
		`https://containers.dev/implementors/json_reference/#scenario-specific`,
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "name": "my project",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "build": {
    "dockerfile": "Dockerfile"
  }
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "name": "my project",
  "build": {
    "dockerfile": "Dockerfile"
  }
}
`},
			},
		},
	},
	Check: checkConflictingContainerDef,
}

func checkConflictingContainerDef(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	var defined []*hujson.ObjectMember
	for _, name := range []string{"image", "build", "dockerComposeFile"} {
		if m := memberNamed(obj, name); m != nil {
			defined = append(defined, m)
		}
	}
	if len(defined) < 2 {
		return nil
	}
	// One finding per excess definition, anchored at the offending key, so the report points at the
	// keys that conflict with the first-declared variant rather than at the whole object.
	findings := make([]linter.Finding, 0, len(defined)-1)
	for _, m := range defined[1:] {
		findings = append(findings, linter.Finding{
			Message: `devcontainer.json must define only one of "image", "build", or "dockerComposeFile"`,
			Offset:  m.Name.StartOffset,
		})
	}
	return findings
}
