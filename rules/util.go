package rules

import (
	"encoding/csv"
	"iter"
	"path"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// dockerSocketPath is the standard path of the Docker daemon's UNIX socket on Linux hosts. GitHub
// Codespaces makes an exception for a bind mount whose source is this path, forwarding it into the
// container even though other bind mounts are ignored.
const dockerSocketPath = "/var/run/docker.sock"

// isDockerSocketSource reports whether source, a mount source, names the host's Docker socket. It is
// the single answer to that question, so that a rule reporting the socket and a rule excusing it
// never disagree about the same mount.
//
// The daemon cleans a bind mount's source before using it, so spellings such as
// "//var/run/docker.sock" and "/var/run/docker.sock/" all reach the same socket.
func isDockerSocketSource(source string) bool {
	return path.Clean(source) == dockerSocketPath
}

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

// runArgsFlagValues yields every value that arr, a "runArgs" array, gives to flag, in order. Docker
// accepts such a value either as a single combined "flag=value" entry or as two adjacent entries,
// "flag" followed by "value". Each yielded pair is the hujson.Value holding the value and the value
// itself.
func runArgsFlagValues(arr *hujson.Array, flag string) iter.Seq2[*hujson.Value, string] {
	return func(yield func(*hujson.Value, string) bool) {
		for i := range arr.Elements {
			lit, ok := arr.Elements[i].Value.(hujson.Literal)
			if !ok || lit.Kind() != '"' {
				continue
			}

			if v, ok := strings.CutPrefix(lit.String(), flag+"="); ok {
				if !yield(&arr.Elements[i], v) {
					return
				}
				continue
			}

			if lit.String() != flag || i+1 >= len(arr.Elements) {
				continue
			}
			next, ok := arr.Elements[i+1].Value.(hujson.Literal)
			if ok && next.Kind() == '"' && !yield(&arr.Elements[i+1], next.String()) {
				return
			}
		}
	}
}

// runArgsFindFlagValue returns the hujson.Value holding the first value arr gives to flag that match
// accepts, or nil if it gives flag no such value. See [runArgsFlagValues] for the entry forms it
// recognizes.
func runArgsFindFlagValue(arr *hujson.Array, flag string, match func(string) bool) *hujson.Value {
	for v, s := range runArgsFlagValues(arr, flag) {
		if match(s) {
			return v
		}
	}
	return nil
}

// parseMountString extracts the "type" and "source" fields from s, a "--mount" value, as docker/cli
// reads it:
//   - the value is trimmed of surrounding whitespace and then read as a single CSV record, so a
//     field may be quoted to protect a comma inside it;
//   - field keys are matched case-insensitively, and "src" is an alias for "source";
//   - the type is normalized to lower case.
//
// It returns empty strings for a value that is not a well-formed CSV record, which docker rejects
// outright.
//
// Whitespace around a key or a value is the one deliberate deviation: docker rejects it, but reading
// such a field anyway lets the rules name the mount the author meant rather than fall silent on a
// value that already fails to start the container.
func parseMountString(s string) (mountType, source string) {
	fields, err := csv.NewReader(strings.NewReader(strings.TrimSpace(s))).Read()
	if err != nil {
		return "", ""
	}
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "type":
			mountType = strings.ToLower(value)
		case "source", "src":
			source = value
		}
	}
	return mountType, source
}

// parseMountObject extracts the "type" and "source" members from obj, a "mounts" entry. The member
// names are the ones devcontainers/cli reads off the object and so are case-sensitive, but it hands
// their values to docker as a "--mount" value, which lower-cases the type.
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
			mountType = strings.ToLower(value.String())
		case "source":
			source = value.String()
		}
	}
	return mountType, source
}

// volumeSpecSource returns the host path or volume name that s, a "-v"/"--volume" value, mounts, or
// "" if s names no source. A volume spec shares no syntax with a "--mount" value: its fields are
// separated by colons and a comma is an ordinary character. A spec of a single field is an anonymous
// volume, and that field is the container path, not a host path.
func volumeSpecSource(s string) string {
	source, _, ok := strings.Cut(s, ":")
	if !ok {
		return ""
	}
	return source
}

// parseMount extracts the "type" and "source" fields from v, a "mounts" entry, which may be either
// the "--mount" string shorthand (see [parseMountString]) or an object with corresponding members.
// ok is false if v is neither.
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
