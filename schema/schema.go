// Package schema validates devcontainer configuration files against the official Dev Container JSON
// Schemas. It complements the rule engine: the schemas cover the shape of a file — property names,
// value types, enums, and unknown properties — while rules cover security, reproducibility, and
// cross-property semantics.
//
// The schemas are vendored (see the data directory) and embedded, so validation needs no network
// access. [Revision] reports the upstream commits they were taken from.
package schema

import (
	"bytes"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Variant selects which devcontainer.json schema to validate against.
type Variant int

const (
	// VariantOff disables schema validation.
	VariantOff Variant = iota
	// VariantBase validates against the spec base schema only, rejecting VS Code- and
	// Codespaces-specific properties.
	VariantBase
	// VariantMain validates against the main schema, which additionally allows the VS Code and
	// Codespaces extensions to devcontainer.json.
	VariantMain
)

// ParseVariant parses a variant name ("off", "base", or "main"). Any other value is an error.
func ParseVariant(s string) (Variant, error) {
	switch s {
	case "off":
		return VariantOff, nil
	case "base":
		return VariantBase, nil
	case "main":
		return VariantMain, nil
	default:
		return VariantOff, fmt.Errorf("invalid schema variant %q: want base, main, or off", s)
	}
}

// String returns the variant's name, as accepted by [ParseVariant].
func (v Variant) String() string {
	switch v {
	case VariantOff:
		return "off"
	case VariantBase:
		return "base"
	case VariantMain:
		return "main"
	default:
		return "unknown"
	}
}

// Kind identifies which file schema applies. Templates have no official schema and are not passed to
// [Validate].
type Kind int

const (
	// KindDevcontainer selects the devcontainer.json schema (base or main per the variant).
	KindDevcontainer Kind = iota
	// KindFeature selects the devcontainer-feature.json schema, independent of the variant.
	KindFeature
)

// Diagnostic is one schema violation, positioned by a byte offset into the source that produced std.
type Diagnostic struct {
	Message string
	Offset  int
}

// Validate validates std, the standardized (comment- and trailing-comma-free) JSON bytes of a file
// of the given kind, against the selected schema, and returns its violations. It returns nil when
// v is [VariantOff] or the document is valid.
//
// offsetFor maps a schema instance location (a slice of unescaped JSON Pointer segments) to a byte
// offset into std, so each diagnostic can be positioned; see [github.com/bare-devcontainer/decolint/linter.Document.OffsetAt].
func Validate(v Variant, kind Kind, std []byte, offsetFor func(loc []string) int) ([]Diagnostic, error) {
	if v == VariantOff {
		return nil, nil
	}

	entryURL, props, err := setup(v, kind)
	if err != nil {
		return nil, err
	}
	sch, err := compile(entryURL)
	if err != nil {
		return nil, err
	}

	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(std))
	if err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}
	verr := sch.Validate(inst)
	if verr == nil {
		return nil, nil
	}
	root, ok := verr.(*jsonschema.ValidationError)
	if !ok {
		return nil, fmt.Errorf("validate document: %w", verr)
	}
	return diagnostics(root, props, v == VariantMain, offsetFor), nil
}

// setup resolves the schema entry URL and property sets for a variant and kind.
func setup(v Variant, kind Kind) (string, propertyNames, error) {
	if kind == KindFeature {
		props, err := featurePropertyNames()
		return urlFeature, props, err
	}
	props, err := devcontainerPropertyNames()
	if v == VariantBase {
		return urlBase, props, err
	}
	return urlMain, props, err
}
