package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// IDDirMismatch reports a Feature's or Template's "id" property when it does not match the name of
// the directory containing its metadata file, per the Dev Container Features/Templates convention.
var IDDirMismatch = &linter.Rule{
	ID:          "id-dir-mismatch",
	Description: `disallow a Feature's or Template's "id" that does not match the name of its containing directory`,
	LongDescription: `Both specifications require the "id" to match the name of the directory holding the metadata file, since
that directory name is what packaging and distribution address the artifact by. When the two disagree the
published reference does not resolve to what the directory contains; rename the directory or the "id" so
they agree.`,
	References: []string{
		`https://containers.dev/implementors/features/#devcontainer-featurejson-properties`,
		`https://containers.dev/implementors/templates/#devcontainer-templatejson-properties`,
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Feature, linter.Template},
	Paths:     []string{"/id"},
	Example: linter.Example{
		Bad: linter.Snippet{
			DirName: "node",
			Files: []linter.ExampleFile{
				{Path: `devcontainer-feature.json`, Content: `// src/node/devcontainer-feature.json
{
  "id": "nodejs",
  "version": "1.0.0",
  "name": "Node.js"
}
`},
			},
		},
		Good: linter.Snippet{
			DirName: "node",
			Files: []linter.ExampleFile{
				{Path: `devcontainer-feature.json`, Content: `// src/node/devcontainer-feature.json
{
  "id": "node",
  "version": "1.0.0",
  "name": "Node.js"
}
`},
			},
		},
	},
	Check: checkIDDirMismatch,
}

func checkIDDirMismatch(ctx *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	if ctx.Dir.Name == "" {
		return nil
	}
	id := lit.String()
	if id == ctx.Dir.Name {
		return nil
	}
	return []linter.Finding{{
		Message: fmt.Sprintf("id %q does not match containing directory %q", id, ctx.Dir.Name),
		Offset:  node.Value.StartOffset,
	}}
}
