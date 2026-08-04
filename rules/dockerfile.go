package rules

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	dflinter "github.com/moby/buildkit/frontend/dockerfile/linter"
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

// buildTarget returns the stage "build.target" names, or "" when the configuration names none and
// the build produces the Dockerfile's last stage.
func buildTarget(obj *hujson.Object) string {
	m := memberNamed(obj, "build")
	if m == nil {
		return ""
	}
	build, ok := m.Value.Value.(*hujson.Object)
	if !ok {
		return ""
	}
	target, _ := stringMember(build, "target")
	return target
}

// dockerfileBaseImages returns the images the Dockerfile in src builds from when target is built,
// in the order its FROM instructions name them, keeping a repeated image once per FROM. An empty
// target builds the last stage, as "docker build" does.
//
// Only the stages the build actually reaches are considered, since a stage nothing depends on is
// never built and its base image never pulled. Of those, only FROMs naming an image outside the
// build are returned. Left out are:
//   - a reference to an earlier stage of the same Dockerfile, which is not an image at all;
//   - "scratch", the empty base;
//   - a reference containing a variable, whose value comes from "build.args" or an ARG default and
//     is not the linter's to resolve.
//
// It returns nothing for a Dockerfile that does not parse, or a target it does not define, leaving
// a rule with nothing to report rather than a guess.
func dockerfileBaseImages(src []byte, target string) []string {
	result, err := parser.Parse(bytes.NewReader(src))
	if err != nil {
		return nil
	}
	// A Dockerfile may configure buildkit's own linter through a "# check=..." comment, which is
	// merged onto the one passed here — a nil one is dereferenced, so pass a linter that reports
	// nothing instead. Its zero Config leaves Warn nil, which is what turns the warnings off.
	stages, _, err := instructions.Parse(result.AST, dflinter.New(&dflinter.Config{}))
	if err != nil {
		return nil
	}

	built := builtStages(stages, target)
	var images []string
	for i, stage := range stages {
		if !built[i] {
			continue
		}
		base := stage.BaseName
		if base == "" || base == "scratch" || strings.Contains(base, "$") {
			continue
		}
		if j, isStage := stageIndex(stages, base); isStage && j < i {
			continue
		}
		images = append(images, base)
	}
	return images
}

// builtStages returns the indexes of the stages a build of target reaches: the target stage itself,
// the stages it builds on, and the ones it copies from, transitively. An empty target starts from
// the last stage, as "docker build" does. It returns nothing when target names no stage, since such
// a build does not run at all.
func builtStages(stages []instructions.Stage, target string) map[int]bool {
	if len(stages) == 0 {
		return nil
	}
	start := len(stages) - 1
	if target != "" {
		i, ok := stageIndex(stages, target)
		if !ok {
			return nil
		}
		start = i
	}

	built := map[int]bool{}
	for queue := []int{start}; len(queue) > 0; queue = queue[1:] {
		i := queue[0]
		if built[i] {
			continue
		}
		built[i] = true
		for _, dep := range stageDeps(stages, i) {
			queue = append(queue, dep)
		}
	}
	return built
}

// stageDeps returns the indexes of the stages the stage at i is built from: the one its FROM names,
// and the ones its instructions read through "--from", each only when it is a stage defined earlier
// rather than an image.
func stageDeps(stages []instructions.Stage, i int) []int {
	var deps []int
	add := func(ref string) {
		if j, ok := stageIndex(stages, ref); ok && j < i {
			deps = append(deps, j)
		}
	}

	add(stages[i].BaseName)
	for _, cmd := range stages[i].Commands {
		if copyCmd, ok := cmd.(*instructions.CopyCommand); ok {
			add(copyCmd.From)
		}
		if runCmd, ok := cmd.(*instructions.RunCommand); ok {
			for _, mount := range instructions.GetMounts(runCmd) {
				add(mount.From)
			}
		}
	}
	return deps
}

// stageIndex returns the index of the stage ref names, by its name or by its position, and reports
// whether it names one at all. Stage names are matched case-insensitively, as the Dockerfile parser
// matches them.
func stageIndex(stages []instructions.Stage, ref string) (int, bool) {
	if ref == "" {
		return 0, false
	}
	for i, stage := range stages {
		if stage.Name != "" && strings.EqualFold(stage.Name, ref) {
			return i, true
		}
	}
	if i, err := strconv.Atoi(ref); err == nil && i >= 0 && i < len(stages) {
		return i, true
	}
	return 0, false
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
	return dockerfileBaseImages(src, buildTarget(obj)), path, offset, true
}
