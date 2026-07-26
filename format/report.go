package format

import (
	"fmt"
	"io"

	"github.com/bare-devcontainer/decolint/linter"
)

// Report is the outcome of a lint run: the configuration files that were linted and the issues
// found in them.
type Report struct {
	// ConfigPath is the config file the run's settings came from, empty when none was found. Only
	// [TextFormat] renders it; the machine-readable formats leave it out.
	ConfigPath string
	// Files are the configuration files that were linted, in the order they were visited. A file
	// with no issue is listed too, so the report shows what was covered and not just what fired.
	Files []File
	// Issues are the findings, in the order they were reported.
	Issues []linter.Issue
}

// File is a configuration file that was linted.
type File struct {
	Path string          `json:"path"`
	Type linter.FileType `json:"type"`
}

// writeFileList writes files as a human-readable block, one indented line per file naming its path
// and the kind of configuration it was detected as. It writes nothing when no file was linted, so
// callers need not guard the call.
func writeFileList(w io.Writer, st styler, files []File) error {
	if len(files) == 0 {
		return nil
	}
	heading := fmt.Sprintf("Linted %d file%s:", len(files), pluralize(len(files)))
	if _, err := fmt.Fprintln(w, st.bold(heading)); err != nil {
		return fmt.Errorf("write file list: %w", err)
	}
	for _, f := range files {
		if _, err := fmt.Fprintf(w, "  %s %s\n", f.Path, st.dim("("+string(f.Type)+")")); err != nil {
			return fmt.Errorf("write file list: %w", err)
		}
	}
	return nil
}
