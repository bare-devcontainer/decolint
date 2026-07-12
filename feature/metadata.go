package feature

import (
	"fmt"

	"github.com/tailscale/hujson"
)

// Metadata is the declaration of one fetched Feature: the content of its
// devcontainer-feature.json.
type Metadata struct {
	// ID is the Feature's declared identifier.
	ID string
	// Version is the Feature's declared version.
	Version string
	// DependsOn lists the references of the Features this Feature depends on, in declaration
	// order. Dependencies are installed before the Feature and contribute properties of their own.
	DependsOn []string
	// InstallsAfter lists Feature IDs this Feature prefers to be installed after. Unlike DependsOn
	// it does not pull in new Features; it only influences installation order.
	InstallsAfter []string
	// Root is the parsed devcontainer-feature.json with comments stripped. It is the source of
	// truth for the properties the Feature contributes (e.g. Root.Find("/containerEnv")).
	Root hujson.Value
}

// parseMetadata parses src, the content of a devcontainer-feature.json.
func parseMetadata(src []byte) (*Metadata, error) {
	root, err := hujson.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse devcontainer-feature.json: %w", err)
	}
	root.Minimize()
	if _, ok := root.Value.(*hujson.Object); !ok {
		return nil, fmt.Errorf("parse devcontainer-feature.json: root is not an object")
	}

	md := &Metadata{Root: root}
	if v := root.Find("/id"); v != nil {
		if lit, ok := v.Value.(hujson.Literal); ok && lit.Kind() == '"' {
			md.ID = lit.String()
		}
	}
	if v := root.Find("/version"); v != nil {
		if lit, ok := v.Value.(hujson.Literal); ok && lit.Kind() == '"' {
			md.Version = lit.String()
		}
	}
	if v := root.Find("/dependsOn"); v != nil {
		if obj, ok := v.Value.(*hujson.Object); ok {
			for _, m := range obj.Members {
				if lit, ok := m.Name.Value.(hujson.Literal); ok && lit.Kind() == '"' {
					md.DependsOn = append(md.DependsOn, lit.String())
				}
			}
		}
	}
	if v := root.Find("/installsAfter"); v != nil {
		if arr, ok := v.Value.(*hujson.Array); ok {
			for _, e := range arr.Elements {
				if lit, ok := e.Value.(hujson.Literal); ok && lit.Kind() == '"' {
					md.InstallsAfter = append(md.InstallsAfter, lit.String())
				}
			}
		}
	}
	return md, nil
}
