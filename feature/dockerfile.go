package feature

import (
	"context"
	"fmt"

	"github.com/moby/buildkit/client/llb/sourceresolver"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	"github.com/opencontainers/go-digest"
)

// FetchDockerfileMetadata computes the Dev Container metadata entries the image built from
// dockerfile would carry in its "devcontainer.metadata" label: the value set by its own LABEL
// instructions, or, absent one, the value inherited from the base image its FROM names, whose
// config is fetched through the registry. A "devcontainer.metadata" entry in labels overrides the
// Dockerfile's own, as "docker build --label" does. It returns no entries, and no error, when the
// built image would carry no such label.
func (f *Fetcher) FetchDockerfileMetadata(ctx context.Context, dockerfile []byte, buildArgs, labels map[string]string, target string) ([]*Metadata, error) {
	res, err := dockerfile2llb.Dockerfile2LLB(ctx, dockerfile, dockerfile2llb.ConvertOpt{
		BuildArgs:    buildArgs,
		Labels:       labels,
		Target:       target,
		MetaResolver: fetcherMetaResolver{f: f},
	})
	if err != nil {
		return nil, fmt.Errorf("resolve dockerfile: %w", err)
	}
	label, ok := res.Image.Config.Labels[imageMetadataLabel]
	if !ok {
		return nil, nil
	}
	return imageMetadataEntries([]byte(label)), nil
}

// fetcherMetaResolver adapts a Fetcher to buildkit's image metadata resolver interface, so the
// base image named by a Dockerfile's FROM is fetched (and cached) the same way the "image"
// property is. The requested platform is ignored: the "devcontainer.metadata" label is identical
// across the platforms of a multi-platform image (see imageManifest).
type fetcherMetaResolver struct {
	f *Fetcher
}

func (r fetcherMetaResolver) ResolveImageConfig(ctx context.Context, ref string, _ sourceresolver.Opt) (string, digest.Digest, []byte, error) {
	cfg, err := r.f.fetchImageConfig(ctx, ref)
	if err != nil {
		return "", "", nil, err
	}
	return ref, digest.Digest(cfg.manifestDigest), cfg.raw, nil
}
