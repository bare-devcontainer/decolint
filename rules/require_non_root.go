package rules

import (
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// RequireNonRoot reports a devcontainer.json that does not clearly configure a non-root user. Per
// the devcontainer.json spec, "remoteUser" is the user any lifecycle script and remote editor/IDE
// server or terminal session runs as, defaulting to "containerUser" (and, ultimately, the image's
// own default user) when unset. Both properties are therefore consulted: "remoteUser" first, then
// "containerUser" if "remoteUser" is unset. It is off by default
// because most configs don't set either property and enabling it by default would be noisy.
var RequireNonRoot = &linter.Rule{
	ID:          "require-non-root",
	Description: `require "remoteUser" or, if unset, "containerUser" to be set to a non-root user`,
	LongDescription: `"remoteUser" defaults to whatever user the container runs as, which for most images is root. Everything
the developer's session drives then runs as root: lifecycle scripts, terminals, and the language servers
and build tools the editor starts, so a compromised dependency runs with full control of the container.
Naming an unprivileged user — as the specification's own images do — costs nothing and contains it.`,
	References: []string{
		`https://containers.dev/implementors/json_reference/#remoteUser`,
		`https://containers.dev/implementors/spec/#users`,
	},
	Category:  linter.CategorySecurity,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "remoteUser": "root"
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "remoteUser": "vscode"
}
`},
			},
		},
		Note: "`" + `remoteUser` + "`" + ` is what lifecycle scripts and the editor's remote session run as,
and it wins over ` + "`" + `containerUser` + "`" + `; a rule that finds neither reports the
container's default, which is ` + "`" + `root` + "`" + ` for most images.`,
	},
	Check: checkRequireNonRoot,
}

func checkRequireNonRoot(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	if remoteUser, ok := stringMember(obj, "remoteUser"); ok {
		if !isRootUser(remoteUser) {
			return nil
		}
		return []linter.Finding{{
			Message: `"remoteUser" is set to "root", running lifecycle scripts and any remote editor/IDE session as root`,
			Offset:  node.Value.StartOffset,
		}}
	}

	if containerUser, ok := stringMember(obj, "containerUser"); ok {
		if !isRootUser(containerUser) {
			return nil
		}
		return []linter.Finding{{
			Message: `"remoteUser" is not set and "containerUser" is set to "root", running lifecycle scripts and any remote editor/IDE session as root`,
			Offset:  node.Value.StartOffset,
		}}
	}

	return []linter.Finding{{
		Message: `neither "remoteUser" nor "containerUser" is set, so the container defaults to running as root`,
		Offset:  node.Value.StartOffset,
	}}
}

// isRootUser reports whether s, a "containerUser" or "remoteUser" value, identifies the root user.
// Such values may pair a user with a group after a colon (e.g. "root:root" or "0:0"); only the user
// part is considered.
func isRootUser(s string) bool {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	return s == "root" || s == "0"
}
