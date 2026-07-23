package feature

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/ocitest"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/tailscale/hujson"
)

// wantService is the flattened, comparable view of a resolved Compose service the load tests assert
// on, sparing them the pointer-valued MappingWithEquals of types.BuildConfig.
type wantService struct {
	image      string
	hasBuild   bool
	context    string
	dockerfile string
	target     string
	args       map[string]string
	labels     map[string]string
}

// flattenService reduces a resolved service to the fields the merge consumes. The build context,
// which compose-go resolves to an absolute path, is made relative to dir for a stable comparison.
func flattenService(dir string, svc *types.ServiceConfig) wantService {
	got := wantService{image: svc.Image}
	if svc.Build != nil {
		got.hasBuild = true
		got.context = svc.Build.Context
		if svc.Build.Context != "" {
			if rel, err := filepath.Rel(dir, svc.Build.Context); err == nil {
				got.context = rel
			}
		}
		got.dockerfile = svc.Build.Dockerfile
		got.target = svc.Build.Target
		for k, v := range svc.Build.Args {
			if v == nil {
				continue
			}
			if got.args == nil {
				got.args = map[string]string{}
			}
			got.args[k] = *v
		}
		if len(svc.Build.Labels) > 0 {
			got.labels = svc.Build.Labels
		}
	}
	return got
}

func TestReadFileLimited(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"f": "hello"})
	path := filepath.Join(dir, "f")

	if got, err := readFileLimited(path, 8); err != nil || string(got) != "hello" {
		t.Fatalf("readFileLimited within limit = %q, %v; want \"hello\", nil", got, err)
	}
	if _, err := readFileLimited(path, 4); err == nil {
		t.Error("readFileLimited over limit: got nil error")
	}
	if _, err := readFileLimited(filepath.Join(dir, "missing"), 8); err == nil {
		t.Error("readFileLimited on a missing file: got nil error")
	}
}

func TestLoadComposeService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string]string
		want  wantService
	}{
		{
			"image service",
			map[string]string{"a.yml": "services:\n  app:\n    image: registry.invalid/app:1\n"},
			wantService{image: "registry.invalid/app:1"},
		},
		{
			"build mapping",
			map[string]string{"a.yml": "services:\n  app:\n    build:\n      context: ctx\n      dockerfile: Custom\n      target: dev\n      args:\n        A: \"1\"\n      labels:\n        L: v\n"},
			wantService{hasBuild: true, context: "ctx", dockerfile: "Custom", target: "dev", args: map[string]string{"A": "1"}, labels: map[string]string{"L": "v"}},
		},
		{
			"build args and labels in list form",
			map[string]string{"a.yml": "services:\n  app:\n    build:\n      context: ctx\n      args:\n        - A=1\n        - PASSTHROUGH\n      labels:\n        - L=v\n"},
			wantService{hasBuild: true, context: "ctx", args: map[string]string{"A": "1"}, labels: map[string]string{"L": "v"}},
		},
		{
			"build string shorthand sets the context",
			map[string]string{"a.yml": "services:\n  app:\n    build: ctx\n"},
			wantService{hasBuild: true, context: "ctx"},
		},
		{
			"later file overrides scalars and merges args key-wise",
			map[string]string{
				"a.yml": "services:\n  app:\n    image: registry.invalid/app:1\n    build:\n      dockerfile: A\n      args:\n        A: \"1\"\n        B: \"1\"\n",
				"b.yml": "services:\n  app:\n    image: registry.invalid/app:2\n    build:\n      dockerfile: B\n      args:\n        B: \"2\"\n",
			},
			wantService{image: "registry.invalid/app:2", hasBuild: true, dockerfile: "B", args: map[string]string{"A": "1", "B": "2"}},
		},
		{
			"later mapping extends a string shorthand",
			map[string]string{
				"a.yml": "services:\n  app:\n    build: ctx\n",
				"b.yml": "services:\n  app:\n    build:\n      dockerfile: Custom\n",
			},
			wantService{hasBuild: true, context: "ctx", dockerfile: "Custom"},
		},
		{
			"service declared only in a later file",
			map[string]string{
				"a.yml": "services:\n  other:\n    image: registry.invalid/other:1\n",
				"b.yml": "services:\n  app:\n    image: registry.invalid/app:1\n",
			},
			wantService{image: "registry.invalid/app:1"},
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
			svc, err := loadComposeService(t.Context(), dir, paths, "app")
			if err != nil {
				t.Fatalf("loadComposeService: %v", err)
			}
			if diff := cmp.Diff(tt.want, flattenService(dir, svc), cmp.AllowUnexported(wantService{})); diff != "" {
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
		// A null "build" is invalid Compose; docker compose rejects it, so decolint does too.
		{"null build", map[string]string{"a.yml": "services:\n  app:\n    image: registry.invalid/app:1\n    build:\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFiles(t, dir, tt.files)
			if _, err := loadComposeService(t.Context(), dir, []string{"a.yml"}, "app"); err == nil {
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

func TestMerge_ComposeExtends(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"privileged": true}]`,
	}, false)

	src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
	want := `{
	  "dockerComposeFile": "docker-compose.yml",
	  "service": "app",
	  "privileged": true
	}`

	t.Run("service extends a base service in another file", func(t *testing.T) {
		t.Parallel()
		root := mergeFiles(t, src, map[string]string{
			"docker-compose.yml": "services:\n  app:\n    extends:\n      file: base/compose.yml\n      service: base\n",
			"base/compose.yml":   fmt.Sprintf("services:\n  base:\n    image: %s/base:1\n", host),
		})
		assertJSON(t, root, want)
	})

	t.Run("extends chains across nested files", func(t *testing.T) {
		t.Parallel()
		// app -> a/mid.yml(mid) -> a/base.yml(base), where a/base.yml resolves relative to a/mid.yml.
		root := mergeFiles(t, src, map[string]string{
			"docker-compose.yml": "services:\n  app:\n    extends:\n      file: a/mid.yml\n      service: mid\n",
			"a/mid.yml":          "services:\n  mid:\n    extends:\n      file: base.yml\n      service: base\n",
			"a/base.yml":         fmt.Sprintf("services:\n  base:\n    image: %s/base:1\n", host),
		})
		assertJSON(t, root, want)
	})
}

func TestMerge_ComposeInclude(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"privileged": true}]`,
	}, false)

	// The compose file includes another that defines the base service the app extends.
	src := `{"dockerComposeFile": "docker-compose.yml", "service": "app"}`
	root := mergeFiles(t, src, map[string]string{
		"docker-compose.yml": "include:\n  - base/compose.yml\nservices:\n  app:\n    extends:\n      service: base\n",
		"base/compose.yml":   fmt.Sprintf("services:\n  base:\n    image: %s/base:1\n", host),
	})
	assertJSON(t, root, `{
	  "dockerComposeFile": "docker-compose.yml",
	  "service": "app",
	  "privileged": true
	}`)
}

// TestMerge_ComposeFileOutsideConfigDir covers the common layout of a devcontainer.json under
// .devcontainer that names a Compose file at the repository root via a "../" path: Compose
// resolution reads from the real filesystem, so it is not confined to the config directory. The
// config is opened confined to .devcontainer, mirroring discovery.
func TestMerge_ComposeFileOutsideConfigDir(t *testing.T) {
	t.Parallel()

	src := `{"dockerComposeFile": "../docker-compose.yml", "service": "app"}`
	want := `{
	  "dockerComposeFile": "../docker-compose.yml",
	  "service": "app",
	  "privileged": true
	}`

	t.Run("image at the repository root", func(t *testing.T) {
		t.Parallel()
		host := ocitest.Registry(t)
		ocitest.PushImage(t, host, "base", "1", map[string]string{imageMetadataLabel: `[{"privileged": true}]`}, false)

		repo := t.TempDir()
		writeFiles(t, repo, map[string]string{
			"docker-compose.yml": fmt.Sprintf("services:\n  app:\n    image: %s/base:1\n", host),
		})
		root := mergeOutsideDir(t, repo, src)
		assertJSON(t, root, want)
	})

	t.Run("build context at the repository root", func(t *testing.T) {
		t.Parallel()
		// The Dockerfile sits at the repository root, outside the .devcontainer the config is confined
		// to, exercising the out-of-directory read and the absolute display reference.
		repo := t.TempDir()
		writeFiles(t, repo, map[string]string{
			"docker-compose.yml": "services:\n  app:\n    build:\n      context: .\n",
			"Dockerfile":         "FROM scratch\n" + `LABEL devcontainer.metadata='[{"privileged": true}]'` + "\n",
		})
		root := mergeOutsideDir(t, repo, src)
		assertJSON(t, root, want)
	})
}

// mergeOutsideDir parses src as the .devcontainer/devcontainer.json of repo and merges it with the
// root confined to that .devcontainer directory, the confinement discovery applies.
func mergeOutsideDir(t *testing.T, repo, src string) *hujson.Value {
	t.Helper()
	writeFiles(t, repo, map[string]string{".devcontainer/devcontainer.json": src})
	root, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse devcontainer.json: %v", err)
	}
	if err := Merge(t.Context(), NewFetcher(), openRoot(t, filepath.Join(repo, ".devcontainer")), ".", &root); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return &root
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
			"docker-compose.yml":    "services:\n  app:\n    build:\n      context: app\n      dockerfile: Custom.Dockerfile\n",
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

	t.Run("dockerfile_inline is built without a file", func(t *testing.T) {
		t.Parallel()
		// The inline Dockerfile lives in the compose file as a block scalar, so no Dockerfile is
		// written to disk.
		compose := "services:\n" +
			"  app:\n" +
			"    build:\n" +
			"      dockerfile_inline: |\n" +
			"        FROM scratch\n" +
			`        LABEL devcontainer.metadata='[{"privileged": true}]'` + "\n"
		root := mergeFiles(t, src, map[string]string{"docker-compose.yml": compose})
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

// TestMerge_ComposeVariableSubstitutionSkipped covers the declarations a Compose-interpolation
// variable leaves unknowable at lint time (decolint does not apply interpolation), which skip
// compose resolution without falling back to "build" or "image": each fixture would fail the merge
// (a missing file or an unreachable registry) if resolution were attempted.
func TestMerge_ComposeVariableSubstitutionSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		files map[string]string
	}{
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
