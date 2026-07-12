package rules

import (
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// RequireNonRoot reports a devcontainer.json that does not clearly configure a non-root user. Per
// the devcontainer.json spec, "remoteUser" is the user any lifecycle script and remote editor/IDE
// server or terminal session runs as, defaulting to "containerUser" (and, ultimately, the image's
// own default user) when unset. Both properties are therefore consulted: "remoteUser" is checked
// first, falling back to "containerUser" only when "remoteUser" is unset. It is off by default
// because most configs don't set either property and enabling it by default would be noisy.
var RequireNonRoot = &linter.Rule{
	ID:          "require-non-root",
	Description: `require "remoteUser" or, if unset, "containerUser" to be set to a non-root user`,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Paths:       []string{""},
	Check:       checkRequireNonRoot,
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

	// "remoteUser" defaults to "containerUser" when unset, so fall back to it.
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
