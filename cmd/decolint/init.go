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
	buf.WriteString(`// Each entry under "rules" sets that rule's severity: "error", "warn", or "off".
// Whole categories (correctness, security, reproducibility, style) can be set at once
// under "categories"; per-rule entries take precedence, e.g.:
//   "categories": { "security": "error" }
// "platforms" lists target platforms whose rules run in addition to
// platform-agnostic ones (the -platform flag takes precedence), e.g.:
//   "platforms": ["vscode", "codespaces"]
// "merge", when true, fetches the Features referenced in each
// devcontainer.json and lints the merged (effective) configuration, e.g.:
//   "merge": true
// "denyWarnings", when true, treats warnings as failures (exit code 1);
// the -deny-warnings flag takes precedence, e.g.:
//   "denyWarnings": true
// "format" selects the output format ("text", "json", or "github"); the
// -format flag takes precedence, e.g.:
//   "format": "github"
// "schema" selects the Dev Container schema variant ("main", "base", or
// "off"); "base" rejects VS Code/Codespaces properties, "off" disables
// schema validation; the -schema flag takes precedence, e.g.:
//   "schema": "base"
// "localEnv" supplies the values ${localEnv:NAME} resolves to when merging,
// and the environment Compose-file interpolation reads; environment
// variables are never read, e.g.:
//   "localEnv": { "USERPROFILE": "/home/user" }
`)
	if err := json.MarshalWrite(&buf, cfg, jsontext.Multiline(true)); err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	buf.WriteByte('\n')

	if err := os.WriteFile(name, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	if _, err := fmt.Fprintf(output, "wrote %s\n", name); err != nil {
		return fmt.Errorf("write confirmation: %w", err)
	}
	return nil
}
