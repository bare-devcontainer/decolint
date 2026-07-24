package schema

import (
	"bytes"
	"embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// data holds the vendored official Dev Container schemas. They are refreshed by "make
// update-schemas"; the upstream commits they were taken from are recorded in data/REVISIONS.json
// and reported by [Revision].
//
//go:embed data
var data embed.FS

// Canonical URLs the schemas are registered under. They must match the $ref targets used inside the
// schemas verbatim: devContainer.schema.json refers to the base schema relatively and to the two
// VS Code schemas by these absolute URLs, so registering the vendored copies here lets compilation
// resolve every reference offline.
const (
	urlBase       = "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.base.schema.json"
	urlMain       = "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.schema.json"
	urlFeature    = "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainerFeature.schema.json"
	urlCodespaces = "https://raw.githubusercontent.com/microsoft/vscode/main/extensions/configuration-editing/schemas/devContainer.codespaces.schema.json"
	urlVSCode     = "https://raw.githubusercontent.com/microsoft/vscode/main/extensions/configuration-editing/schemas/devContainer.vscode.schema.json"
)

// vscodeInternalRefs are $ref targets in the VS Code schema that address editor-internal resources
// (machine settings, MCP configuration). They are unreachable outside VS Code, so each is registered
// as a permissive stub: decolint validates the devcontainer structure, not the VS Code settings blob.
var vscodeInternalRefs = []string{
	"vscode://schemas/settings/machine",
	"vscode://schemas/mcp",
}

// compiled caches one compiled schema per entry URL; compilation is deterministic and the schemas
// are read-only, so a single package-wide cache is safe for concurrent use.
var (
	compiledMu sync.Mutex
	compiled   = map[string]*jsonschema.Schema{}
)

// compile returns the schema compiled from the vendored resource registered under entryURL.
func compile(entryURL string) (*jsonschema.Schema, error) {
	compiledMu.Lock()
	defer compiledMu.Unlock()
	if s, ok := compiled[entryURL]; ok {
		return s, nil
	}

	c := jsonschema.NewCompiler()
	// Every reference resolves to a registered resource, so no loader ever runs. Installing one that
	// fails loudly turns a forgotten vendored dependency (e.g. a newly added upstream $ref) into a
	// clear compile error instead of a silent network fetch.
	c.UseLoader(errorLoader{})

	resources := map[string]string{
		urlBase:       "data/devContainer.base.schema.json",
		urlMain:       "data/devContainer.schema.json",
		urlFeature:    "data/devContainerFeature.schema.json",
		urlCodespaces: "data/devContainer.codespaces.schema.json",
		urlVSCode:     "data/devContainer.vscode.schema.json",
	}
	for url, path := range resources {
		doc, err := readJSON(path)
		if err != nil {
			return nil, err
		}
		if err := c.AddResource(url, doc); err != nil {
			return nil, fmt.Errorf("add schema resource %s: %w", url, err)
		}
	}
	for _, url := range vscodeInternalRefs {
		if err := c.AddResource(url, map[string]any{}); err != nil {
			return nil, fmt.Errorf("add stub resource %s: %w", url, err)
		}
	}

	s, err := c.Compile(entryURL)
	if err != nil {
		return nil, fmt.Errorf("compile schema %s: %w", entryURL, err)
	}
	compiled[entryURL] = s
	return s, nil
}

// errorLoader rejects any attempt to load a schema that was not pre-registered, keeping validation
// fully offline.
type errorLoader struct{}

func (errorLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("schema %s is not vendored; run \"make update-schemas\"", url)
}

// readJSON decodes an embedded schema file into the value shape the validator expects.
func readJSON(path string) (any, error) {
	b, err := data.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read embedded schema %s: %w", path, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode embedded schema %s: %w", path, err)
	}
	return doc, nil
}
