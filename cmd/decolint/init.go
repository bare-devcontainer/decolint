package main

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

// initConfigFile writes a fresh .decolint.jsonc file to the current directory, listing every
// built-in rule at its default severity. It writes a confirmation message to output. It is an
// error if the file already exists, so -init never silently overwrites a user's customized config.
func initConfigFile(output io.Writer) error {
	name := defaultConfigNames[0] // ".decolint.jsonc"

	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("%s already exists", name)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", name, err)
	}

	cfg := Config{Rules: make(map[string]linter.Severity, len(rules.Builtin()))}
	for _, reg := range rules.Builtin() {
		cfg.Rules[reg.Rule.ID] = reg.DefaultSeverity
	}

	var buf bytes.Buffer
	if err := json.MarshalWrite(&buf, cfg, jsontext.Multiline(true)); err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	buf.WriteByte('\n')

	if err := os.WriteFile(name, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	_, err := fmt.Fprintf(output, "wrote %s\n", name)
	return err
}
