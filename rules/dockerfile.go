package rules

import (
	"bytes"
	"slices"
	"strconv"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	dflinter "github.com/moby/buildkit/frontend/dockerfile/linter"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// dockerfilePulledImages returns the images a build of the Dockerfile in src pulls when target is
// built: the one each stage's FROM builds on, and the ones its COPY and RUN --mount instructions
// read through "--from". They come in the order the instructions name them, one entry per
// instruction. An empty target builds the last stage, as "docker build" does.
//
// Only the stages the build actually reaches are considered, since a stage nothing depends on is
// never built and its images never pulled. Within them, a reference naming another stage is left
// out, being no image at all, as are the references [isPulledImage] rejects.
//
// It returns nothing for a Dockerfile that does not parse, or a target it does not define, leaving
// a rule with nothing to report rather than a guess.
func dockerfilePulledImages(src []byte, target string) []string {
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
	for i := range stages {
		if !built[i] {
			continue
		}
		if _, isStage := stageBase(stages, i); !isStage && isPulledImage(stages[i].BaseName) {
			images = append(images, stages[i].BaseName)
		}
		for _, from := range stageFroms(stages, i) {
			if from.stage < 0 && isPulledImage(from.ref) {
				images = append(images, from.ref)
			}
		}
	}
	return images
}

// isPulledImage reports whether ref, a reference naming no stage, names an image the build pulls.
// Left out are:
//   - the empty reference;
//   - "scratch", the empty base, which BuildKit recognizes in that spelling alone;
//   - a reference containing a variable, whose value comes from "build.args" or an ARG default and
//     is not the linter's to resolve.
func isPulledImage(ref string) bool {
	return ref != "" && ref != "scratch" && !strings.Contains(ref, "$")
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
		// A target names a stage and never a position, and BuildKit lower-cases it before the
		// lookup, so "DEV" reaches the stage declared "AS dev".
		i, ok := stageNamed(stages, strings.ToLower(target))
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
		queue = append(queue, stageDeps(stages, i)...)
	}
	return built
}

// stageDeps returns the indexes of the stages the stage at i is built from: the one its FROM builds
// on, and the ones its instructions read through "--from", each only when it names a stage rather
// than an image.
func stageDeps(stages []instructions.Stage, i int) []int {
	var deps []int
	if j, ok := stageBase(stages, i); ok {
		deps = append(deps, j)
	}
	for _, from := range stageFroms(stages, i) {
		if from.stage >= 0 {
			deps = append(deps, from.stage)
		}
	}
	return deps
}

// stageFrom is a "--from" value of a COPY or a RUN --mount, resolved against the Dockerfile's
// stages: stage is the index of the stage it names, or -1 for a value naming an image, which the
// build pulls like a FROM base.
type stageFrom struct {
	ref   string
	stage int
}

// stageFroms returns the "--from" values the instructions of the stage at i read, in the order they
// are written. A value naming neither a stage nor an image — a COPY's position that is out of range,
// which fails the build — is left out.
//
// The two instructions resolve a value differently: a COPY's is a stage position when it parses as
// an integer, while a RUN --mount's is always a name. Both are matched against the stage names
// case-insensitively, and against every stage rather than only the earlier ones, since BuildKit
// resolves them once the whole Dockerfile is read.
func stageFroms(stages []instructions.Stage, i int) []stageFrom {
	byName := func(ref string) stageFrom {
		if j, ok := stageNamed(stages, strings.ToLower(ref)); ok {
			return stageFrom{ref: ref, stage: j}
		}
		return stageFrom{ref: ref, stage: -1}
	}

	var froms []stageFrom
	for _, cmd := range stages[i].Commands {
		switch c := cmd.(type) {
		case *instructions.CopyCommand:
			if c.From == "" {
				continue
			}
			if j, err := strconv.Atoi(c.From); err == nil {
				if j >= 0 && j < len(stages) {
					froms = append(froms, stageFrom{ref: c.From, stage: j})
				}
				continue
			}
			froms = append(froms, byName(c.From))
		case *instructions.RunCommand:
			for _, mount := range instructions.GetMounts(c) {
				if mount.From == "" {
					continue
				}
				froms = append(froms, byName(mount.From))
			}
		}
	}
	return froms
}

// stageBase returns the index of the stage the FROM of the stage at i builds on, and reports whether
// it names one rather than an image. BuildKit matches a base name against the stages declared before
// it only, and matches it as written against names the parser has already lower-cased — so
// "FROM Builder" after "AS builder" names an image, as its "repository name must be lowercase"
// failure shows.
func stageBase(stages []instructions.Stage, i int) (int, bool) {
	return stageNamed(stages[:i], stages[i].BaseName)
}

// stageNamed returns the index of the last stage named ref and reports whether one is. Several
// stages may share a name, which BuildKit only warns about, and it keeps one stage per name as it
// registers them in turn, so a reference reaches the last of them. A caller whose reference BuildKit
// lower-cases before the lookup passes it lower-cased; stage names need no folding, the parser
// having lower-cased them already. A name cannot begin with a digit, so no reference written as a
// position reaches a stage here.
func stageNamed(stages []instructions.Stage, ref string) (int, bool) {
	for i, stage := range slices.Backward(stages) {
		// A stage left unnamed has no name to be reached by, whatever ref is.
		if stage.Name != "" && stage.Name == ref {
			return i, true
		}
	}
	return 0, false
}
