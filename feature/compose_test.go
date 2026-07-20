package feature

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/ocitest"
	"github.com/google/go-cmp/cmp"
	"github.com/tailscale/hujson"
)

func TestComposeStringMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   any
		want map[string]string
	}{
		{"map form", map[string]any{"A": "1", "B": "2"}, map[string]string{"A": "1", "B": "2"}},
		{"map non-string value dropped", map[string]any{"A": uint64(1), "B": "2"}, map[string]string{"B": "2"}},
		{"list form", []any{"A=1", "B=2=3"}, map[string]string{"A": "1", "B": "2=3"}},
		{"list entry without = dropped", []any{"A=1", "PASSTHROUGH"}, map[string]string{"A": "1"}},
		{"list non-string entry dropped", []any{uint64(1), "A=1"}, map[string]string{"A": "1"}},
		{"nil", nil, nil},
		{"unsupported type", "A=1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, composeStringMap(tt.in)); diff != "" {
				t.Errorf("composeStringMap(%v) mismatch (-want +got):\n%s", tt.in, diff)
			}
		})
	}
}

func TestLoadComposeService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string]string
		want  composeService
	}{
		{
			"image service",
			map[string]string{"a.yml": "services:\n  app:\n    image: registry.invalid/app:1\n"},
			composeService{image: "registry.invalid/app:1"},
		},
		{
			"build mapping",
			map[string]string{"a.yml": "services:\n  app:\n    build:\n      context: ctx\n      dockerfile: Custom\n      target: dev\n      args:\n        A: \"1\"\n      labels:\n        L: v\n"},
			composeService{hasBuild: true, context: "ctx", dockerfile: "Custom", target: "dev", args: map[string]string{"A": "1"}, labels: map[string]string{"L": "v"}},
		},
		{
			"build string shorthand sets the context",
			map[string]string{"a.yml": "services:\n  app:\n    build: ctx\n"},
			composeService{hasBuild: true, context: "ctx"},
		},
		{
			"null build is undeclared",
			map[string]string{"a.yml": "services:\n  app:\n    image: registry.invalid/app:1\n    build:\n"},
			composeService{image: "registry.invalid/app:1"},
		},
		{
			"later file overrides scalars and merges args key-wise",
			map[string]string{
				"a.yml": "services:\n  app:\n    image: registry.invalid/app:1\n    build:\n      dockerfile: A\n      args:\n        A: \"1\"\n        B: \"1\"\n",
				"b.yml": "services:\n  app:\n    image: registry.invalid/app:2\n    build:\n      dockerfile: B\n      args:\n        B: \"2\"\n",
			},
			composeService{image: "registry.invalid/app:2", hasBuild: true, dockerfile: "B", args: map[string]string{"A": "1", "B": "2"}},
		},
		{
			"later mapping extends a string shorthand",
			map[string]string{
				"a.yml": "services:\n  app:\n    build: ctx\n",
				"b.yml": "services:\n  app:\n    build:\n      dockerfile: Custom\n",
			},
			composeService{hasBuild: true, context: "ctx", dockerfile: "Custom"},
		},
		{
			"service declared only in a later file",
			map[string]string{
				"a.yml": "services:\n  other:\n    image: registry.invalid/other:1\n",
				"b.yml": "services:\n  app:\n    image: registry.invalid/app:1\n",
			},
			composeService{image: "registry.invalid/app:1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFiles(t, dir, tt.files)
			paths := make([]string, 0, len(tt.files))
			for _, p := range []string{"a.yml", "b.yml"} {
				if _, ok := tt.files[p]; ok {
					paths = append(paths, p)
				}
			}
			svc, err := loadComposeService(openRoot(t, dir), ".", paths, "app")
			if err != nil {
				t.Fatalf("loadComposeService: %v", err)
			}
			if diff := cmp.Diff(&tt.want, svc, cmp.AllowUnexported(composeService{})); diff != "" {
				t.Errorf("service mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadComposeService_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string]string
	}{
		{"missing file", nil},
		{"unparsable YAML", map[string]string{"a.yml": "services: [\n"}},
		{"service not found", map[string]string{"a.yml": "services:\n  other:\n    image: registry.invalid/other:1\n"}},
		{"missing services key", map[string]string{"a.yml": "version: \"3\"\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFiles(t, dir, tt.files)
			if _, err := loadComposeService(openRoot(t, dir), ".", []string{"a.yml"}, "app"); err == nil {
				t.Fatal("loadComposeService: got nil error")
			}
		})
	}
}

func TestMerge_ComposeImageService(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"privileged": true, "remoteUser": "vscode"}]`,
	}, false)

	src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
	root := mergeFiles(t, src, map[string]string{
		"docker-compose.yml": fmt.Sprintf("services:\n  app:\n    image: %s/base:1\n", host),
	})
	assertJSON(t, root, `{
	  "dockerComposeFile": "docker-compose.yml",
	  "service": "app",
	  "privileged": true,
	  "remoteUser": "vscode"
	}`)
}

func TestMerge_ComposeBuildService(t *testing.T) {
	t.Parallel()

	dockerfile := "FROM scratch\n" +
		`LABEL devcontainer.metadata='[{"privileged": true}]'` + "\n"
	want := `{
	  "dockerComposeFile": "docker-compose.yml",
	  "service": "app",
	  "privileged": true
	}`
	src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`

	t.Run("context and dockerfile", func(t *testing.T) {
		t.Parallel()
		root := mergeFiles(t, src, map[string]string{
			"docker-compose.yml": "services:\n  app:\n    build:\n      context: app\n      dockerfile: Custom.Dockerfile\n",
			"app/Custom.Dockerfile": dockerfile,
		})
		assertJSON(t, root, want)
	})

	t.Run("string shorthand sets the context, Dockerfile is the default", func(t *testing.T) {
		t.Parallel()
		root := mergeFiles(t, src, map[string]string{
			"docker-compose.yml": "services:\n  app:\n    build: app\n",
			"app/Dockerfile":     dockerfile,
		})
		assertJSON(t, root, want)
	})

	t.Run("context resolves relative to the first compose file", func(t *testing.T) {
		t.Parallel()
		root := mergeFiles(t, `{"dockerComposeFile": "sub/docker-compose.yml", "service": "app"}`, map[string]string{
			"sub/docker-compose.yml": "services:\n  app:\n    build: app\n",
			"sub/app/Dockerfile":     dockerfile,
		})
		assertJSON(t, root, `{
		  "dockerComposeFile": "sub/docker-compose.yml",
		  "service": "app",
		  "privileged": true
		}`)
	})
}

func TestMerge_ComposeBuildArgsAndTarget(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{imageMetadataLabel: `[{"remoteUser": "one"}]`}, false)
	ocitest.PushImage(t, host, "base", "2", map[string]string{imageMetadataLabel: `[{"remoteUser": "two"}]`}, false)
	files := map[string]string{
		"Dockerfile": fmt.Sprintf("ARG TAG=1\nFROM %s/base:${TAG} AS dev\nFROM scratch AS prod\n", host),
	}
	src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
	want := func(remoteUser string) string {
		return fmt.Sprintf(`{
		  "dockerComposeFile": "docker-compose.yml",
		  "service": "app",
		  "remoteUser": %q
		}`, remoteUser)
	}

	tests := []struct {
		name       string
		build      string
		remoteUser string
	}{
		{"args map form and target select the built stage", "      args:\n        TAG: \"2\"\n      target: dev\n", "two"},
		{"args list form", "      args:\n        - TAG=2\n      target: dev\n", "two"},
		{"arg with a variable substitution is dropped for its default", "      args:\n        TAG: ${localEnv:TAG}\n      target: dev\n", "one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := map[string]string{
				"docker-compose.yml": "services:\n  app:\n    build:\n      context: .\n" + tt.build,
			}
			maps.Copy(f, files)
			root := mergeFiles(t, src, f)
			assertJSON(t, root, want(tt.remoteUser))
		})
	}
}

func TestMerge_ComposeBuildLabelsOverride(t *testing.T) {
	t.Parallel()

	// The Dockerfile's own label must lose to the compose build labels, which "docker build --label"
	// applies after the LABEL instructions.
	dockerfile := "FROM scratch\n" +
		`LABEL devcontainer.metadata='[{"remoteUser": "from-dockerfile"}]'` + "\n"
	src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
	want := `{
	  "dockerComposeFile": "docker-compose.yml",
	  "service": "app",
	  "remoteUser": "from-labels"
	}`

	tests := []struct {
		name   string
		labels string
	}{
		{"map form", "      labels:\n        devcontainer.metadata: '[{\"remoteUser\": \"from-labels\"}]'\n"},
		{"list form", "      labels:\n        - 'devcontainer.metadata=[{\"remoteUser\": \"from-labels\"}]'\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := mergeFiles(t, src, map[string]string{
				"docker-compose.yml": "services:\n  app:\n    build:\n      context: .\n" + tt.labels,
				"Dockerfile":         dockerfile,
			})
			assertJSON(t, root, want)
		})
	}
}

func TestMerge_ComposeBuildPrecedesImage(t *testing.T) {
	t.Parallel()

	// When the service declares both, the reference implementation builds and the "image" only
	// names the resulting tag, so it must not even be fetched: fetching the unresolvable .invalid
	// reference would fail the merge.
	src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
	root := mergeFiles(t, src, map[string]string{
		"docker-compose.yml": "services:\n  app:\n    image: registry.invalid/app:1\n    build: .\n",
		"Dockerfile": "FROM scratch\n" +
			`LABEL devcontainer.metadata='{"privileged": true}'` + "\n",
	})
	assertJSON(t, root, `{
	  "dockerComposeFile": "docker-compose.yml",
	  "service": "app",
	  "privileged": true
	}`)
}

func TestMerge_ComposeRuntimeLabelsIgnored(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{"other": "value"}, false)

	// Service-level "labels" are container labels, not image labels; the reference implementation
	// derives the effective configuration from the image before the container exists, so a
	// "devcontainer.metadata" entry there must not contribute.
	src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
	root := mergeFiles(t, src, map[string]string{
		"docker-compose.yml": fmt.Sprintf(
			"services:\n  app:\n    image: %s/base:1\n    labels:\n      devcontainer.metadata: '[{\"remoteUser\": \"runtime\"}]'\n", host),
	})
	assertJSON(t, root, src)
}

func TestMerge_ComposeMultipleFiles(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{imageMetadataLabel: `[{"remoteUser": "one"}]`}, false)

	// The later file overrides the Dockerfile scalar (the earlier one would fail to resolve) while
	// the earlier file's arg still applies, proving the key-wise merge.
	src := `{"dockerComposeFile": ["a.yml", "b.yml"], "service": "app"}`
	root := mergeFiles(t, src, map[string]string{
		"a.yml":        "services:\n  app:\n    build:\n      context: .\n      dockerfile: A.Dockerfile\n      args:\n        TAG: \"1\"\n",
		"b.yml":        "services:\n  app:\n    build:\n      dockerfile: B.Dockerfile\n",
		"A.Dockerfile": "FROM registry.invalid/app:1\n",
		"B.Dockerfile": fmt.Sprintf("ARG TAG=0\nFROM %s/base:${TAG}\n", host),
	})
	assertJSON(t, root, `{
	  "dockerComposeFile": ["a.yml", "b.yml"],
	  "service": "app",
	  "remoteUser": "one"
	}`)
}

func TestMerge_ComposePrecedesBuildAndImage(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"remoteUser": "compose"}]`,
	}, false)

	// The reference implementation branches to the compose path first, so the conflicting "image"
	// and "build" must not be resolved: both would fail the merge.
	src := `{
	  "dockerComposeFile": "docker-compose.yml",
	  "service": "app",
	  "image": "registry.invalid/app:1",
	  "build": {"dockerfile": "Missing.Dockerfile"}
	}`
	root := mergeFiles(t, src, map[string]string{
		"docker-compose.yml": fmt.Sprintf("services:\n  app:\n    image: %s/base:1\n", host),
	})
	assertJSON(t, root, `{
	  "dockerComposeFile": "docker-compose.yml",
	  "service": "app",
	  "image": "registry.invalid/app:1",
	  "build": {"dockerfile": "Missing.Dockerfile"},
	  "remoteUser": "compose"
	}`)
}

// TestMerge_ComposeVariableSubstitutionSkipped covers the lint-time-unknowable declarations that
// skip compose resolution without falling back to "build" or "image": each fixture would fail the
// merge (a missing file or an unreachable registry) if resolution were attempted.
func TestMerge_ComposeVariableSubstitutionSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		files map[string]string
	}{
		{
			"compose file path",
			`{"dockerComposeFile": "${localEnv:DIR}/docker-compose.yml", "service": "app"}`,
			nil,
		},
		{
			"service name",
			`{"dockerComposeFile": "docker-compose.yml", "service": "${localEnv:SVC}"}`,
			nil,
		},
		{
			"service image",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{"docker-compose.yml": "services:\n  app:\n    image: registry.invalid/app:${TAG}\n"},
		},
		{
			"build context",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{"docker-compose.yml": "services:\n  app:\n    build: ${DIR}\n"},
		},
		{
			"build dockerfile",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{"docker-compose.yml": "services:\n  app:\n    build:\n      dockerfile: ${DF}\n"},
		},
		{
			"build target",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{
				"docker-compose.yml": "services:\n  app:\n    build:\n      context: .\n      target: ${STAGE}\n",
				"Dockerfile":         "FROM registry.invalid/app:1\n",
			},
		},
		{
			"metadata build label",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{
				"docker-compose.yml": "services:\n  app:\n    build:\n      context: .\n      labels:\n        devcontainer.metadata: ${MD}\n",
				"Dockerfile":         "FROM registry.invalid/app:1\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := mergeFiles(t, tt.src, tt.files)
			assertJSON(t, root, tt.src)
		})
	}
}

// TestMerge_ComposeMissingService covers declarations that resolve to nothing without an error: a
// missing or non-string "service" (a lint rule already flags the former), and a resolved service
// declaring neither "build" nor "image".
func TestMerge_ComposeMissingService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		files map[string]string
	}{
		{
			"service property absent",
			`{"dockerComposeFile": "docker-compose.yml"}`,
			nil,
		},
		{
			"service declares no image or build",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{"docker-compose.yml": "services:\n  app:\n    init: true\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := mergeFiles(t, tt.src, tt.files)
			assertJSON(t, root, tt.src)
		})
	}
}

// TestMerge_ComposeResolveErrors covers compose declarations whose resolution fails: the failure
// must surface as an error rather than a silent skip.
func TestMerge_ComposeResolveErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		files map[string]string
	}{
		{
			"missing compose file",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			nil,
		},
		{
			"unparsable compose file",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{"docker-compose.yml": "services: [\n"},
		},
		{
			"service not found",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{"docker-compose.yml": "services:\n  other:\n    image: registry.invalid/other:1\n"},
		},
		{
			"missing Dockerfile",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{"docker-compose.yml": "services:\n  app:\n    build: .\n"},
		},
		{
			"unfetchable image",
			`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`,
			map[string]string{"docker-compose.yml": "services:\n  app:\n    image: registry.invalid/app:1\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFiles(t, dir, tt.files)
			root, err := hujson.Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("parse devcontainer.json: %v", err)
			}
			if err := Merge(t.Context(), NewFetcher(), openRoot(t, dir), ".", &root); err == nil {
				t.Fatal("Merge: got nil error")
			}
		})
	}
}

func TestMerge_ComposeAnchor(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"privileged": true, "containerEnv": {"KEY": "value"}}]`,
	}, false)

	src := "{\n  \"dockerComposeFile\": \"docker-compose.yml\",\n  \"service\": \"app\"\n}"
	root := mergeFiles(t, src, map[string]string{
		"docker-compose.yml": fmt.Sprintf("services:\n  app:\n    image: %s/base:1\n", host),
	})
	anchor := strings.Index(src, `"dockerComposeFile"`)
	if anchor < 0 {
		t.Fatal("anchor not found in source")
	}
	for _, ptr := range []string{"/privileged", "/containerEnv", "/containerEnv/KEY"} {
		v := root.Find(ptr)
		if v == nil {
			t.Errorf("merged tree lacks %s", ptr)
			continue
		}
		if v.StartOffset != anchor {
			t.Errorf("%s StartOffset = %d, want %d (the dockerComposeFile key)", ptr, v.StartOffset, anchor)
		}
	}
}
