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

// BuildConfig is the Dockerfile build a devcontainer.json declares: the Dockerfile it builds and
// the options shaping what that build produces. They are read together because the options say
// nothing without the Dockerfile they shape.
type BuildConfig struct {
	// Dockerfile is the Dockerfile's path, relative to the devcontainer.json.
	Dockerfile string
	// DockerfileDecl is where the Dockerfile is named, whichever of the two properties names it.
	DockerfileDecl Decl
	// Args are the arguments passed to the build, nil when it declares none.
	Args map[string]string
	// Target is the stage the build stops at, empty when it declares none.
	Target string
}

// Build returns the Dockerfile build obj declares. ok is false when it declares no Dockerfile, the
// options alone building nothing.
//
// The specification defines two mutually exclusive ways to name the Dockerfile, the top-level
// "dockerFile" and the nested "build.dockerfile"; the top-level one wins, as the reference
// implementation prefers it (getDockerfile: 'dockerFile' in config ? config.dockerFile :
// config.build.dockerfile). The options are always the "build" object's, which the legacy top-level
// form carries alongside it.
func Build(obj *hujson.Object) (config BuildConfig, ok bool) {
	config.Dockerfile, config.DockerfileDecl, ok = dockerfilePath(obj)
	if !ok {
		return BuildConfig{}, false
	}
	build := buildObject(obj)
	if build == nil {
		return config, true
	}
	if m := memberNamed(build, "args"); m != nil {
		if argsObj, isObj := m.Value.Value.(*hujson.Object); isObj {
			for _, arg := range argsObj.Members {
				name, nameOK := arg.Name.Value.(hujson.Literal)
				value, valueOK := arg.Value.Value.(hujson.Literal)
				if !nameOK || name.Kind() != '"' || !valueOK || value.Kind() != '"' {
					continue
				}
				if config.Args == nil {
					config.Args = map[string]string{}
				}
				config.Args[name.String()] = value.String()
			}
		}
	}
	if m := memberNamed(build, "target"); m != nil {
		if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
			config.Target = lit.String()
		}
	}
	return config, true
}

// dockerfilePath returns the path the configuration names its Dockerfile by, in either form.
func dockerfilePath(obj *hujson.Object) (string, Decl, bool) {
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

// ComposeConfig is the Compose declaration of a devcontainer.json: the files it names and the
// service the dev container runs in. The two are read together because neither settles anything
// alone — a configuration that names files but no service says which Compose project it belongs to
// and not which container it is.
type ComposeConfig struct {
	// Files are the Compose file paths, in the order declared, later ones overriding earlier ones.
	// It is empty when the declaration names no readable path.
	Files []string
	// FilesDecl is where "dockerComposeFile" is written.
	FilesDecl Decl
	// Service is the service the dev container runs in, empty when the configuration names none or
	// names it as something other than a string.
	Service string
	// ServiceDecl is where "service" is written; the zero value when the configuration has none.
	ServiceDecl Decl
}

// Usable reports whether the declaration settles a container: a service to attach to, and at least
// one file that may define it. A declaration that does not is still a Compose declaration, so a
// caller must not fall back to another form on it — see [Compose].
func (c ComposeConfig) Usable() bool {
	return len(c.Files) > 0 && c.Service != ""
}

// Compose returns the Compose declaration of obj. declared reports whether "dockerComposeFile" is
// there at all, which is what tells a Compose-based configuration from one that builds or pulls an
// image; whether the declaration settles a container is [ComposeConfig.Usable].
//
// "dockerComposeFile" is a single path or an array of them. A value, or an element, that is not a
// string contributes no path, so a declaration can be there with no path to read.
func Compose(obj *hujson.Object) (config ComposeConfig, declared bool) {
	m := memberNamed(obj, "dockerComposeFile")
	if m == nil {
		return ComposeConfig{}, false
	}
	switch v := m.Value.Value.(type) {
	case hujson.Literal:
		if v.Kind() == '"' {
			config.Files = []string{v.String()}
		}
	case *hujson.Array:
		for _, e := range v.Elements {
			if lit, isLit := e.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
				config.Files = append(config.Files, lit.String())
			}
		}
	}
	config.FilesDecl = declOf(m)

	if m := memberNamed(obj, "service"); m != nil {
		if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
			config.Service, config.ServiceDecl = lit.String(), declOf(m)
		}
	}
	return config, true
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
