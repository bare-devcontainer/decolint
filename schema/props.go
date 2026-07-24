package schema

import "sync"

// propertyNames derives, from the vendored schemas, the property-name sets the diagnostics use:
//   - candidates: every property name declared anywhere in the schemas, used to suggest a correction
//     for a misspelled property.
//   - extensionRoot: the root properties contributed only by the VS Code and Codespaces schemas.
//     Because the main schema composes those as sibling allOf branches of the base schema, the base
//     schema's root "unevaluatedProperties": false cannot observe them and reports them as unknown.
//     Suppressing that class of false positive is what lets the main variant accept these properties.
type propertyNames struct {
	candidates    map[string]bool
	extensionRoot map[string]bool
}

var (
	devcontainerPropsOnce sync.Once
	devcontainerProps     propertyNames
	devcontainerPropsErr  error
	featurePropsOnce      sync.Once
	featureProps          propertyNames
	featurePropsErr       error
)

// devcontainerPropertyNames returns the property sets for devcontainer.json across the base, VS Code,
// and Codespaces schemas.
func devcontainerPropertyNames() (propertyNames, error) {
	devcontainerPropsOnce.Do(func() {
		candidates := map[string]bool{}
		for _, path := range []string{
			"data/devContainer.base.schema.json",
			"data/devContainer.codespaces.schema.json",
			"data/devContainer.vscode.schema.json",
		} {
			doc, err := readJSON(path)
			if err != nil {
				devcontainerPropsErr = err
				return
			}
			collectPropertyNames(doc, candidates)
		}
		extensionRoot := map[string]bool{}
		for _, path := range []string{
			"data/devContainer.codespaces.schema.json",
			"data/devContainer.vscode.schema.json",
		} {
			doc, err := readJSON(path)
			if err != nil {
				devcontainerPropsErr = err
				return
			}
			for name := range rootProperties(doc) {
				extensionRoot[name] = true
			}
		}
		devcontainerProps = propertyNames{candidates: candidates, extensionRoot: extensionRoot}
	})
	return devcontainerProps, devcontainerPropsErr
}

// featurePropertyNames returns the property sets for devcontainer-feature.json. The feature schema
// has no cross-resource composition, so extensionRoot is empty.
func featurePropertyNames() (propertyNames, error) {
	featurePropsOnce.Do(func() {
		doc, err := readJSON("data/devContainerFeature.schema.json")
		if err != nil {
			featurePropsErr = err
			return
		}
		candidates := map[string]bool{}
		collectPropertyNames(doc, candidates)
		featureProps = propertyNames{candidates: candidates, extensionRoot: map[string]bool{}}
	})
	return featureProps, featurePropsErr
}

// rootProperties returns the keys of the top-level "properties" object of a schema document.
func rootProperties(doc any) map[string]bool {
	names := map[string]bool{}
	obj, ok := doc.(map[string]any)
	if !ok {
		return names
	}
	props, ok := obj["properties"].(map[string]any)
	if !ok {
		return names
	}
	for name := range props {
		names[name] = true
	}
	return names
}

// collectPropertyNames walks a decoded schema and records every key that appears under a
// "properties" object, at any depth, into out.
func collectPropertyNames(node any, out map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		if props, ok := v["properties"].(map[string]any); ok {
			for name := range props {
				out[name] = true
			}
		}
		for _, child := range v {
			collectPropertyNames(child, out)
		}
	case []any:
		for _, child := range v {
			collectPropertyNames(child, out)
		}
	}
}
