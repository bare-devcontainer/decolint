package feature

import (
	"fmt"

	"github.com/tailscale/hujson"
)

// Metadata is the declaration of one fetched Feature: the content of its
// devcontainer-feature.json.
type Metadata struct {
	// ID is the Feature's declared identifier, or "" when it declares none (as image-metadata
	// entries do; see [contributor.hasID]).
	ID      string
	Version string
	// DependsOn lists the Features this Feature depends on, in declaration order. Dependencies are
	// installed before the Feature and contribute properties of their own.
	DependsOn []Dependency
	// InstallsAfter lists Feature IDs this Feature prefers to be installed after. Unlike DependsOn
	// it does not pull in new Features; it only influences installation order.
	InstallsAfter []string
	// Aliases are the Feature's identifiers, its ID followed by any legacy IDs. They match a renamed
	// Feature named by another Feature's "installsAfter" or by "overrideFeatureInstallOrder".
	Aliases []string
	// Digest is the resolved manifest digest of an OCI Feature (e.g. "sha256:..."). It is empty for
	// local and tarball Features. It distinguishes otherwise identical references for install order.
	Digest string
	// Root is the parsed devcontainer-feature.json. It is the source of truth for the properties
	// the Feature contributes (e.g. Root.Find("/containerEnv")); values grafted from it are
	// stripped of comments and re-anchored into the target document at merge time.
	Root hujson.Value
}

// Dependency is one entry of a Feature's "dependsOn": the reference of a required Feature and the
// options it is requested with. The options make an otherwise identical dependency a distinct
// contributor for install ordering.
type Dependency struct {
	Ref     string
	Options optionValue
}

// parseMetadata parses src, the content of a devcontainer-feature.json.
func parseMetadata(src []byte) (*Metadata, error) {
	root, err := hujson.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse devcontainer-feature.json: %w", err)
	}
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
					md.DependsOn = append(md.DependsOn, Dependency{Ref: lit.String(), Options: parseOptions(m.Value)})
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
	md.Aliases = parseAliases(root, md.ID)
	return md, nil
}

// parseAliases returns the Feature's [Metadata.Aliases]: its id followed by any "legacyIds". A
// Feature that declares no id yields at most its "legacyIds".
func parseAliases(root hujson.Value, id string) []string {
	var aliases []string
	if id != "" {
		aliases = append(aliases, id)
	}
	if v := root.Find("/legacyIds"); v != nil {
		if arr, ok := v.Value.(*hujson.Array); ok {
			for _, e := range arr.Elements {
				// An empty alias would spuriously match any other Feature carrying one, so a malformed
				// entry is dropped rather than treated as an identifier.
				if lit, ok := e.Value.(hujson.Literal); ok && lit.Kind() == '"' && lit.String() != "" {
					aliases = append(aliases, lit.String())
				}
			}
		}
	}
	return aliases
}
