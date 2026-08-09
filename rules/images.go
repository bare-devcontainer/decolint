package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/containerdef"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// pulledImage is an image a configuration pulls, wherever it is named.
type pulledImage struct {
	// ref is the image reference as written.
	ref string
	// source locates the image for a finding that continues with the reference, e.g.
	// `Dockerfile "Dockerfile": `. It is empty for the "image" property, which the reference alone
	// already names.
	source string
	// offset is the byte offset of the property the finding anchors at, which is the one the
	// devcontainer.json declares — the Dockerfile and the Compose file are not the linted file.
	offset int
}

// configImages returns every image a build of obj, a devcontainer.json, pulls: the one "image"
// names, the ones the Dockerfile it builds pulls, and, for a Compose-based configuration, the image
// its service runs or the ones the Dockerfile that service builds from pulls.
//
// Every form the configuration declares is read, not only the one the tooling would build, so that
// an unpinned image is reported wherever it is written; a configuration declaring more than one form
// is reported by [ConflictingContainerDef]. An image this cannot resolve is left out rather than
// guessed at; see [dockerfilePulledImages] and [composeServiceSource] for what each leaves behind.
func configImages(dir linter.Dir, obj *hujson.Object) []pulledImage {
	var images []pulledImage
	for def := range containerdef.Defs(obj) {
		switch def := def.(type) {
		case *containerdef.ImageDef:
			images = append(images, pulledImage{ref: def.Ref, offset: def.Decl.ValueOffset})
		case *containerdef.BuildDef:
			images = append(images, dockerfileImages(dir, def)...)
		case *containerdef.ComposeDef:
			images = append(images, composeImages(dir, def)...)
		}
	}
	return images
}

// dockerfileImages returns the images the Dockerfile def names pulls, anchored at the property
// naming it.
func dockerfileImages(dir linter.Dir, def *containerdef.BuildDef) []pulledImage {
	src, ok := readConfigFile(dir, def.Dockerfile)
	if !ok {
		return nil
	}
	images := dockerfilePulledImages(src, def.Args, def.Target)
	return locate(images, fmt.Sprintf("Dockerfile %q: ", def.Dockerfile), def.DockerfileDecl.ValueOffset)
}

// composeImages returns the images the Compose service the dev container runs pulls: the one it
// runs, or the ones the Dockerfile it builds from pulls.
func composeImages(dir linter.Dir, def *containerdef.ComposeDef) []pulledImage {
	if !def.Usable() {
		return nil
	}
	service, offset := def.Service, def.FilesDecl.ValueOffset
	source, ok := composeServiceSource(dir, def.Files, service)
	if !ok {
		return nil
	}
	if source.build == nil {
		if source.image == "" {
			return nil
		}
		return []pulledImage{{ref: source.image, source: fmt.Sprintf("compose service %q: ", service), offset: offset}}
	}

	src := []byte(source.build.inline)
	where := fmt.Sprintf("compose service %q inline Dockerfile: ", service)
	if source.build.dockerfile != "" {
		if src, ok = readConfigFile(dir, source.build.dockerfile); !ok {
			return nil
		}
		where = fmt.Sprintf("Dockerfile %q: ", source.build.dockerfile)
	}
	return locate(dockerfilePulledImages(src, source.build.args, source.build.target), where, offset)
}

// locate pairs each reference with where it was found and the offset to report it at.
func locate(refs []string, source string, offset int) []pulledImage {
	images := make([]pulledImage, 0, len(refs))
	for _, ref := range refs {
		images = append(images, pulledImage{ref: ref, source: source, offset: offset})
	}
	return images
}
