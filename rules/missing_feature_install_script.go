package rules

import (
	"errors"
	"io/fs"

	"github.com/bare-devcontainer/decolint/linter"
)

// installScriptName is the name of the install entrypoint the Features specification requires next
// to a Feature's devcontainer-feature.json.
const installScriptName = "install.sh"

// MissingFeatureInstallScript reports a Feature whose directory has no install.sh install script,
// the entrypoint the Features specification requires alongside devcontainer-feature.json.
var MissingFeatureInstallScript = &linter.Rule{
	ID:          "missing-feature-install-script",
	Description: "disallow a Feature directory without the required `install.sh` install script",
	LongDescription: `A Feature is distributed as its metadata file plus the "install.sh" the tooling runs inside the container,
which is where the Feature does all of its work. A directory without one publishes a Feature that
installs nothing, and the omission only surfaces when someone builds a container with it.`,
	References: []string{
		"https://containers.dev/implementors/features/#folder-structure",
		"https://containers.dev/implementors/features/#invoking-installsh",
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Feature},
	Paths:     []string{""},
	Check:     checkMissingFeatureInstallScript,
}

func checkMissingFeatureInstallScript(ctx *linter.Context, node *linter.Node) []linter.Finding {
	if ctx.Dir.FS == nil {
		return nil
	}
	info, err := fs.Stat(ctx.Dir.FS, installScriptName)
	switch {
	case err == nil && !info.IsDir():
		return nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		// The directory exists but install.sh could not be stat'd for some other reason; don't guess
		// that it is missing.
		return nil
	}
	return []linter.Finding{{
		Message: `Feature has no "install.sh" install script next to devcontainer-feature.json`,
		Offset:  node.Value.StartOffset,
	}}
}
