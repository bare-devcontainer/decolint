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
	Category:    linter.CategoryCorrectness,
	FileTypes:   []linter.FileType{linter.Feature},
	Paths:       []string{""},
	Check:       checkFeatureInstallScriptNotExecutable,
}

func checkFeatureInstallScriptNotExecutable(ctx *linter.Context, node *linter.Node) []linter.Finding {
	// Windows working trees carry no executable bits (git does not set them there), so the check
	// would report every install.sh as non-executable. Skip it rather than emit false positives.
	if ctx.Dir == nil || runtime.GOOS == "windows" {
		return nil
	}
	info, err := fs.Stat(ctx.Dir, installScriptName)
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
