package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/tailscale/hujson"
)

// composeFile is the subset of a Compose file the merge consumes. Runtime service properties,
// including "labels" under the service itself, are intentionally not modeled: they configure the
// container, not the image, so they never carry image metadata (the reference implementation
// computes the effective configuration from image labels before the container exists).
type composeFile struct {
	Services map[string]composeRawService `yaml:"services"`
}

// composeRawService is one service as declared in a single Compose file. Build is untyped because
// Compose accepts a string (a context shorthand) or a mapping, whose "args" and "labels" in turn
// accept a mapping or a list of "KEY=VALUE" strings; merge folds all of these into composeService.
type composeRawService struct {
	Image string `yaml:"image"`
	Build any    `yaml:"build"`
}

// composeService is a service's image-relevant fields, normalized and merged across the declared
// Compose files.
type composeService struct {
	image string
	// hasBuild reports whether any file declared "build", which takes precedence over image,
	// matching the reference implementation.
	hasBuild   bool
	context    string
	dockerfile string
	target     string
	args       map[string]string
	labels     map[string]string
}

// merge folds raw's declared fields into svc, applying Compose's per-service merge semantics: a
// scalar overrides the value an earlier file declared, and "args" and "labels" merge key-wise. A
// string "build" sets the context, the normalization Compose itself applies before merging, so a
// later mapping form extends it rather than replacing it. A "build" of any other type, including
// null, is treated as undeclared.
func (svc *composeService) merge(raw composeRawService) {
	if raw.Image != "" {
		svc.image = raw.Image
	}
	switch b := raw.Build.(type) {
	case string:
		svc.hasBuild = true
		svc.context = b
	case map[string]any:
		svc.hasBuild = true
		if s, ok := b["context"].(string); ok && s != "" {
			svc.context = s
		}
		if s, ok := b["dockerfile"].(string); ok && s != "" {
			svc.dockerfile = s
		}
		if s, ok := b["target"].(string); ok && s != "" {
			svc.target = s
		}
		svc.args = mergeStringMap(svc.args, composeStringMap(b["args"]))
		svc.labels = mergeStringMap(svc.labels, composeStringMap(b["labels"]))
	}
}

// mergeStringMap overlays src onto dst key-wise, allocating dst only when there is something to
// merge.
func mergeStringMap(dst, src map[string]string) map[string]string {
	for k, v := range src {
		if dst == nil {
			dst = map[string]string{}
		}
		dst[k] = v
	}
	return dst
}

// composeStringMap converts a Compose map-or-list value (a "KEY: value" mapping or a
// ["KEY=value", ...] list) into a map. A list entry without "=" requests environment passthrough,
// which is unknowable at lint time, so it is dropped like a "${...}" value; a non-string entry or
// mapping value is ignored.
func composeStringMap(v any) map[string]string {
	var m map[string]string
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok {
				m = mergeStringMap(m, map[string]string{k: s})
			}
		}
	case []any:
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				continue
			}
			if k, val, found := strings.Cut(s, "="); found {
				m = mergeStringMap(m, map[string]string{k: val})
			}
		}
	}
	return m
}

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
	svc, err := loadComposeService(fsRoot, configDir, paths, service)
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

// loadComposeService reads each Compose file through fsRoot and returns the named service merged
// across all of them, in declaration order. A missing or unparsable file, or a service found in
// none of them, is an error.
func loadComposeService(fsRoot *os.Root, configDir string, paths []string, service string) (*composeService, error) {
	svc := &composeService{}
	found := false
	for _, p := range paths {
		src, err := readBounded(fsRoot, filepath.Join(configDir, p), maxComposeFileBytes)
		if err != nil {
			return nil, err
		}
		var file composeFile
		if err := yaml.Unmarshal(src, &file); err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		raw, ok := file.Services[service]
		if !ok {
			continue
		}
		found = true
		svc.merge(raw)
	}
	if !found {
		return nil, fmt.Errorf("service %q not found in %s", service, strings.Join(paths, ", "))
	}
	return svc, nil
}

// composeServiceMetadata fetches the metadata entries of svc's base image, along with the
// reference they were reached through (the Dockerfile path or the image name). A "build" resolves
// relative to firstComposePath, the first declared Compose file, matching the reference
// implementation's default project directory. A service declaring neither a resolvable "build" nor
// a resolvable "image" contributes nothing.
func composeServiceMetadata(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir, firstComposePath string, svc *composeService) ([]*Metadata, string, error) {
	if svc.hasBuild {
		return composeBuildMetadata(ctx, f, fsRoot, configDir, firstComposePath, svc)
	}
	if svc.image == "" || strings.Contains(svc.image, "${") {
		return nil, "", nil
	}
	entries, err := f.FetchImageMetadata(ctx, svc.image)
	return entries, svc.image, err
}

// composeBuildMetadata fetches the metadata the image built from svc's "build" would carry. The
// Dockerfile (default "Dockerfile") resolves relative to the context (default: the first Compose
// file's directory), honoring "args" and "target" like a devcontainer.json "build" object. A
// "devcontainer.metadata" entry in "labels" overrides the Dockerfile's own label, as
// "docker build --label" does; an unresolvable variable substitution skips the build, and an arg
// carrying one is dropped so the Dockerfile's ARG default applies.
func composeBuildMetadata(ctx context.Context, f *Fetcher, fsRoot *os.Root, configDir, firstComposePath string, svc *composeService) ([]*Metadata, string, error) {
	if strings.Contains(svc.context, "${") || strings.Contains(svc.dockerfile, "${") || strings.Contains(svc.target, "${") {
		return nil, "", nil
	}
	var labels map[string]string
	if v, ok := svc.labels[imageMetadataLabel]; ok {
		if strings.Contains(v, "${") {
			return nil, "", nil
		}
		labels = map[string]string{imageMetadataLabel: v}
	}
	var args map[string]string
	for k, v := range svc.args {
		if strings.Contains(v, "${") {
			continue
		}
		args = mergeStringMap(args, map[string]string{k: v})
	}
	dockerfile := svc.dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	path := filepath.Join(filepath.Dir(firstComposePath), svc.context, dockerfile)
	src, err := readBounded(fsRoot, filepath.Join(configDir, path), maxDockerfileBytes)
	if err != nil {
		return nil, "", err
	}
	entries, err := f.FetchDockerfileMetadata(ctx, src, args, labels, svc.target)
	if err != nil {
		return nil, "", fmt.Errorf("build %s: %w", path, err)
	}
	return entries, path, nil
}
