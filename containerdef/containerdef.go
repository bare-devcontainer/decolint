// Package containerdef reads what a devcontainer.json declares its container is made from: an image
// it pulls, a Dockerfile it builds, or a Compose service it attaches to. It reads the declaration
// and nothing else — resolving it is the caller's, whether that is the merge fetching what the
// declaration names or a lint rule reading it from the linted directory.
//
// Each declaration carries the byte offsets of the property name declaring it and of its value, so
// a caller can anchor what it reports at whichever the reader of its output expects to see.
package containerdef

import (
	"iter"

	"github.com/tailscale/hujson"
)

// Def is one form a devcontainer.json declares its container in: [*ImageDef], [*BuildDef] or
// [*ComposeDef]. A caller tells them apart with a type switch.
type Def interface {
	containerDef()
}

// ImageDef is an image the configuration pulls, as "image" names it.
type ImageDef struct {
	// Ref is the image reference as written.
	Ref string
	// KeyOffset is the byte offset of the "image" property name.
	KeyOffset int
	// ValueOffset is the byte offset of the reference.
	ValueOffset int
}

func (*ImageDef) containerDef()   {}
func (*BuildDef) containerDef()   {}
func (*ComposeDef) containerDef() {}

// Defs yields the container definitions obj declares, in the order the reference implementation
// resolves them: the Compose declaration, then the Dockerfile build, then the image. Each is read as
// it is reached, so a caller that stops at the first reads no further than it.
//
// Only one form is valid at a time, so the order matters only for a configuration declaring several.
// A caller resolving the container the configuration produces takes the first; one reading
// everything the configuration names takes them all.
func Defs(obj *hujson.Object) iter.Seq[Def] {
	return func(yield func(Def) bool) {
		if def := compose(obj); def != nil && !yield(def) {
			return
		}
		if def := build(obj); def != nil && !yield(def) {
			return
		}
		if def := image(obj); def != nil {
			yield(def)
		}
	}
}

// image returns the image "image" names, or nil when the property is absent or is not a string.
func image(obj *hujson.Object) *ImageDef {
	m := memberNamed(obj, "image")
	if m == nil {
		return nil
	}
	lit, isLit := m.Value.Value.(hujson.Literal)
	if !isLit || lit.Kind() != '"' {
		return nil
	}
	return &ImageDef{Ref: lit.String(), KeyOffset: m.Name.StartOffset, ValueOffset: m.Value.StartOffset}
}

// BuildDef is the Dockerfile build a devcontainer.json declares: the Dockerfile it builds and
// the options shaping what that build produces. They are read together because the options say
// nothing without the Dockerfile they shape.
type BuildDef struct {
	// Dockerfile is the Dockerfile's path, relative to the devcontainer.json.
	Dockerfile string
	// DockerfileKeyOffset is the byte offset of the property naming the Dockerfile, whichever of the
	// two names it.
	DockerfileKeyOffset int
	// DockerfileValueOffset is the byte offset of that property's value.
	DockerfileValueOffset int
	// Args are the arguments passed to the build, nil when it declares none.
	Args map[string]string
	// Target is the stage the build stops at, empty when it declares none.
	Target string
}

// build returns the Dockerfile build obj declares, or nil when it declares no Dockerfile — the
// options alone build nothing.
//
// The specification defines two mutually exclusive ways to name the Dockerfile, the top-level
// "dockerFile" and the nested "build.dockerfile"; the top-level one wins, as the reference
// implementation prefers it (getDockerfile: 'dockerFile' in config ? config.dockerFile :
// config.build.dockerfile). The options are always the "build" object's, which the legacy top-level
// form carries alongside it.
func build(obj *hujson.Object) *BuildDef {
	path, keyOffset, valueOffset, ok := dockerfilePath(obj)
	if !ok {
		return nil
	}
	config := &BuildDef{Dockerfile: path, DockerfileKeyOffset: keyOffset, DockerfileValueOffset: valueOffset}
	build := buildObject(obj)
	if build == nil {
		return config
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
	return config
}

// dockerfilePath returns the path the configuration names its Dockerfile by, in either form, with
// the byte offset of the naming property.
func dockerfilePath(obj *hujson.Object) (path string, keyOffset, valueOffset int, ok bool) {
	if m := memberNamed(obj, "dockerFile"); m != nil {
		if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
			return lit.String(), m.Name.StartOffset, m.Value.StartOffset, true
		}
	}
	if build := buildObject(obj); build != nil {
		if m := memberNamed(build, "dockerfile"); m != nil {
			if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
				return lit.String(), m.Name.StartOffset, m.Value.StartOffset, true
			}
		}
	}
	return "", 0, 0, false
}

// ComposeDef is the Compose declaration of a devcontainer.json: the files it names and the
// service the dev container runs in. The two are read together because neither settles anything
// alone — a configuration that names files but no service says which Compose project it belongs to
// and not which container it is.
type ComposeDef struct {
	// Files are the Compose file paths, in the order declared, later ones overriding earlier ones.
	// It is empty when the declaration names no readable path.
	Files []string
	// FilesKeyOffset is the byte offset of the "dockerComposeFile" property name.
	FilesKeyOffset int
	// FilesValueOffset is the byte offset of that property's value.
	FilesValueOffset int
	// Service is the service the dev container runs in, empty when the configuration names none or
	// names it as something other than a string.
	Service string
}

// Usable reports whether the declaration settles a container: a service to attach to, and at least
// one file that may define it. A caller that resolves the container reads nothing for a declaration
// that does not, and must not fall back to another form on it.
func (c ComposeDef) Usable() bool {
	return len(c.Files) > 0 && c.Service != ""
}

// compose returns the Compose declaration of obj, or nil when "dockerComposeFile" is not there —
// which is what tells a Compose-based configuration from one that builds or pulls an image. A
// declaration that is there settles a container only if [ComposeDef.Usable]; that it settles none
// does not make the configuration any less Compose-based.
//
// "dockerComposeFile" is a single path or an array of them. A value, or an element, that is not a
// string contributes no path, so a declaration can be there with no path to read.
func compose(obj *hujson.Object) *ComposeDef {
	m := memberNamed(obj, "dockerComposeFile")
	if m == nil {
		return nil
	}
	var config ComposeDef
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
	config.FilesKeyOffset, config.FilesValueOffset = m.Name.StartOffset, m.Value.StartOffset

	if m := memberNamed(obj, "service"); m != nil {
		if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
			config.Service = lit.String()
		}
	}
	return &config
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
