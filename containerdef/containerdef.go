// Package containerdef reads what a devcontainer.json declares its container is made from: an image
// it pulls, a Dockerfile it builds, or a Compose service it attaches to. It reads the declaration
// and nothing else — resolving it is the caller's, whether that is the merge fetching what the
// declaration names or a lint rule reading it from the linted directory.
//
// Every reader returns the byte offsets of both the key and the value, so a caller can anchor at
// whichever the reader of its output expects to see.
package containerdef

import "github.com/tailscale/hujson"

// Decl is a property's value as declared, with where it is written.
type Decl struct {
	// KeyOffset is the byte offset of the property name.
	KeyOffset int
	// ValueOffset is the byte offset of the value.
	ValueOffset int
}

// Image returns the image "image" names. ok is false when the property is absent or is not a string.
func Image(obj *hujson.Object) (ref string, decl Decl, ok bool) {
	m := memberNamed(obj, "image")
	if m == nil {
		return "", Decl{}, false
	}
	lit, isLit := m.Value.Value.(hujson.Literal)
	if !isLit || lit.Kind() != '"' {
		return "", Decl{}, false
	}
	return lit.String(), declOf(m), true
}

// Dockerfile returns the Dockerfile path the configuration builds from. The specification defines
// two mutually exclusive forms, the top-level "dockerFile" and the nested "build.dockerfile"; the
// top-level one wins, as the reference implementation prefers it (getDockerfile: 'dockerFile' in
// config ? config.dockerFile : config.build.dockerfile). ok is false when neither names one.
func Dockerfile(obj *hujson.Object) (path string, decl Decl, ok bool) {
	if m := memberNamed(obj, "dockerFile"); m != nil {
		if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
			return lit.String(), declOf(m), true
		}
	}
	if build := buildObject(obj); build != nil {
		if m := memberNamed(build, "dockerfile"); m != nil {
			if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
				return lit.String(), declOf(m), true
			}
		}
	}
	return "", Decl{}, false
}

// BuildOptions returns the "build" options that shape what the Dockerfile produces: the arguments
// passed to the build, and the stage it stops at. Both are zero when "build" declares none.
func BuildOptions(obj *hujson.Object) (args map[string]string, target string) {
	build := buildObject(obj)
	if build == nil {
		return nil, ""
	}
	if m := memberNamed(build, "args"); m != nil {
		if argsObj, isObj := m.Value.Value.(*hujson.Object); isObj {
			for _, arg := range argsObj.Members {
				name, nameOK := arg.Name.Value.(hujson.Literal)
				value, valueOK := arg.Value.Value.(hujson.Literal)
				if !nameOK || name.Kind() != '"' || !valueOK || value.Kind() != '"' {
					continue
				}
				if args == nil {
					args = map[string]string{}
				}
				args[name.String()] = value.String()
			}
		}
	}
	if m := memberNamed(build, "target"); m != nil {
		if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
			target = lit.String()
		}
	}
	return args, target
}

// ComposeFiles returns the Compose file paths "dockerComposeFile" names, in the order they are
// declared, later ones overriding earlier ones. The property is a single path or an array of them.
//
// declared reports whether the property is there at all, which is what tells a Compose-based
// configuration from one that builds or pulls an image. A declaration whose value, or whose element,
// is not a string contributes no path, so declared can be true with no paths: the configuration says
// it is Compose-based while naming nothing readable.
func ComposeFiles(obj *hujson.Object) (paths []string, decl Decl, declared bool) {
	m := memberNamed(obj, "dockerComposeFile")
	if m == nil {
		return nil, Decl{}, false
	}
	switch v := m.Value.Value.(type) {
	case hujson.Literal:
		if v.Kind() == '"' {
			paths = []string{v.String()}
		}
	case *hujson.Array:
		for _, e := range v.Elements {
			if lit, isLit := e.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
				paths = append(paths, lit.String())
			}
		}
	}
	return paths, declOf(m), true
}

// ComposeService returns the Compose service "service" names, the one the dev container runs in. ok
// is false when the property is absent or is not a string.
func ComposeService(obj *hujson.Object) (name string, decl Decl, ok bool) {
	m := memberNamed(obj, "service")
	if m == nil {
		return "", Decl{}, false
	}
	lit, isLit := m.Value.Value.(hujson.Literal)
	if !isLit || lit.Kind() != '"' {
		return "", Decl{}, false
	}
	return lit.String(), declOf(m), true
}

// buildObject returns the "build" object, or nil when the configuration declares none or declares it
// as something other than an object.
func buildObject(obj *hujson.Object) *hujson.Object {
	m := memberNamed(obj, "build")
	if m == nil {
		return nil
	}
	build, ok := m.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	return build
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

func declOf(m *hujson.ObjectMember) Decl {
	return Decl{KeyOffset: m.Name.StartOffset, ValueOffset: m.Value.StartOffset}
}
