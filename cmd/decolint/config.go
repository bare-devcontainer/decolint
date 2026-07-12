package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// Config is the on-disk shape of a decolint config file.
type Config struct {
	// Platforms lists target platforms whose rules are linted in addition to platform-agnostic
	// ones. The -platform flag, when given, takes precedence.
	Platforms []linter.Platform `json:"platforms"`
	// MergeFeatures, when true, fetches the Features referenced in each devcontainer.json and
	// lints the merged (effective) configuration. The -merge-features flag can enable it as well.
	MergeFeatures bool `json:"mergeFeatures"`
	// Categories maps a category name to the severity every rule in that category should be
	// overridden to. Per-rule entries in Rules take precedence.
	Categories map[string]linter.Severity `json:"categories"`
	// Rules maps a rule ID to the severity it should be overridden to.
	Rules map[string]linter.Severity `json:"rules"`
}

// MarshalJSONTo encodes cfg with its categories and rules written in sorted key order, for use
// with encoding/json/v2. Map iteration order is otherwise unspecified, so without this, marshaling
// the same Config twice could produce differently ordered output. The "platforms",
// "mergeFeatures", and "categories" members are omitted when empty or false, so generated configs
// (see initConfigFile) stay minimal.
func (cfg Config) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if len(cfg.Platforms) > 0 {
		if err := enc.WriteToken(jsontext.String("platforms")); err != nil {
			return err
		}
		if err := json.MarshalEncode(enc, cfg.Platforms); err != nil {
			return err
		}
	}
	if cfg.MergeFeatures {
		if err := enc.WriteToken(jsontext.String("mergeFeatures")); err != nil {
			return err
		}
		if err := enc.WriteToken(jsontext.Bool(true)); err != nil {
			return err
		}
	}
	if len(cfg.Categories) > 0 {
		if err := enc.WriteToken(jsontext.String("categories")); err != nil {
			return err
		}
		if err := writeSeverityMap(enc, cfg.Categories); err != nil {
			return err
		}
	}
	if err := enc.WriteToken(jsontext.String("rules")); err != nil {
		return err
	}
	if err := writeSeverityMap(enc, cfg.Rules); err != nil {
		return err
	}
	return enc.WriteToken(jsontext.EndObject)
}

// writeSeverityMap encodes m as a JSON object with its members in sorted key order.
func writeSeverityMap(enc *jsontext.Encoder, m map[string]linter.Severity) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(m)) {
		if err := enc.WriteToken(jsontext.String(key)); err != nil {
			return err
		}
		if err := json.MarshalEncode(enc, m[key]); err != nil {
			return err
		}
	}
	return enc.WriteToken(jsontext.EndObject)
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
