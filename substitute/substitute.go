// Package substitute resolves the ${...} variables in a devcontainer.json at lint time, following
// the reference implementation (devcontainers/cli, src/spec-common/variableSubstitution.ts) with
// one deliberate divergence: a variable whose value the linter cannot know resolves to the empty
// string instead of being deferred to container creation, so no ${...} survives substitution.
//
// Variable values come from:
//
//   - ${localEnv:NAME} and ${env:NAME}: [Context.LocalEnv] only — the host environment is never
//     read. An absent name resolves to the default argument (${localEnv:NAME:default}) or the
//     empty string.
//   - ${localWorkspaceFolder} and ${localWorkspaceFolderBasename}: [Context.LocalWorkspaceFolder].
//   - ${containerWorkspaceFolder} and ${containerWorkspaceFolderBasename}: the configuration's own
//     "workspaceFolder", or its spec default.
//   - ${devcontainerId}: the fixed placeholder [DevcontainerID], but only in the properties the
//     spec allows it in (those not used to build the image, since the id is unknown at build time;
//     see [devcontainerIDProperties]). Elsewhere it resolves to the empty string.
//   - Anything else (${containerEnv:...}, unrecognized or malformed variables): the empty string.
package substitute

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tailscale/hujson"
)

// DevcontainerID is the placeholder every ${devcontainerId} resolves to. It has the format of a
// real id — 52 base-32 chars, as devcontainers/cli derives from a SHA-256 of the container's id
// labels, which only exist once a container is created — and is the real algorithm applied to the
// fixed input "decolint".
const DevcontainerID = "1chpi9f3o037fhb9uo08e7p6i6i29ikr887u4fh4eq6a2rml99ua"

// devcontainerIDProperties are the devcontainer.json top-level properties in which ${devcontainerId}
// may appear. The spec restricts it to properties not used to build the image, because the id is not
// known at (pre-)build time. See
// https://containers.dev/implementors/json_reference/#variables-in-devcontainerjson.
var devcontainerIDProperties = map[string]struct{}{
	"name":                 {},
	"runArgs":              {},
	"initializeCommand":    {},
	"onCreateCommand":      {},
	"updateContentCommand": {},
	"postCreateCommand":    {},
	"postStartCommand":     {},
	"postAttachCommand":    {},
	"workspaceFolder":      {},
	"workspaceMount":       {},
	"mounts":               {},
	"containerEnv":         {},
	"remoteEnv":            {},
	"containerUser":        {},
	"remoteUser":           {},
	"customizations":       {},
}

// Context supplies the values variables resolve to.
type Context struct {
	// LocalEnv maps names to the values ${localEnv:NAME} and ${env:NAME} resolve to. Host
	// environment variables are never read, and lookups are case-sensitive on every platform.
	LocalEnv map[string]string
	// LocalWorkspaceFolder is the absolute host path of the directory the devcontainer
	// configuration belongs to.
	LocalWorkspaceFolder string
}

// variablePattern matches one ${...} occurrence. The lazy body is the same pattern the reference
// implementation uses, so nested braces match identically: in "${a${b}}" the match is "${a${b}"
// (an unknown variable) and the trailing "}" is left alone.
var variablePattern = regexp.MustCompile(`\$\{(.*?)\}`)

// Apply resolves variables in every string value of root in place. Object member names are never
// substituted, and replaced text is not re-scanned, both per the reference implementation. Every
// node keeps its original byte offsets, so findings on substituted values still point at the
// source text.
//
// ${devcontainerId} is resolved only within the top-level properties that allow it (see
// [devcontainerIDProperties]); elsewhere it resolves to the empty string like an unknowable
// variable.
func Apply(ctx Context, root *hujson.Value) {
	containerWS := containerWorkspaceFolder(ctx, root)
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		apply(ctx, containerWS, false, root)
		return
	}
	for i := range obj.Members {
		name := ""
		if lit, ok := obj.Members[i].Name.Value.(hujson.Literal); ok && lit.Kind() == '"' {
			name = lit.String()
		}
		_, idAllowed := devcontainerIDProperties[name]
		apply(ctx, containerWS, idAllowed, &obj.Members[i].Value)
	}
}

// apply resolves variables in v and its descendants. idAllowed reports whether the top-level
// property v lives under permits ${devcontainerId}; it propagates unchanged into nested nodes.
func apply(ctx Context, containerWS string, idAllowed bool, v *hujson.Value) {
	switch t := v.Value.(type) {
	case *hujson.Object:
		for i := range t.Members {
			apply(ctx, containerWS, idAllowed, &t.Members[i].Value)
		}
	case *hujson.Array:
		for i := range t.Elements {
			apply(ctx, containerWS, idAllowed, &t.Elements[i])
		}
	case hujson.Literal:
		if t.Kind() != '"' {
			return
		}
		s := t.String()
		if resolved := resolveString(ctx, containerWS, idAllowed, s); resolved != s {
			v.Value = hujson.String(resolved)
		}
	}
}

// containerWorkspaceFolder computes the value ${containerWorkspaceFolder} resolves to, from the
// raw configuration: the "workspaceFolder" member if it is a string, else the spec default — "/"
// for a Docker Compose configuration, /workspaces/<localWorkspaceFolderBasename> otherwise. A
// value taken from the configuration is itself resolved once, with any self-reference inside it
// reading the raw value, as the reference implementation does.
func containerWorkspaceFolder(ctx Context, root *hujson.Value) string {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return ""
	}
	if raw, ok := stringMember(obj, "workspaceFolder"); ok {
		// ${devcontainerId} is left unresolved here: this value feeds the container workspace
		// folder, a build-time value the id is not available for.
		return resolveString(ctx, raw, false, raw)
	}
	if hasMember(obj, "dockerComposeFile") {
		return "/"
	}
	return path.Join("/workspaces", filepath.Base(ctx.LocalWorkspaceFolder))
}

// resolveString resolves every variable in s. containerWS is the value ${containerWorkspaceFolder}
// resolves to. idAllowed reports whether s lives in a property that permits ${devcontainerId}.
//
// Divergences from the reference implementation, which defers what it cannot resolve locally: an
// unrecognized variable (kept as written there) and a bare ${localEnv}/${env} (a configuration
// error there) both resolve to the empty string, like an undefined environment variable.
func resolveString(ctx Context, containerWS string, idAllowed bool, s string) string {
	return variablePattern.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[len("${") : len(match)-len("}")]
		parts := strings.Split(inner, ":")
		name, args := parts[0], parts[1:]
		switch name {
		case "env", "localEnv":
			if len(args) == 0 {
				return ""
			}
			if value, ok := ctx.LocalEnv[args[0]]; ok {
				return value
			}
			// The default is the second argument alone; anything after a further colon is
			// dropped, as the reference implementation does.
			if len(args) > 1 {
				return args[1]
			}
			return ""
		case "localWorkspaceFolder":
			return ctx.LocalWorkspaceFolder
		case "localWorkspaceFolderBasename":
			return filepath.Base(ctx.LocalWorkspaceFolder)
		case "containerWorkspaceFolder":
			return containerWS
		case "containerWorkspaceFolderBasename":
			return posixBasename(containerWS)
		case "devcontainerId":
			if idAllowed {
				return DevcontainerID
			}
			return ""
		default:
			return ""
		}
	})
}

// posixBasename returns the last path segment of p under POSIX rules with Node.js semantics, which
// the reference implementation uses for container paths: unlike [path.Base], the basename of "/"
// (a Compose configuration's default workspace folder) is "".
func posixBasename(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// hasMember reports whether obj has a member named name.
func hasMember(obj *hujson.Object, name string) bool {
	for i := range obj.Members {
		if lit, ok := obj.Members[i].Name.Value.(hujson.Literal); ok && lit.Kind() == '"' && lit.String() == name {
			return true
		}
	}
	return false
}

// stringMember returns the string value of obj's member named name; ok is false when the member is
// absent or not a string.
func stringMember(obj *hujson.Object, name string) (string, bool) {
	for i := range obj.Members {
		lit, ok := obj.Members[i].Name.Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' || lit.String() != name {
			continue
		}
		value, ok := obj.Members[i].Value.Value.(hujson.Literal)
		if !ok || value.Kind() != '"' {
			return "", false
		}
		return value.String(), true
	}
	return "", false
}
