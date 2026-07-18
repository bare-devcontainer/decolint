// Package feature fetches Dev Container Features referenced by a devcontainer.json and merges the
// properties they contribute into the parsed configuration, producing the effective configuration
// defined by the Dev Container specification's merge logic (see
// https://containers.dev/implementors/spec/#merge-logic).
package feature

import (
	"fmt"
	"strings"

	"oras.land/oras-go/v2/registry"
)

// RefKind identifies how a Feature reference locates the Feature.
type RefKind int

const (
	// KindOCI is a reference to a Feature distributed as an OCI artifact, e.g.
	// "ghcr.io/devcontainers/features/node:1".
	KindOCI RefKind = iota
	// KindTarball is a direct HTTPS URI to a Feature tarball.
	KindTarball
	// KindLocal is a relative path to a Feature directory next to the devcontainer.json.
	KindLocal
)

// Ref is a parsed Feature reference, as used for the keys of the "features" object in a
// devcontainer.json.
type Ref struct {
	// Raw is the reference exactly as written.
	Raw string
	// Kind identifies how the reference locates the Feature.
	Kind RefKind
	// OCI holds the parsed registry reference (registry, repository, and the tag or digest) for a
	// KindOCI reference; it is the zero value for other kinds.
	OCI registry.Reference
}

// ParseRef parses a Feature reference. Relative paths ("./..." or "../...") are local Features,
// HTTPS URIs are tarball Features, and everything else is parsed as an OCI reference of the form
// "registry/repository[:tag][@digest]".
func ParseRef(raw string) (Ref, error) {
	if strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		return Ref{Raw: raw, Kind: KindLocal}, nil
	}
	if strings.HasPrefix(raw, "https://") {
		return Ref{Raw: raw, Kind: KindTarball}, nil
	}

	// oras-go parses and validates the OCI reference (registry host, repository, tag, and digest),
	// the same grammar it enforces when the Feature is later fetched from the registry.
	parsed, err := registry.ParseReference(raw)
	if err != nil {
		return Ref{}, fmt.Errorf("invalid feature reference %q: %w", raw, err)
	}
	return Ref{Raw: raw, Kind: KindOCI, OCI: parsed}, nil
}
