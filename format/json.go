package format

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"io"

	"github.com/bare-devcontainer/decolint/linter"
)

// JSONFormat prints the report as a JSON object.
type JSONFormat struct{}

// jsonReport is the wire shape of the JSON output. Members marshal in declaration order, so the
// output is deterministic.
type jsonReport struct {
	Files  []File         `json:"files"`
	Issues []linter.Issue `json:"issues"`
}

// WriteReport writes report to w as a JSON object with a "files" and an "issues" member, both
// always arrays. It marshals into an in-memory buffer first so that a failure never leaves partial
// JSON on w.
func (JSONFormat) WriteReport(w io.Writer, report Report) error {
	out := jsonReport{Files: report.Files, Issues: report.Issues}
	if out.Files == nil {
		out.Files = []File{}
	}
	if out.Issues == nil {
		out.Issues = []linter.Issue{}
	}
	var buf bytes.Buffer
	if err := json.MarshalWrite(&buf, out); err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	buf.WriteByte('\n')
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
