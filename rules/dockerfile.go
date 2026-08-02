package rules

import (
	"bytes"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/tailscale/hujson"
)

// dockerfileRef locates the Dockerfile a devcontainer.json builds from: its path as written, and
// the byte offset of the value declaring it, which is where a rule reporting the Dockerfile's
// contents anchors its findings.
//
// The specification defines two mutually exclusive forms, the top-level "dockerFile" and the nested
// "build.dockerfile". The top-level one is preferred, as the reference implementation prefers it;
// the merge resolves the same two the same way, in feature's dockerfilePath.
func dockerfileRef(obj *hujson.Object) (path string, offset int, ok bool) {
	if m := memberNamed(obj, "dockerFile"); m != nil {
		if lit, isLit := m.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
			return lit.String(), m.Value.StartOffset, true
		}
	}
	if m := memberNamed(obj, "build"); m != nil {
		if build, isObj := m.Value.Value.(*hujson.Object); isObj {
			if d := memberNamed(build, "dockerfile"); d != nil {
				if lit, isLit := d.Value.Value.(hujson.Literal); isLit && lit.Kind() == '"' {
					return lit.String(), d.Value.StartOffset, true
				}
			}
		}
	}
	return "", 0, false
}

// dockerfileBaseImages returns the images the Dockerfile in src builds from, in the order its FROM
// instructions name them, keeping a repeated image once per FROM. It returns nothing for a
// Dockerfile that does not parse, leaving a rule with nothing to report rather than a guess.
//
// Only FROMs naming an image outside the build are returned. Left out are:
//   - a reference to an earlier stage of the same Dockerfile, which is not an image at all;
//   - "scratch", the empty base;
//   - a reference containing a variable, whose value comes from "build.args" or an ARG default and
//     is not the linter's to resolve.
func dockerfileBaseImages(src []byte) []string {
	result, err := parser.Parse(bytes.NewReader(src))
	if err != nil {
		return nil
	}
	// The linter argument reports the lint warnings buildkit itself defines; a nil one turns them
	// off, which is what a caller reading the stages wants.
	stages, _, err := instructions.Parse(result.AST, nil)
	if err != nil {
		return nil
	}

	var images []string
	stageNames := map[string]struct{}{}
	for _, stage := range stages {
		base := stage.BaseName
		_, isStage := stageNames[strings.ToLower(base)]
		if base != "" && base != "scratch" && !isStage && !strings.Contains(base, "$") {
			images = append(images, base)
		}
		if stage.Name != "" {
			stageNames[strings.ToLower(stage.Name)] = struct{}{}
		}
	}
	return images
}

// dockerfileBuildImages returns the images the Dockerfile that obj, a devcontainer.json, declares
// builds from (see [dockerfileBaseImages]), along with the Dockerfile's path as written and the
// offset to anchor findings at. ok is false when obj declares no Dockerfile, or when the file
// cannot be read (see [readConfigFile]).
func dockerfileBuildImages(dir linter.Dir, obj *hujson.Object) (images []string, path string, offset int, ok bool) {
	path, offset, ok = dockerfileRef(obj)
	if !ok {
		return nil, "", 0, false
	}
	src, ok := readConfigFile(dir, path)
	if !ok {
		return nil, "", 0, false
	}
	return dockerfileBaseImages(src), path, offset, true
}
