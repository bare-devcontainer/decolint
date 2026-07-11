package format

import (
	"bytes"
	"encoding/json/v2"
	"io"

	"github.com/bare-devcontainer/decolint/linter"
)

// JSONFormat prints a JSON array of issues.
type JSONFormat struct{}

// WriteIssues writes issues to w as a JSON array. It marshals into an in-memory buffer
// first so that a failure never leaves partial JSON on w.
func (JSONFormat) WriteIssues(w io.Writer, issues []linter.Issue) error {
	if issues == nil {
		issues = []linter.Issue{}
	}
	var buf bytes.Buffer
	if err := json.MarshalWrite(&buf, issues); err != nil {
		return err
	}
	buf.WriteByte('\n')
	_, err := w.Write(buf.Bytes())
	return err
}
