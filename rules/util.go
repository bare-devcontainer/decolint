package rules

import (
	"encoding/csv"
	"iter"
	"path"
	"strings"

	"github.com/bare-devcontainer/decolint/dockerargs"
	"github.com/bare-devcontainer/decolint/feature"
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

// isAllCapability reports whether s names the pseudo-capability standing for every Linux
// capability. See [dockerargs.Capability] for the spellings that reach it.
func isAllCapability(s string) bool {
	return dockerargs.Capability(s) == dockerargs.AllCapabilities
}

// securityOptSeccompProfile returns the seccomp profile s, a "securityOpt" entry, selects, and
// reports whether it selects one at all. See [dockerargs.ParseSecurityOpt] for the forms an entry
// can be written in.
func securityOptSeccompProfile(s string) (profile string, ok bool) {
	opt, ok := dockerargs.ParseSecurityOpt(s)
	if !ok || opt.Key != "seccomp" {
		return "", false
	}
	return opt.Value, true
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

// stringArrayContains reports whether any array member of obj named name (see [arrayMembers])
// contains a string element for which match returns true.
func stringArrayContains(obj *hujson.Object, name string, match func(string) bool) bool {
	for arr := range arrayMembers(obj, name) {
		for _, elem := range arr.Elements {
			lit, ok := elem.Value.(hujson.Literal)
			if ok && lit.Kind() == '"' && match(lit.String()) {
				return true
			}
		}
	}
	return false
}

// arrayMembers yields the value of every member of obj named name that is an array, skipping the
// members whose value is not one. A malformed object may repeat a name: JSON parsers keep a single
// copy — the last one — but decolint reads them all rather than report as unset something the
// document plainly sets.
func arrayMembers(obj *hujson.Object, name string) iter.Seq[*hujson.Array] {
	return func(yield func(*hujson.Array) bool) {
		for _, m := range obj.Members {
			nameLit, ok := m.Name.Value.(hujson.Literal)
			if !ok || nameLit.String() != name {
				continue
			}
			arr, ok := m.Value.Value.(*hujson.Array)
			if !ok {
				continue
			}
			if !yield(arr) {
				return
			}
		}
	}
}

// runArgsHasFlagValue reports whether any "runArgs" member of obj (see [arrayMembers]) gives the
// "docker run" flag named flag a value match accepts. flag is the flag's long name without the
// leading "--", so "volume" covers both "-v" and "--volume".
//
// It is for the rules that report a flag's absence, which the engine cannot hand an occurrence of.
// A rule reporting a flag's presence declares a "/runArgs/--flag" path instead and never reads the
// array itself.
func runArgsHasFlagValue(obj *hujson.Object, flag string, match func(string) bool) bool {
	for arr := range arrayMembers(obj, "runArgs") {
		for _, arg := range linter.RunArgs(arr) {
			if arg.Flag == flag && match(arg.Value) {
				return true
			}
		}
	}
	return false
}

// underRunArgs reports whether node is a value inside a "runArgs" rather than the property a rule's
// other paths name. Only a devcontainer.json's "runArgs" is a "docker run" argv: in a Feature or a
// Template it is ordinary data, so a "/runArgs/--flag" path matches a member merely spelled like a
// flag there, with [linter.Node.Arg] nil just as on the property. A rule reporting both a property
// and a flag must ignore such a node.
func underRunArgs(node *linter.Node) bool {
	return strings.HasPrefix(node.Pointer, "/runArgs/")
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

// holdsFeatureRefs reports whether pointer names the property that holds Feature references in a
// file of the given type: "features" in a devcontainer.json, "dependsOn" in a Feature. A rule
// declares its paths for every file type it applies to, so one covering both properties is offered
// each of them in each file — including the combinations the specification does not define.
func holdsFeatureRefs(fileType linter.FileType, pointer string) bool {
	switch fileType {
	case linter.Devcontainer:
		return pointer == "/features"
	case linter.Feature:
		return pointer == "/dependsOn"
	default:
		return false
	}
}

// featureRef is a Feature reference, as written for a key of a devcontainer.json "features" or a
// Feature's "dependsOn", with how it locates the Feature and the byte offset of that key.
type featureRef struct {
	ref    string
	kind   feature.RefKind
	offset int
}

// featureRefs returns the Feature references the members of v are keyed by, for a v that is an
// object of them. It returns none for a value that is not one.
//
// References are told apart by [feature.ParseRef], which is what resolves them for the merge, so a
// rule reads the three forms the specification defines and the reference implementation accepts. One
// that parses as none of them names no Feature to report on and is left out; a rule that can report
// on only some kinds skips the rest by their [featureRef.kind].
//
// A caller reading the version a reference names takes it from the reference as written, not from
// the parsed [feature.Ref]: ParseRef normalizes a reference with no version to the "latest" tag, and
// a rule telling those two apart would lose the distinction.
func featureRefs(v *hujson.Value) []featureRef {
	obj, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	var refs []featureRef
	for _, m := range obj.Members {
		name, ok := m.Name.Value.(hujson.Literal)
		if !ok || name.Kind() != '"' {
			continue
		}
		ref := name.String()
		parsed, err := feature.ParseRef(ref)
		if err != nil {
			continue
		}
		refs = append(refs, featureRef{ref: ref, kind: parsed.Kind, offset: m.Name.StartOffset})
	}
	return refs
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
