package schema

import (
	"encoding/json/v2"
	"fmt"
	"sync"
)

// revisions is the decoded data/REVISIONS.json: the upstream commits the vendored schemas were taken
// from.
type revisions struct {
	Spec   string `json:"spec"`
	VSCode string `json:"vscode"`
}

var (
	revisionOnce sync.Once
	revisionStr  string
)

// Revision reports the upstream commits the embedded schemas were vendored from, as
// "spec@<sha> vscode@<sha>", for display by "decolint -version". A malformed or missing revisions
// file yields "unknown" rather than an error, since it must not stop the program from reporting its
// version.
func Revision() string {
	revisionOnce.Do(func() {
		revisionStr = "unknown"
		b, err := data.ReadFile("data/REVISIONS.json")
		if err != nil {
			return
		}
		var rev revisions
		if err := json.Unmarshal(b, &rev); err != nil {
			return
		}
		if rev.Spec == "" || rev.VSCode == "" {
			return
		}
		revisionStr = fmt.Sprintf("spec@%s vscode@%s", short(rev.Spec), short(rev.VSCode))
	})
	return revisionStr
}

// short truncates a git commit hash to its 7-character short form.
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
