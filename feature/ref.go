// Package feature fetches Dev Container Features referenced by a devcontainer.json and merges the
// properties they contribute into the parsed configuration, producing the effective configuration
// defined by the Dev Container specification's merge logic (see
// https://containers.dev/implementors/spec/#merge-logic).
package feature

import (
	"fmt"
	"strings"
)

// RefKind identifies how a Feature reference locates the Feature.
type RefKind int

const (
	// KindOCI is a reference to a Feature distributed as an OCI artifact, e.g.
	// "ghcr.io/devcontainers/features/node:1".
	KindOCI RefKind = iota
	// KindTarball is a direct HTTP(S) URI to a Feature tarball.
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
	// Registry, Repository, Tag, and Digest are the components of an OCI reference; they are empty
	// for other kinds. Tag defaults to "latest" when the reference names neither a tag nor a digest.
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

// ParseRef parses a Feature reference. Relative paths ("./..." or "../...") are local Features,
// HTTP(S) URIs are tarball Features, and everything else is parsed as an OCI reference of the form
// "registry/repository[:tag][@digest]".
func ParseRef(raw string) (Ref, error) {
	if strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		return Ref{Raw: raw, Kind: KindLocal}, nil
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return Ref{Raw: raw, Kind: KindTarball}, nil
	}

	ref := Ref{Raw: raw, Kind: KindOCI}
	rest := raw
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		ref.Digest = rest[at+1:]
		rest = rest[:at]
		if !strings.HasPrefix(ref.Digest, "sha256:") {
			return Ref{}, fmt.Errorf("invalid feature reference %q: digest must start with \"sha256:\"", raw)
		}
	}
	// A colon after the last slash separates the tag; a colon before it belongs to a registry host
	// with a port (e.g. "localhost:5000/f").
	if colon := strings.LastIndex(rest, ":"); colon > strings.LastIndex(rest, "/") {
		ref.Tag = rest[colon+1:]
		rest = rest[:colon]
	}
	if ref.Tag == "" && ref.Digest == "" {
		ref.Tag = "latest"
	}

	registry, repository, ok := strings.Cut(rest, "/")
	if !ok || registry == "" || repository == "" {
		return Ref{}, fmt.Errorf("invalid feature reference %q: want \"registry/repository[:tag]\"", raw)
	}
	ref.Registry = registry
	ref.Repository = repository
	return ref, nil
}
