package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bare-devcontainer/decolint/containerdef"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/template"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/tailscale/hujson"
)

// composeContributors returns the metadata contributors of the base image reached through
// "dockerComposeFile" and "service" of root: the image the named service's "build" would produce,
// or, absent one, the image its "image" names. The Compose files are interpolated with localEnv as
// the environment (see [loadComposeService]). declared reports whether root declares
// "dockerComposeFile" at all, which [baseImageContributors] uses to choose the base-image form. A
// declaration with a missing "service" property (which a lint rule already flags) contributes
// nothing.
//
// Unlike the rest of the merge, Compose resolution reads from the real filesystem rather than
// through fsRoot: the reference implementation runs "docker compose config", which resolves
// "extends" and "include" against files that commonly sit outside the configuration directory (a
// compose file named "../docker-compose.yml", or a service that extends a root-level file), so the
// fsRoot confinement is deliberately not applied here.
func composeContributors(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir string, localEnv map[string]string, root *hujson.Value) ([]*contributor, bool, error) {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, false, nil
	}
	compose, declared := containerdef.Compose(obj)
	if !declared {
		return nil, false, nil
	}
	if !compose.Usable() {
		return nil, true, nil
	}
	paths, service, anchor := compose.Files, compose.Service, compose.FilesDecl.KeyOffset
	// The Compose files resolve relative to the referencing devcontainer.json's directory on the real
	// filesystem, as "docker compose" itself would. The directory is made absolute so that compose-go
	// resolves the build context to an absolute path too, which composeDisplayRef relies on.
	baseDir, err := filepath.Abs(filepath.Join(fsRoot.Name(), configDir))
	if err != nil {
		return nil, true, fmt.Errorf("resolve %s: %w", configDir, err)
	}
	svc, err := loadComposeService(ctx, baseDir, paths, service, localEnv)
	if err != nil {
		return nil, true, err
	}
	entries, ref, err := composeServiceMetadata(ctx, f, baseDir, paths[0], svc)
	if err != nil {
		return nil, true, err
	}
	contribs := make([]*contributor, 0, len(entries))
	for _, md := range entries {
		contribs = append(contribs, &contributor{ref: ref, anchor: anchor, md: md})
	}
	return contribs, true, nil
}

// loadComposeService reads each Compose file at baseDir and returns the named service resolved by
// compose-go, merged across all of them per the Compose specification, with "extends" and "include"
// resolved as "docker compose config" would. "${...}" variables are interpolated from env as
// "docker compose config" would: an unset variable resolves to its declared default ("${VAR:-def}")
// or the empty string, and a "${VAR:?}" requirement on an unset variable is an error. The
// COMPOSE_FILE variable, profiles, and ".env" files are not applied. A missing or unparsable file
// (including one an "extends" or "include" names), or a service found in none of them, is an error.
func loadComposeService(ctx context.Context, baseDir string, paths []string, service string, env map[string]string) (*types.ServiceConfig, error) {
	files := make([]types.ConfigFile, 0, len(paths))
	for _, p := range paths {
		files = append(files, types.ConfigFile{Filename: filepath.Join(baseDir, p)})
	}
	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		WorkingDir:  filepath.Dir(filepath.Join(baseDir, paths[0])),
		ConfigFiles: files,
		Environment: types.Mapping(env),
	}, func(o *loader.Options) {
		o.SkipValidation = true
		o.SkipNormalization = true
		o.SkipConsistencyCheck = true
		o.SkipDefaultValues = true
		// The ".env" files docker compose reads are the only environment source decolint omits; the
		// service "environment:" passthrough this resolves needs no lint-time value.
		o.SkipResolveEnvironment = true
		// Resolve a service's build context to an absolute path against the file that declares it, so
		// a build inherited through "extends" from another directory reads its Dockerfile correctly.
		o.ResolvePaths = true
		// An unset variable resolves to the empty string by design; the reference implementation's
		// per-variable warning is not decolint's output channel, so suppress it.
		o.Interpolate.Substitute = func(s string, m template.Mapping) (string, error) {
			return template.SubstituteWithOptions(s, m, template.WithoutLogging)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", strings.Join(paths, ", "), err)
	}
	svc, ok := project.Services[service]
	if !ok {
		return nil, fmt.Errorf("service %q not found in %s", service, strings.Join(paths, ", "))
	}
	// A passthrough build arg (a bare "- NAME" with no value) takes its value from the environment,
	// as "docker compose build" would; one absent from env stays unset so the Dockerfile's ARG
	// default applies. Interpolation does not cover these: they are Compose environment lookups, not
	// "${...}" substitutions.
	if svc.Build != nil {
		svc.Build.Args = svc.Build.Args.Resolve(types.Mapping(env).Resolve)
	}
	return &svc, nil
}

// composeServiceMetadata fetches the metadata entries of svc's base image, along with the
// reference they were reached through (the Dockerfile path or the image name). A "build" resolves
// relative to firstComposePath, the first declared Compose file, matching the reference
// implementation's default project directory. A service declaring neither a resolvable "build" nor
// a resolvable "image" contributes nothing.
func composeServiceMetadata(ctx context.Context, f *Fetcher, baseDir, firstComposePath string, svc *types.ServiceConfig) ([]*Metadata, string, error) {
	if svc.Build != nil {
		return composeBuildMetadata(ctx, f, baseDir, firstComposePath, svc.Build)
	}
	if svc.Image == "" {
		return nil, "", nil
	}
	entries, err := f.FetchImageMetadata(ctx, svc.Image)
	return entries, svc.Image, err
}

// composeBuildMetadata fetches the metadata the image built from build would carry. The Dockerfile
// (default "Dockerfile", or the inline "dockerfile_inline" content) resolves relative to the
// context (default: the first Compose file's directory), honoring "args" and "target". A
// "devcontainer.metadata" entry in the build "labels" is forwarded to override the Dockerfile's own
// (see [Fetcher.FetchDockerfileMetadata]). A passthrough arg still absent from the environment (see
// [loadComposeService]) is dropped so the Dockerfile's ARG default applies.
func composeBuildMetadata(ctx context.Context, f *Fetcher, baseDir, firstComposePath string, build *types.BuildConfig) ([]*Metadata, string, error) {
	var labels map[string]string
	if v, ok := build.Labels[imageMetadataLabel]; ok {
		labels = map[string]string{imageMetadataLabel: v}
	}
	var args map[string]string
	for k, v := range build.Args {
		if v == nil {
			continue
		}
		if args == nil {
			args = map[string]string{}
		}
		args[k] = *v
	}

	if build.DockerfileInline != "" {
		entries, err := f.FetchDockerfileMetadata(ctx, []byte(build.DockerfileInline), args, labels, build.Target)
		if err != nil {
			return nil, "", fmt.Errorf("build dockerfile_inline: %w", err)
		}
		return entries, "dockerfile_inline", nil
	}

	// build.Context is the absolute context directory compose-go resolved (see ResolvePaths); it is
	// empty only when the build declares no context, defaulting to the first Compose file's directory.
	context := build.Context
	if context == "" {
		context = filepath.Dir(filepath.Join(baseDir, firstComposePath))
	}
	dockerfile := build.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	dockerfilePath := dockerfile
	if !filepath.IsAbs(dockerfilePath) {
		dockerfilePath = filepath.Join(context, dockerfile)
	}
	src, err := readFileLimited(dockerfilePath, maxDockerfileBytes)
	if err != nil {
		return nil, "", err
	}
	ref := composeDisplayRef(baseDir, dockerfilePath)
	entries, err := f.FetchDockerfileMetadata(ctx, src, args, labels, build.Target)
	if err != nil {
		return nil, "", fmt.Errorf("build %s: %w", ref, err)
	}
	return entries, ref, nil
}

// composeDisplayRef renders a Dockerfile path for display: relative to the configuration directory
// when it lies within it, and absolute otherwise (a build context reached through "extends" or a
// "../" Compose file may sit outside it).
//
// It is deliberately not relative to the working directory: the reference identifies a contributor
// within the merged configuration (see [contributor.displayID]), so it must not depend on where
// decolint ran.
func composeDisplayRef(baseDir, path string) string {
	if rel, err := filepath.Rel(baseDir, path); err == nil && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return path
}

// readFileLimited reads the file at path from the real filesystem, rejecting a file larger than
// maxBytes before reading it into memory. It is used for the Compose build path, which resolves
// against files that may sit outside the configuration's fsRoot boundary.
func readFileLimited(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxBytes)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return src, nil
}
