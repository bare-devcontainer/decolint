package format

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
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
		return fmt.Errorf("marshal issues: %w", err)
	}
	buf.WriteByte('\n')
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write issues: %w", err)
	}
	return nil
}
