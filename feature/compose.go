package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/tailscale/hujson"
)

// composeContributors returns the metadata contributors of the base image reached through
// "dockerComposeFile" and "service" of root: the image the named service's "build" would produce,
// or, absent one, the image its "image" names. declared reports whether root declares
// "dockerComposeFile" at all, so the caller falls back to "build" or "image" only when it does not,
// matching the reference implementation's branch order. A declaration that cannot be resolved at
// lint time (a variable substitution in a path, the service name, image, context, dockerfile, or
// target, or a missing "service" property, which a lint rule already flags) contributes nothing.
func composeContributors(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, root *hujson.Value) ([]*contributor, bool, error) {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, false, nil
	}
	paths, anchor, ok := composeFilePaths(obj)
	if !ok {
		return nil, false, nil
	}
	if len(paths) == 0 {
		return nil, true, nil
	}
	// Variable substitutions resolve at container creation time; linting cannot know their values,
	// so a declaration carrying one is skipped rather than rejected.
	if slices.ContainsFunc(paths, func(p string) bool { return strings.Contains(p, "${") }) {
		return nil, true, nil
	}
	service, ok := composeServiceName(obj)
	if !ok || strings.Contains(service, "${") {
		return nil, true, nil
	}
	svc, err := loadComposeService(ctx, fsRoot, configDir, paths, service)
	if err != nil {
		return nil, true, err
	}
	entries, ref, err := composeServiceMetadata(ctx, f, fsRoot, configDir, paths[0], svc)
	if err != nil {
		return nil, true, err
	}
	contribs := make([]*contributor, 0, len(entries))
	for _, md := range entries {
		contribs = append(contribs, &contributor{ref: ref, anchor: anchor, md: md})
	}
	return contribs, true, nil
}

// composeFilePaths returns the Compose file paths root declares, relative to the devcontainer.json
// directory, with the byte offset of the "dockerComposeFile" key. The property is a single path or
// an array of paths, later ones overriding earlier ones. ok reports whether the member exists at
// all; a present but unusable value returns ok=true with no paths, so the caller still treats
// Compose as declared and does not fall back to "build" or "image".
func composeFilePaths(obj *hujson.Object) (paths []string, anchor int, ok bool) {
	i := findMember(obj, "dockerComposeFile")
	if i < 0 {
		return nil, 0, false
	}
	anchor = obj.Members[i].Name.StartOffset
	switch v := obj.Members[i].Value.Value.(type) {
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
	return paths, anchor, true
}

// composeServiceName returns the "service" property of root; ok is false when it is absent or not
// a string.
func composeServiceName(obj *hujson.Object) (string, bool) {
	i := findMember(obj, "service")
	if i < 0 {
		return "", false
	}
	lit, ok := obj.Members[i].Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return "", false
	}
	return lit.String(), true
}

// loadComposeService reads each Compose file through fsRoot and returns the named service resolved
// by compose-go, merged across all of them per the Compose specification. Interpolation is skipped
// so a "${...}" variable is preserved verbatim for the caller to recognize as unresolvable at lint
// time; extends and includes, which would read files outside fsRoot, are skipped as well, leaving
// them unresolved like the reference implementation's other unsupported constructs. A missing or
// unparsable file, or a service found in none of them, is an error.
func loadComposeService(ctx context.Context, fsRoot *os.Root, configDir string, paths []string, service string) (*types.ServiceConfig, error) {
	files := make([]types.ConfigFile, 0, len(paths))
	for _, p := range paths {
		src, err := readBounded(fsRoot, filepath.Join(configDir, p), maxComposeFileBytes)
		if err != nil {
			return nil, err
		}
		files = append(files, types.ConfigFile{Filename: p, Content: src})
	}
	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: files,
		Environment: types.Mapping{},
	}, func(o *loader.Options) {
		o.SkipInterpolation = true
		o.SkipValidation = true
		o.SkipNormalization = true
		o.SkipConsistencyCheck = true
		o.SkipDefaultValues = true
		o.SkipResolveEnvironment = true
		o.SkipExtends = true
		o.SkipInclude = true
	})
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", strings.Join(paths, ", "), err)
	}
	svc, ok := project.Services[service]
	if !ok {
		return nil, fmt.Errorf("service %q not found in %s", service, strings.Join(paths, ", "))
	}
	return &svc, nil
}

// composeServiceMetadata fetches the metadata entries of svc's base image, along with the
// reference they were reached through (the Dockerfile path or the image name). A "build" resolves
// relative to firstComposePath, the first declared Compose file, matching the reference
// implementation's default project directory. A service declaring neither a resolvable "build" nor
// a resolvable "image" contributes nothing.
func composeServiceMetadata(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir, firstComposePath string, svc *types.ServiceConfig) ([]*Metadata, string, error) {
	if svc.Build != nil {
		return composeBuildMetadata(ctx, f, fsRoot, configDir, firstComposePath, svc.Build)
	}
	if svc.Image == "" || strings.Contains(svc.Image, "${") {
		return nil, "", nil
	}
	entries, err := f.FetchImageMetadata(ctx, svc.Image)
	return entries, svc.Image, err
}

// composeBuildMetadata fetches the metadata the image built from build would carry. The Dockerfile
// (default "Dockerfile", or the inline "dockerfile_inline" content) resolves relative to the
// context (default: the first Compose file's directory), honoring "args" and "target" like a
// devcontainer.json "build" object. A "devcontainer.metadata" entry in "labels" overrides the
// Dockerfile's own label, as "docker build --label" does; an unresolvable variable substitution
// skips the build, and an arg carrying one (or an environment passthrough, which has no value in
// the file) is dropped so the Dockerfile's ARG default applies.
func composeBuildMetadata(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir, firstComposePath string, build *types.BuildConfig) ([]*Metadata, string, error) {
	if strings.Contains(build.Context, "${") || strings.Contains(build.Dockerfile, "${") || strings.Contains(build.Target, "${") {
		return nil, "", nil
	}
	var labels map[string]string
	if v, ok := build.Labels[imageMetadataLabel]; ok {
		if strings.Contains(v, "${") {
			return nil, "", nil
		}
		labels = map[string]string{imageMetadataLabel: v}
	}
	var args map[string]string
	for k, v := range build.Args {
		if v == nil || strings.Contains(*v, "${") {
			continue
		}
		if args == nil {
			args = map[string]string{}
		}
		args[k] = *v
	}

	if build.DockerfileInline != "" {
		if strings.Contains(build.DockerfileInline, "${") {
			return nil, "", nil
		}
		entries, err := f.FetchDockerfileMetadata(ctx, []byte(build.DockerfileInline), args, labels, build.Target)
		if err != nil {
			return nil, "", fmt.Errorf("build dockerfile_inline: %w", err)
		}
		return entries, "dockerfile_inline", nil
	}

	dockerfile := build.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	path := filepath.Join(filepath.Dir(firstComposePath), build.Context, dockerfile)
	src, err := readBounded(fsRoot, filepath.Join(configDir, path), maxDockerfileBytes)
	if err != nil {
		return nil, "", err
	}
	entries, err := f.FetchDockerfileMetadata(ctx, src, args, labels, build.Target)
	if err != nil {
		return nil, "", fmt.Errorf("build %s: %w", path, err)
	}
	return entries, path, nil
}
