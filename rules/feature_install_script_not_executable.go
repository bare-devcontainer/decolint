package rules

import (
	"fmt"
	"io/fs"
	"runtime"

	"github.com/bare-devcontainer/decolint/linter"
)

// FeatureInstallScriptNotExecutable reports a Feature whose install.sh exists but carries no
// executable permission bit, so the container runtime cannot run it. Reporting a missing install.sh
// is [MissingFeatureInstallScript]'s job; the two never fire together.
var FeatureInstallScriptNotExecutable = &linter.Rule{
	ID:          "feature-install-script-not-executable",
	Description: "disallow a Feature's `install.sh` that lacks executable permission bits",
	LongDescription: `The specification has the installing tool invoke "install.sh" directly rather than through a shell, so
that the script's own shebang selects the interpreter. That requires the execute bit: without it the
Feature fails to install when a container is built. Run "chmod +x install.sh" and commit the mode change.`,
	References: []string{
		`https://containers.dev/implementors/features/#invoking-installsh`,
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Feature},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: "devcontainer-feature.json", Content: featureInstallScriptExampleFeature},
				{Path: installScriptName, Content: featureInstallScriptExampleScript, Mode: 0o644},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: "devcontainer-feature.json", Content: featureInstallScriptExampleFeature},
				{Path: installScriptName, Content: featureInstallScriptExampleScript, Mode: 0o755},
			},
		},
		Note: "Git records the executable bit, so committing the mode change is what makes\n" +
			"the fix stick. On Windows, where the filesystem has no executable bit, set it\n" +
			"in the index directly: `git update-index --chmod=+x install.sh`.",
	},
	Check: checkFeatureInstallScriptNotExecutable,
}

const featureInstallScriptExampleFeature = `{
  "id": "node",
  "version": "1.0.0",
  "name": "Node.js"
}
`

const featureInstallScriptExampleScript = `#!/usr/bin/env bash
set -e
apt-get update && apt-get install -y nodejs
`

func checkFeatureInstallScriptNotExecutable(ctx *linter.Context, node *linter.Node) []linter.Finding {
	// Windows working trees carry no executable bits (git does not set them there), so the check
	// would report every install.sh as non-executable. Skip it rather than emit false positives.
	if ctx.Dir.FS == nil || runtime.GOOS == "windows" {
		return nil
	}
	info, err := fs.Stat(ctx.Dir.FS, installScriptName)
	if err != nil || info.IsDir() {
		return nil
	}
	if info.Mode().Perm()&0o111 != 0 {
		return nil
	}
	return []linter.Finding{{
		Message: fmt.Sprintf(`"install.sh" is not executable (mode %04o); run "chmod +x install.sh"`, info.Mode().Perm()),
		Offset:  node.Value.StartOffset,
	}}
}
