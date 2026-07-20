package feature

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/ocitest"
)

func TestFetchDockerfileMetadata(t *testing.T) {
	t.Parallel()

	t.Run("reads LABEL instruction entries", func(t *testing.T) {
		t.Parallel()
		dockerfile := "FROM scratch\n" +
			`LABEL devcontainer.metadata='[{"id": "a", "privileged": true}, {"id": "b"}]'` + "\n"

		entries, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, "")
		if err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if len(entries) != 2 || entries[0].ID != "a" || entries[1].ID != "b" {
			t.Fatalf("entries = %+v, want ids [a b]", entries)
		}
		if entries[0].Root.Find("/privileged") == nil {
			t.Error("first entry lost its /privileged property")
		}
	})

	t.Run("inherits the FROM base image label", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{
			imageMetadataLabel: `[{"id": "from-base", "remoteUser": "vscode"}]`,
		}, false)
		dockerfile := fmt.Sprintf("FROM %s/base:1\nRUN true\n", host)

		entries, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, "")
		if err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if len(entries) != 1 || entries[0].ID != "from-base" {
			t.Fatalf("entries = %+v, want one entry from-base", entries)
		}
	})

	t.Run("LABEL instruction overrides the base image label", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{
			imageMetadataLabel: `[{"id": "from-base"}]`,
		}, false)
		dockerfile := fmt.Sprintf("FROM %s/base:1\n", host) +
			`LABEL devcontainer.metadata='[{"id": "from-dockerfile"}]'` + "\n"

		entries, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, "")
		if err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if len(entries) != 1 || entries[0].ID != "from-dockerfile" {
			t.Fatalf("entries = %+v, want one entry from-dockerfile", entries)
		}
	})

	t.Run("resolves a FROM through build args", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{imageMetadataLabel: `[{"id": "one"}]`}, false)
		ocitest.PushImage(t, host, "base", "2", map[string]string{imageMetadataLabel: `[{"id": "two"}]`}, false)
		dockerfile := fmt.Sprintf("ARG TAG=1\nFROM %s/base:${TAG}\n", host)

		entries, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), map[string]string{"TAG": "2"}, "")
		if err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if len(entries) != 1 || entries[0].ID != "two" {
			t.Fatalf("entries = %+v, want one entry two (the overridden tag)", entries)
		}
	})

	t.Run("unset build arg falls back to the ARG default", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{imageMetadataLabel: `[{"id": "one"}]`}, false)
		dockerfile := fmt.Sprintf("ARG TAG=1\nFROM %s/base:${TAG}\n", host)

		entries, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, "")
		if err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if len(entries) != 1 || entries[0].ID != "one" {
			t.Fatalf("entries = %+v, want one entry one (the default tag)", entries)
		}
	})

	t.Run("selects the target stage", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "dev", map[string]string{imageMetadataLabel: `[{"id": "dev"}]`}, false)
		ocitest.PushImage(t, host, "base", "prod", map[string]string{imageMetadataLabel: `[{"id": "prod"}]`}, false)
		dockerfile := fmt.Sprintf("FROM %s/base:dev AS dev\nFROM %s/base:prod AS prod\n", host, host)

		entries, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, "dev")
		if err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if len(entries) != 1 || entries[0].ID != "dev" {
			t.Fatalf("entries = %+v, want one entry dev (the target stage)", entries)
		}
	})

	t.Run("last stage inherits through earlier stages", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{
			imageMetadataLabel: `[{"id": "from-base"}]`,
		}, false)
		// The final stage builds on the intermediate one, so the base image's label reaches it
		// through the stage chain.
		dockerfile := fmt.Sprintf("FROM %s/base:1 AS intermediate\nFROM intermediate\nRUN true\n", host)

		entries, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, "")
		if err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if len(entries) != 1 || entries[0].ID != "from-base" {
			t.Fatalf("entries = %+v, want one entry from-base", entries)
		}
	})

	t.Run("image without the label yields nothing", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{"other": "value"}, false)
		dockerfile := fmt.Sprintf("FROM %s/base:1\n", host)

		entries, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, "")
		if err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if entries != nil {
			t.Errorf("entries = %+v, want nil", entries)
		}
	})

	t.Run("unfetchable base image is an error", func(t *testing.T) {
		t.Parallel()
		dockerfile := "FROM registry.invalid/app:1\n"
		_, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, "")
		if err == nil {
			t.Fatal("FetchDockerfileMetadata with an unreachable base image: got nil error")
		}
	})

	t.Run("unparsable Dockerfile is an error", func(t *testing.T) {
		t.Parallel()
		_, err := NewFetcher().FetchDockerfileMetadata(t.Context(), []byte("RUN true\n"), nil, "")
		if err == nil {
			t.Fatal("FetchDockerfileMetadata without a FROM: got nil error")
		}
	})

	t.Run("shares the image config cache with the image path", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{
			imageMetadataLabel: `[{"id": "a"}]`,
		}, false)
		dockerfile := fmt.Sprintf("FROM %s/base:1\n", host)

		var log strings.Builder
		f := NewFetcher(WithLogWriter(&log))
		if _, err := f.FetchImageMetadata(t.Context(), host+"/base:1"); err != nil {
			t.Fatalf("FetchImageMetadata: %v", err)
		}
		if _, err := f.FetchDockerfileMetadata(t.Context(), []byte(dockerfile), nil, ""); err != nil {
			t.Fatalf("FetchDockerfileMetadata: %v", err)
		}
		if got := strings.Count(log.String(), "Downloading image metadata"); got != 1 {
			t.Errorf("downloads = %d, want 1 (the config cache must be shared):\n%s", got, log.String())
		}
	})
}
