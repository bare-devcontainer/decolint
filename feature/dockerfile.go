package feature

import (
	"context"
	"fmt"

	"github.com/moby/buildkit/client/llb/sourceresolver"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	"github.com/moby/buildkit/frontend/dockerui"
	"github.com/opencontainers/go-digest"
)

// FetchDockerfileMetadata computes the Dev Container metadata entries the image built from
// dockerfile would carry in its "devcontainer.metadata" label: the value set by the Dockerfile's
// own LABEL instructions, or, absent one, the value inherited from the base image named by FROM,
// whose config is fetched through the registry. buildArgs and target are the "args" and "target"
// properties of the devcontainer.json "build" object; they select the built stage and resolve ARG
// references, including in FROM. labels are applied onto the built image after its LABEL
// instructions, as "docker build --label" does, so a "devcontainer.metadata" label there overrides
// the Dockerfile's own. It returns no entries when the built image would carry no such label; a
// Dockerfile that cannot be parsed or whose base image cannot be fetched is an error.
func (f *Fetcher) FetchDockerfileMetadata(ctx context.Context, dockerfile []byte, buildArgs, labels map[string]string, target string) ([]*Metadata, error) {
	res, err := dockerfile2llb.Dockerfile2LLB(ctx, dockerfile, dockerfile2llb.ConvertOpt{
		Config: dockerui.Config{
			BuildArgs: buildArgs,
			Labels:    labels,
			Target:    target,
		},
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
