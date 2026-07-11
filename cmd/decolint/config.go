package main

import (
	"encoding/json/v2"
	"fmt"
	"os"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// Config is the on-disk shape of a decolint config file.
type Config struct {
	// Rules maps a rule ID to the severity it should be overridden to.
	Rules map[string]linter.Severity `json:"rules"`
}

// defaultConfigNames are the config file names discovered automatically in the current directory
// when no explicit path is given, in precedence order (first match wins).
var defaultConfigNames = []string{".decolint.jsonc", ".decolint.json"}

// loadConfig loads the config file at path and returns the Config it declares. If path is empty,
// the first of defaultConfigNames found in the current directory is used instead; if none exists,
// loadConfig returns a zero Config and no error.
//
// It is an error if path is explicitly given and the file does not exist, can't be read, or fails
// to parse.
func loadConfig(path string) (Config, error) {
	if path == "" {
		found, err := findDefaultConfig()
		if err != nil {
			return Config{}, err
		}
		if found == "" {
			return Config{}, nil
		}
		path = found
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	return parseConfig(path, src)
}

// findDefaultConfig returns the path of the first of defaultConfigNames that exists as a regular
// file in the current directory, or "" if none does.
func findDefaultConfig() (string, error) {
	for _, name := range defaultConfigNames {
		info, err := os.Stat(name)
		if err == nil && !info.IsDir() {
			return name, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("stat config %s: %w", name, err)
		}
	}
	return "", nil
}

// parseConfig parses src, the JSONC content of the config file at path, into a Config. path is used
// only to annotate error messages.
func parseConfig(path string, src []byte) (Config, error) {
	std, err := hujson.Standardize(src)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(std, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}
