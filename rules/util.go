package rules

import (
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// dockerSocketPath is the standard path of the Docker daemon's UNIX socket on Linux hosts. GitHub
// Codespaces makes an exception for a bind mount whose source is this path, forwarding it into the
// container even though other bind mounts are ignored.
const dockerSocketPath = "/var/run/docker.sock"

// hasMember reports whether obj has a member named name.
func hasMember(obj *hujson.Object, name string) bool {
	return memberNamed(obj, name) != nil
}

// memberNamed returns obj's member named name, or nil if obj has no such member.
func memberNamed(obj *hujson.Object, name string) *hujson.ObjectMember {
	for i := range obj.Members {
		if lit, ok := obj.Members[i].Name.Value.(hujson.Literal); ok && lit.String() == name {
			return &obj.Members[i]
		}
	}
	return nil
}

// stringArrayContains reports whether obj has a member named name whose value is an array
// containing a string element for which match returns true. It returns false if obj has no such
// member or the member's value is not an array.
func stringArrayContains(obj *hujson.Object, name string, match func(string) bool) bool {
	for _, m := range obj.Members {
		nameLit, ok := m.Name.Value.(hujson.Literal)
		if !ok || nameLit.String() != name {
			continue
		}
		arr, ok := m.Value.Value.(*hujson.Array)
		if !ok {
			return false
		}
		for _, elem := range arr.Elements {
			lit, ok := elem.Value.(hujson.Literal)
			if ok && lit.Kind() == '"' && match(lit.String()) {
				return true
			}
		}
		return false
	}
	return false
}

// arrayMember returns obj's member named name as an array. ok is false if obj has no such member or
// the member's value is not an array.
func arrayMember(obj *hujson.Object, name string) (arr *hujson.Array, ok bool) {
	for _, m := range obj.Members {
		nameLit, isLit := m.Name.Value.(hujson.Literal)
		if !isLit || nameLit.String() != name {
			continue
		}
		arr, ok = m.Value.Value.(*hujson.Array)
		return arr, ok
	}
	return nil, false
}

// runArgsFindFlagValue scans arr, a "runArgs" array, for an entry that sets flag to a value accepted
// by match. Docker accepts such a value either as a single combined "flag=value" entry or as two
// adjacent entries, "flag" followed by "value". It returns the hujson.Value holding the matching
// value, or nil if no entry sets flag to a value match accepts.
func runArgsFindFlagValue(arr *hujson.Array, flag string, match func(string) bool) *hujson.Value {
	for i := range arr.Elements {
		lit, ok := arr.Elements[i].Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' {
			continue
		}

		if v, ok := strings.CutPrefix(lit.String(), flag+"="); ok {
			if match(v) {
				return &arr.Elements[i]
			}
			continue
		}

		if lit.String() != flag || i+1 >= len(arr.Elements) {
			continue
		}
		next, ok := arr.Elements[i+1].Value.(hujson.Literal)
		if ok && next.Kind() == '"' && match(next.String()) {
			return &arr.Elements[i+1]
		}
	}
	return nil
}

// parseMountString extracts the "type" and "source" fields from s, a "key=value,..." mount entry.
func parseMountString(s string) (mountType, source string) {
	for _, part := range strings.Split(s, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "type":
			mountType = strings.TrimSpace(value)
		case "source":
			source = strings.TrimSpace(value)
		}
	}
	return mountType, source
}

// parseMountObject extracts the "type" and "source" members from obj, a "mounts" entry.
func parseMountObject(obj *hujson.Object) (mountType, source string) {
	for _, m := range obj.Members {
		name, ok := m.Name.Value.(hujson.Literal)
		if !ok {
			continue
		}
		value, ok := m.Value.Value.(hujson.Literal)
		if !ok || value.Kind() != '"' {
			continue
		}
		switch name.String() {
		case "type":
			mountType = value.String()
		case "source":
			source = value.String()
		}
	}
	return mountType, source
}

// parseMount extracts the "type" and "source" fields from v, a "mounts" entry, which may be either
// the "key=value,..." string shorthand or an object with corresponding members. ok is false if v is
// neither.
func parseMount(v *hujson.Value) (mountType, source string, ok bool) {
	switch val := v.Value.(type) {
	case hujson.Literal:
		if val.Kind() != '"' {
			return "", "", false
		}
		mountType, source = parseMountString(val.String())
		return mountType, source, true
	case *hujson.Object:
		mountType, source = parseMountObject(val)
		return mountType, source, true
	default:
		return "", "", false
	}
}

// stringMember returns the string value of obj's member named name and reports whether it is
// present with a string value.
func stringMember(obj *hujson.Object, name string) (string, bool) {
	for _, m := range obj.Members {
		nameLit, ok := m.Name.Value.(hujson.Literal)
		if !ok || nameLit.String() != name {
			continue
		}
		lit, ok := m.Value.Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' {
			return "", false
		}
		return lit.String(), true
	}
	return "", false
}

// runArgsApplicable reports whether ctx is for a devcontainer.json, the only file type where
// "runArgs" is meaningful; a Feature has no use for it, so rules should not flag one there.
func runArgsApplicable(ctx *linter.Context) bool {
	return ctx.Type == linter.Devcontainer
}

// refTag extracts the tag from an OCI-style reference, e.g. a container image or Feature reference.
// A reference pinned by digest (e.g. "ref@sha256:...") is treated as tagged. The colon in a
// registry host with a port (e.g. "localhost:5000/img") is not a tag separator.
func refTag(ref string) (tag string, ok bool) {
	if strings.Contains(ref, "@") {
		return "", true
	}
	colon := strings.LastIndex(ref, ":")
	if colon < 0 || colon < strings.LastIndex(ref, "/") {
		return "", false
	}
	return ref[colon+1:], true
}
