package rules

import (
	"path"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
	"go.yaml.in/yaml/v3"
)

// composeSource is what the Compose service a dev container runs is made from: the image it pulls,
// or the build that produces one. At most one is set; neither is when the service names an image
// this cannot resolve (see [composeServiceSource]).
type composeSource struct {
	image string
	build *composeBuild
}

// composeBuild is a Compose service's "build", reduced to what a rule reading its Dockerfile needs.
// Exactly one of dockerfile and inline is set.
type composeBuild struct {
	// dockerfile is the Dockerfile's path, relative to the directory being linted.
	dockerfile string
	// inline is the Dockerfile's content, for a build that gives it as "dockerfile_inline".
	inline string
	// target is the stage "target" names, empty when it names none.
	target string
}

// composeFilePaths returns the Compose file paths obj declares, with the byte offset of the value
// declaring them. The property is a single path or an array of paths, later ones overriding earlier
// ones; the merge reads the same property in feature's composeFilePaths.
func composeFilePaths(obj *hujson.Object) (paths []string, offset int, ok bool) {
	m := memberNamed(obj, "dockerComposeFile")
	if m == nil {
		return nil, 0, false
	}
	switch v := m.Value.Value.(type) {
	case hujson.Literal:
		if v.Kind() != '"' {
			return nil, 0, false
		}
		paths = []string{v.String()}
	case *hujson.Array:
		for _, e := range v.Elements {
			lit, isLit := e.Value.(hujson.Literal)
			if !isLit || lit.Kind() != '"' {
				return nil, 0, false
			}
			paths = append(paths, lit.String())
		}
	default:
		return nil, 0, false
	}
	return paths, m.Value.StartOffset, true
}

// composeService is the part of a Compose service definition that says what the service runs, or
// that the definition is not all in this file.
type composeService struct {
	Image string `yaml:"image"`
	// Build is untyped because Compose writes it two ways: the build context as a string, or an
	// object of build options. See [composeServiceBuild].
	Build   any `yaml:"build"`
	Extends any `yaml:"extends"`
}

// composeDoc is the part of a Compose file that defines the services, or pulls definitions in from
// files of its own.
type composeDoc struct {
	Services map[string]composeService `yaml:"services"`
	Include  any                       `yaml:"include"`
}

// composeServiceSource returns what the named Compose service is made from, reading the files at
// paths in the order they are declared, each later one overriding the earlier ones as Compose merges
// them.
//
// This reads the declared files and nothing else, which is narrower than the resolution the merge
// performs through compose-go (see feature's loadComposeService: it applies "extends" and "include"
// and interpolates variables, reading files outside the linted directory and an environment a rule
// does not have). ok is therefore false for everything this cannot settle from the files
// themselves, so that what it does report is what the full resolution would report too:
//
//   - a file that cannot be read (see [readConfigFile]) or does not parse;
//   - a file declaring "include", or a service declaring "extends", either of which can define or
//     override the service from a file not named here;
//   - a service none of the files defines;
//   - a service more than one file gives a "build", which Compose merges option by option.
//
// A service whose image or build context is written with a variable resolves to neither an image nor
// a build: the value comes from the environment. The same is true of a build context naming a remote
// repository, which is no path in the linted directory.
func composeServiceSource(dir linter.Dir, paths []string, service string) (composeSource, bool) {
	var src composeSource
	var found, built bool
	for _, p := range paths {
		data, ok := readConfigFile(dir, p)
		if !ok {
			return composeSource{}, false
		}
		var doc composeDoc
		if err := yaml.Unmarshal(data, &doc); err != nil || doc.Include != nil {
			return composeSource{}, false
		}
		svc, ok := doc.Services[service]
		if !ok {
			continue
		}
		found = true
		if svc.Extends != nil {
			return composeSource{}, false
		}
		if svc.Image != "" {
			src.image = svc.Image
		}
		if svc.Build == nil {
			continue
		}
		if built {
			return composeSource{}, false
		}
		built = true
		src.build = composeServiceBuild(svc.Build, path.Dir(p))
	}
	if !found {
		return composeSource{}, false
	}
	if src.build != nil {
		// The "image" of a service that builds names what the build produces, not what it starts
		// from, so the build is the whole answer.
		return composeSource{build: src.build}, true
	}
	// Both "${VAR}" and the bare "$VAR" Compose accepts leave the image unresolved here.
	if strings.Contains(src.image, "$") {
		src.image = ""
	}
	return src, true
}

// composeServiceBuild reads a service's "build" in either of the forms Compose writes it, resolving
// the Dockerfile against baseDir, the directory of the Compose file declaring the build, as Compose
// resolves it against the file it is written in. It returns nil for a build whose Dockerfile is not
// a path in the linted directory.
//
// The Dockerfile defaults to "Dockerfile" in the build context, and the context to the Compose
// file's own directory.
func composeServiceBuild(value any, baseDir string) *composeBuild {
	var context, dockerfile, inline, target string
	switch v := value.(type) {
	case string:
		// The short form is the build context alone.
		context = v
	case map[string]any:
		context, _ = v["context"].(string)
		dockerfile, _ = v["dockerfile"].(string)
		inline, _ = v["dockerfile_inline"].(string)
		target, _ = v["target"].(string)
	default:
		return nil
	}

	if inline != "" {
		return &composeBuild{inline: inline, target: target}
	}
	// A context naming a remote repository, or one written as a variable, is no path the Dockerfile
	// can be read through.
	if strings.Contains(context, "://") || strings.Contains(context, "$") {
		return nil
	}
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	if strings.Contains(dockerfile, "$") {
		return nil
	}
	return &composeBuild{dockerfile: path.Join(baseDir, context, dockerfile), target: target}
}
