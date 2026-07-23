package feature

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/ocitest"
	"github.com/google/go-cmp/cmp"
	"github.com/tailscale/hujson"
)

func TestMerge_NoFeatures(t *testing.T) {
	t.Parallel()

	src := `{"name": "app"}`
	root := mergeSrc(t, src, nil)
	assertJSON(t, root, src)
}

func TestMerge_ImageMetadata(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"privileged": true, "remoteUser": "vscode", "containerEnv": {"FROM_IMAGE": "1"}}]`,
	}, false)

	src := fmt.Sprintf(`{"image": %q}`, host+"/base:1")
	root := mergeSrc(t, src, nil)
	assertJSON(t, root, fmt.Sprintf(`{
	  "image": %q,
	  "privileged": true,
	  "remoteUser": "vscode",
	  "containerEnv": {"FROM_IMAGE": "1"}
	}`, host+"/base:1"))
}

func TestMerge_ImageMetadataPrecedence(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	// Two label entries: the later one wins the intra-image "E" and "remoteUser" conflicts.
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"remoteUser": "first", "containerEnv": {"A": "image", "D": "image", "E": "e1"}}, {"remoteUser": "second", "containerEnv": {"E": "e2"}}]`,
	}, false)

	src := fmt.Sprintf(`{
	  "image": %q,
	  "containerEnv": {"D": "user"},
	  "features": {"./f": {}}
	}`, host+"/base:1")
	root := mergeSrc(t, src, map[string]string{
		"f": `{"id": "f", "containerEnv": {"A": "feat"}}`,
	})
	// A: the Feature applies after image metadata and wins. D: the user's own value beats both.
	// E and remoteUser: the later image entry beats the earlier one.
	assertJSON(t, root, fmt.Sprintf(`{
	  "image": %q,
	  "containerEnv": {"D": "user", "A": "feat", "E": "e2"},
	  "features": {"./f": {}},
	  "remoteUser": "second"
	}`, host+"/base:1"))
}

func TestMerge_ImageMetadataUserScalarWins(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"remoteUser": "vscode"}]`,
	}, false)

	src := fmt.Sprintf(`{"image": %q, "remoteUser": "root"}`, host+"/base:1")
	root := mergeSrc(t, src, nil)
	assertJSON(t, root, fmt.Sprintf(`{"image": %q, "remoteUser": "root"}`, host+"/base:1"))
}

func TestMerge_ImageMetadataAnchor(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"privileged": true, "containerEnv": {"KEY": "value"}}]`,
	}, false)

	src := fmt.Sprintf("{\n  \"image\": %q\n}", host+"/base:1")
	root := mergeSrc(t, src, nil)
	anchor := strings.Index(src, `"image"`)
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
			t.Errorf("%s StartOffset = %d, want %d (the image key)", ptr, v.StartOffset, anchor)
		}
	}
}

func TestMerge_ImageFetchError(t *testing.T) {
	t.Parallel()

	// The reserved .invalid TLD never resolves, so fetching the image fails and the failure must
	// surface as an error rather than a silent skip.
	src := `{"image": "registry.invalid/app:1"}`
	root, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse devcontainer.json: %v", err)
	}
	if err := Merge(t.Context(), NewFetcher(), nil, "", &root); err == nil {
		t.Fatal("Merge with an unreachable image: got nil error")
	}
}

func TestMerge_BuildDockerfileMetadata(t *testing.T) {
	t.Parallel()

	src := `{"build": {"dockerfile": "Dockerfile"}}`
	root := mergeFiles(t, src, map[string]string{
		"Dockerfile": "FROM scratch\n" +
			`LABEL devcontainer.metadata='[{"privileged": true, "remoteUser": "vscode"}]'` + "\n",
	})
	assertJSON(t, root, `{
	  "build": {"dockerfile": "Dockerfile"},
	  "privileged": true,
	  "remoteUser": "vscode"
	}`)
}

func TestMerge_BuildDockerfileBaseImage(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{
		imageMetadataLabel: `[{"containerEnv": {"FROM_BASE": "1"}}]`,
	}, false)

	src := `{"build": {"dockerfile": "Dockerfile"}}`
	root := mergeFiles(t, src, map[string]string{
		"Dockerfile": fmt.Sprintf("FROM %s/base:1\n", host),
	})
	assertJSON(t, root, `{
	  "build": {"dockerfile": "Dockerfile"},
	  "containerEnv": {"FROM_BASE": "1"}
	}`)
}

func TestMerge_BuildDockerfileArgsAndTarget(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	ocitest.PushImage(t, host, "base", "1", map[string]string{imageMetadataLabel: `[{"remoteUser": "one"}]`}, false)
	ocitest.PushImage(t, host, "base", "2", map[string]string{imageMetadataLabel: `[{"remoteUser": "two"}]`}, false)
	dockerfile := fmt.Sprintf("ARG TAG=1\nFROM %s/base:${TAG} AS dev\nFROM scratch AS prod\n", host)

	t.Run("args and target select the built stage", func(t *testing.T) {
		t.Parallel()
		src := `{"build": {"dockerfile": "Dockerfile", "args": {"TAG": "2"}, "target": "dev"}}`
		root := mergeFiles(t, src, map[string]string{"Dockerfile": dockerfile})
		assertJSON(t, root, `{
		  "build": {"dockerfile": "Dockerfile", "args": {"TAG": "2"}, "target": "dev"},
		  "remoteUser": "two"
		}`)
	})

}

func TestMerge_BuildDockerfilePrecedesImage(t *testing.T) {
	t.Parallel()

	// When both are declared the reference implementation builds the Dockerfile, so the "image"
	// must not even be fetched: fetching the unresolvable .invalid reference would fail the merge.
	src := `{"image": "registry.invalid/app:1", "build": {"dockerfile": "Dockerfile"}}`
	root := mergeFiles(t, src, map[string]string{
		"Dockerfile": "FROM scratch\n" +
			`LABEL devcontainer.metadata='{"privileged": true}'` + "\n",
	})
	assertJSON(t, root, `{
	  "image": "registry.invalid/app:1",
	  "build": {"dockerfile": "Dockerfile"},
	  "privileged": true
	}`)
}

func TestMerge_LegacyDockerFileProperty(t *testing.T) {
	t.Parallel()

	src := `{"dockerFile": "Dockerfile"}`
	root := mergeFiles(t, src, map[string]string{
		"Dockerfile": "FROM scratch\n" +
			`LABEL devcontainer.metadata='{"init": true}'` + "\n",
	})
	assertJSON(t, root, `{"dockerFile": "Dockerfile", "init": true}`)
}

// TestMerge_LegacyDockerFilePropertyPrecedence pins the reference implementation's preference for
// the top-level "dockerFile" over "build.dockerfile" when both are present. The schema makes the two
// forms mutually exclusive, so this only differs on an invalid configuration; decolint tracks the
// real tooling, which reads the top-level property first (getDockerfile in devcontainers/cli).
func TestMerge_LegacyDockerFilePropertyPrecedence(t *testing.T) {
	t.Parallel()

	src := `{"dockerFile": "Top", "build": {"dockerfile": "Nested"}}`
	root := mergeFiles(t, src, map[string]string{
		"Top":    "FROM scratch\n" + `LABEL devcontainer.metadata='{"remoteUser": "top"}'` + "\n",
		"Nested": "FROM scratch\n" + `LABEL devcontainer.metadata='{"remoteUser": "nested"}'` + "\n",
	})
	assertJSON(t, root, `{
	  "dockerFile": "Top",
	  "build": {"dockerfile": "Nested"},
	  "remoteUser": "top"
	}`)
}

func TestMerge_BuildDockerfileAnchor(t *testing.T) {
	t.Parallel()

	src := "{\n  \"build\": {\n    \"dockerfile\": \"Dockerfile\"\n  }\n}"
	root := mergeFiles(t, src, map[string]string{
		"Dockerfile": "FROM scratch\n" +
			`LABEL devcontainer.metadata='{"privileged": true, "containerEnv": {"KEY": "value"}}'` + "\n",
	})
	anchor := strings.Index(src, `"dockerfile"`)
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
			t.Errorf("%s StartOffset = %d, want %d (the dockerfile key)", ptr, v.StartOffset, anchor)
		}
	}
}

func TestMerge_BuildDockerfileMissing(t *testing.T) {
	t.Parallel()

	root, err := hujson.Parse([]byte(`{"build": {"dockerfile": "Dockerfile"}}`))
	if err != nil {
		t.Fatalf("parse devcontainer.json: %v", err)
	}
	if err := Merge(t.Context(), NewFetcher(), openRoot(t, t.TempDir()), ".", &root); err == nil {
		t.Fatal("Merge with a missing Dockerfile: got nil error")
	}
}

func TestMerge_BooleanOr(t *testing.T) {
	t.Parallel()

	feature := `{"id": "f", "privileged": true, "init": true}`

	t.Run("absent member is added", func(t *testing.T) {
		t.Parallel()
		root := mergeSrc(t, `{"features": {"./f": {}}}`, map[string]string{"f": feature})
		assertJSON(t, root, `{"features": {"./f": {}}, "privileged": true, "init": true}`)
	})

	t.Run("explicit false is overridden", func(t *testing.T) {
		t.Parallel()
		root := mergeSrc(t, `{"privileged": false, "features": {"./f": {}}}`, map[string]string{"f": feature})
		assertJSON(t, root, `{"privileged": true, "features": {"./f": {}}, "init": true}`)
	})

	t.Run("feature false does not override", func(t *testing.T) {
		t.Parallel()
		root := mergeSrc(t, `{"privileged": false, "features": {"./f": {}}}`,
			map[string]string{"f": `{"id": "f", "privileged": false}`})
		assertJSON(t, root, `{"privileged": false, "features": {"./f": {}}}`)
	})
}

func TestMerge_UnionArrays(t *testing.T) {
	t.Parallel()

	root := mergeSrc(t,
		`{"capAdd": ["SYS_PTRACE"], "features": {"./a": {}, "./b": {}}}`,
		map[string]string{
			"a": `{"id": "a", "capAdd": ["SYS_PTRACE", "NET_ADMIN"], "securityOpt": ["seccomp=unconfined"]}`,
			"b": `{"id": "b", "capAdd": ["NET_ADMIN", "SYS_ADMIN"]}`,
		})
	assertJSON(t, root, `{
	  "capAdd": ["SYS_PTRACE", "NET_ADMIN", "SYS_ADMIN"],
	  "features": {"./a": {}, "./b": {}},
	  "securityOpt": ["seccomp=unconfined"]
	}`)
}

func TestMerge_ContainerEnv(t *testing.T) {
	t.Parallel()

	root := mergeSrc(t,
		`{"containerEnv": {"USER_VAR": "user"}, "features": {"./a": {}, "./b": {}}}`,
		map[string]string{
			"a": `{"id": "a", "containerEnv": {"USER_VAR": "a", "SHARED": "a", "A_ONLY": "a"}}`,
			"b": `{"id": "b", "containerEnv": {"SHARED": "b"}}`,
		})
	// The user's own key always wins; for keys only features set, the later feature wins.
	assertJSON(t, root, `{
	  "containerEnv": {"USER_VAR": "user", "SHARED": "b", "A_ONLY": "a"},
	  "features": {"./a": {}, "./b": {}}
	}`)
}

func TestMerge_Mounts(t *testing.T) {
	t.Parallel()

	root := mergeSrc(t,
		`{
		  "mounts": [{"type": "volume", "source": "user-vol", "target": "/data"}],
		  "features": {"./a": {}, "./b": {}}
		}`,
		map[string]string{
			"a": `{"id": "a", "mounts": [
			  {"type": "volume", "source": "a-vol", "target": "/data"},
			  "source=a-cache,target=/cache,type=volume"
			]}`,
			"b": `{"id": "b", "mounts": ["source=b-cache,target=/cache,type=volume"]}`,
		})
	// /data is mounted by the user (user wins); /cache is contributed by both features (the later
	// one wins).
	assertJSON(t, root, `{
	  "mounts": [
	    {"type": "volume", "source": "user-vol", "target": "/data"},
	    "source=b-cache,target=/cache,type=volume"
	  ],
	  "features": {"./a": {}, "./b": {}}
	}`)
}

func TestMerge_MountsUnparsableTargetPassthrough(t *testing.T) {
	t.Parallel()

	// A mount whose target cannot be determined (a string with no target key, or an object that
	// declares none) is not deduplicated: it has no key to match on, so every such entry is kept as
	// contributed, including two identical target-less strings from different features.
	root := mergeSrc(t,
		`{"features": {"./a": {}, "./b": {}}}`,
		map[string]string{
			"a": `{"id": "a", "mounts": [
			  "source=shared,type=volume",
			  {"type": "tmpfs", "source": "no-target"}
			]}`,
			"b": `{"id": "b", "mounts": ["source=shared,type=volume"]}`,
		})
	assertJSON(t, root, `{
	  "mounts": [
	    "source=shared,type=volume",
	    {"type": "tmpfs", "source": "no-target"},
	    "source=shared,type=volume"
	  ],
	  "features": {"./a": {}, "./b": {}}
	}`)
}

func TestMerge_Customizations(t *testing.T) {
	t.Parallel()

	root := mergeSrc(t,
		`{
		  "customizations": {"vscode": {"extensions": ["user.ext"], "settings": {"a": "user"}}},
		  "features": {"./f": {}}
		}`,
		map[string]string{
			"f": `{"id": "f", "customizations": {"vscode": {
			  "extensions": ["feature.ext", "user.ext"],
			  "settings": {"a": "feature", "b": "feature"}
			}}}`,
		})
	// Extension lists are concatenated (without duplicates); on a scalar conflict the user wins.
	assertJSON(t, root, `{
	  "customizations": {"vscode": {
	    "extensions": ["feature.ext", "user.ext"],
	    "settings": {"a": "user", "b": "feature"}
	  }},
	  "features": {"./f": {}}
	}`)
}

func TestMerge_LifecycleHooks(t *testing.T) {
	t.Parallel()

	t.Run("user command in string form", func(t *testing.T) {
		t.Parallel()
		root := mergeSrc(t,
			`{"postCreateCommand": "make setup", "features": {"./f": {}}}`,
			map[string]string{"f": `{"id": "f", "postCreateCommand": "f-setup.sh"}`})
		assertJSON(t, root, `{
		  "postCreateCommand": {"f": "f-setup.sh", "devcontainer.json": "make setup"},
		  "features": {"./f": {}}
		}`)
	})

	t.Run("user command in object form", func(t *testing.T) {
		t.Parallel()
		root := mergeSrc(t,
			`{"onCreateCommand": {"mine": "make setup"}, "features": {"./f": {}}}`,
			map[string]string{"f": `{"id": "f", "onCreateCommand": ["echo", "hi"]}`})
		assertJSON(t, root, `{
		  "onCreateCommand": {"f": ["echo", "hi"], "mine": "make setup"},
		  "features": {"./f": {}}
		}`)
	})

	t.Run("no user command", func(t *testing.T) {
		t.Parallel()
		root := mergeSrc(t,
			`{"features": {"./f": {}}}`,
			map[string]string{"f": `{"id": "f", "postStartCommand": "f-start.sh"}`})
		assertJSON(t, root, `{
		  "postStartCommand": {"f": "f-start.sh"},
		  "features": {"./f": {}}
		}`)
	})

	t.Run("multiple features contribute the same hook", func(t *testing.T) {
		t.Parallel()
		// Each contributing Feature gets its own key, keyed by ID, alongside the user's own command;
		// the object form holds them all rather than one command overwriting another.
		root := mergeSrc(t,
			`{"postCreateCommand": "make setup", "features": {"./a": {}, "./b": {}}}`,
			map[string]string{
				"a": `{"id": "a", "postCreateCommand": "a-setup.sh"}`,
				"b": `{"id": "b", "postCreateCommand": ["echo", "b"]}`,
			})
		assertJSON(t, root, `{
		  "postCreateCommand": {
		    "a": "a-setup.sh",
		    "b": ["echo", "b"],
		    "devcontainer.json": "make setup"
		  },
		  "features": {"./a": {}, "./b": {}}
		}`)
	})
}

func TestMerge_DependsOn(t *testing.T) {
	t.Parallel()

	root := mergeSrc(t,
		`{"features": {"./a": {}}}`,
		map[string]string{
			"a":   `{"id": "a", "dependsOn": {"./dep": {}}, "containerEnv": {"SHARED": "a"}}`,
			"dep": `{"id": "dep", "containerEnv": {"SHARED": "dep", "DEP_ONLY": "dep"}, "privileged": true}`,
		})
	// The dependency installs first, so the dependent feature wins the SHARED conflict; the
	// dependency's other contributions are merged as well.
	assertJSON(t, root, `{
	  "features": {"./a": {}},
	  "containerEnv": {"SHARED": "a", "DEP_ONLY": "dep"},
	  "privileged": true
	}`)
}

func TestMerge_AnchorsDeclaredDependencyAtOwnKey(t *testing.T) {
	t.Parallel()

	// b is declared directly and is also a dependency of the earlier-declared a. Its contributions
	// must anchor to its own "./b" key, not to a's, so findings and inline suppressions land there.
	src := `{
  "features": {
    "./a": {},
    "./b": {}
  }
}`
	root := mergeSrc(t, src, map[string]string{
		"a": `{"id": "a", "dependsOn": {"./b": {}}}`,
		"b": `{"id": "b", "privileged": true}`,
	})
	anchor := strings.Index(src, `"./b"`)
	if anchor < 0 {
		t.Fatal("anchor not found in source")
	}
	v := root.Find("/privileged")
	if v == nil {
		t.Fatal("merged tree lacks /privileged")
	}
	if v.StartOffset != anchor {
		t.Errorf("/privileged StartOffset = %d, want %d (the ./b feature key)", v.StartOffset, anchor)
	}
}

func TestMerge_DependsOnCycle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLocalFeature(t, dir, "a", `{"id": "a", "dependsOn": {"./b": {}}}`)
	writeLocalFeature(t, dir, "b", `{"id": "b", "dependsOn": {"./a": {}}}`)
	root, err := hujson.Parse([]byte(`{"features": {"./a": {}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := Merge(t.Context(), NewFetcher(), openRoot(t, dir), ".", &root); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("Merge with a dependency cycle: err = %v, want a cycle error", err)
	}
}

// TestMerge_ResolveErrors covers the resolution failures Merge surfaces before ordering: every point
// at which installSequence, resolveAll, or applyOverride parses a reference or fetches a Feature. The
// cases are same-shaped (Merge returns an error), only the failing reference differs.
func TestMerge_ResolveErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		features map[string]string
	}{
		{
			name: "declared feature cannot be fetched",
			src:  `{"features": {"./missing": {}}}`,
		},
		{
			name: "declared reference is invalid",
			src:  `{"features": {"no-slash": {}}}`,
		},
		{
			name:     "dependsOn reference is invalid",
			src:      `{"features": {"./a": {}}}`,
			features: map[string]string{"a": `{"id": "a", "dependsOn": {"no-slash": {}}}`},
		},
		{
			name:     "installsAfter reference is invalid",
			src:      `{"features": {"./a": {}}}`,
			features: map[string]string{"a": `{"id": "a", "installsAfter": ["no-slash"]}`},
		},
		{
			name:     "installsAfter target cannot be fetched",
			src:      `{"features": {"./a": {}}}`,
			features: map[string]string{"a": `{"id": "a", "installsAfter": ["./missing"]}`},
		},
		{
			name:     "override reference is invalid",
			src:      `{"overrideFeatureInstallOrder": ["no-slash"], "features": {"./a": {}}}`,
			features: map[string]string{"a": `{"id": "a"}`},
		},
		{
			name:     "override target cannot be fetched",
			src:      `{"overrideFeatureInstallOrder": ["./missing"], "features": {"./a": {}}}`,
			features: map[string]string{"a": `{"id": "a"}`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for name, content := range tt.features {
				writeLocalFeature(t, dir, name, content)
			}
			root, err := hujson.Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("parse devcontainer.json: %v", err)
			}
			if err := Merge(t.Context(), NewFetcher(), openRoot(t, dir), ".", &root); err == nil {
				t.Error("Merge: got nil error, want a resolution error")
			}
		})
	}
}

func TestMerge_InstallsAfter(t *testing.T) {
	t.Parallel()

	// b declares installsAfter a, so a installs first and b wins the conflict even though b is
	// declared first. A local Feature is matched by its path, per the specification.
	root := mergeSrc(t,
		`{"features": {"./b": {}, "./a": {}}}`,
		map[string]string{
			"a": `{"id": "a", "containerEnv": {"SHARED": "a"}}`,
			"b": `{"id": "b", "installsAfter": ["./a"], "containerEnv": {"SHARED": "b"}}`,
		})
	assertJSON(t, root, `{
	  "features": {"./b": {}, "./a": {}},
	  "containerEnv": {"SHARED": "b"}
	}`)
}

func TestMerge_OverrideFeatureInstallOrder(t *testing.T) {
	t.Parallel()

	// The override moves b to the front, so a installs later and wins the conflict.
	root := mergeSrc(t,
		`{
		  "overrideFeatureInstallOrder": ["./b"],
		  "features": {"./a": {}, "./b": {}}
		}`,
		map[string]string{
			"a": `{"id": "a", "containerEnv": {"SHARED": "a"}}`,
			"b": `{"id": "b", "containerEnv": {"SHARED": "b"}}`,
		})
	assertJSON(t, root, `{
	  "overrideFeatureInstallOrder": ["./b"],
	  "features": {"./a": {}, "./b": {}},
	  "containerEnv": {"SHARED": "a"}
	}`)
}

// TestMerge_OverrideFeatureInstallOrderOCILegacyAlias covers the alias branch of applyOverride: an
// "overrideFeatureInstallOrder" entry that names a renamed OCI Feature by its current id must still
// match a contributor requested under a legacy id, through the Feature's declared "legacyIds".
func TestMerge_OverrideFeatureInstallOrderOCILegacyAlias(t *testing.T) {
	t.Parallel()

	host := ocitest.Registry(t)
	// The renamed Feature declares its legacy id and is published under both paths, so a reference by
	// either the current id ("renamed", used in the override) or the legacy id ("legacy", used in
	// "features") resolves to the same identity.
	renamed := ocitest.FeatureArchive(t, `{"id": "renamed", "legacyIds": ["legacy"], "containerEnv": {"SHARED": "renamed"}}`, false)
	ocitest.PushFeature(t, host, "features/renamed", "1", renamed, false)
	ocitest.PushFeature(t, host, "features/legacy", "1", renamed, false)
	ocitest.PushFeature(t, host, "features/aaa", "1",
		ocitest.FeatureArchive(t, `{"id": "aaa", "containerEnv": {"SHARED": "aaa"}}`, false), false)

	features := `"` + host + `/features/legacy:1": {}, "` + host + `/features/aaa:1": {}`
	merge := func(t *testing.T, src string) *hujson.Value {
		t.Helper()
		root, err := hujson.Parse([]byte(src))
		if err != nil {
			t.Fatalf("parse devcontainer.json: %v", err)
		}
		// OCI Features need no confining root; local resolution is not exercised here.
		if err := Merge(t.Context(), NewFetcher(), nil, "", &root); err != nil {
			t.Fatalf("Merge: %v", err)
		}
		return &root
	}
	sharedEnv := func(t *testing.T, root *hujson.Value) string {
		t.Helper()
		v := root.Find("/containerEnv/SHARED")
		if v == nil {
			t.Fatal("merged tree lacks /containerEnv/SHARED")
		}
		lit, ok := v.Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' {
			t.Fatalf("SHARED is not a string: %+v", v.Value)
		}
		return lit.String()
	}

	t.Run("without override the renamed feature installs last and wins", func(t *testing.T) {
		// "aaa" < "legacy" by resource id, so legacy installs last and wins the SHARED conflict.
		root := merge(t, `{"features": {`+features+`}}`)
		if got := sharedEnv(t, root); got != "renamed" {
			t.Errorf("SHARED = %q, want renamed", got)
		}
	})

	t.Run("override matched by legacy alias reorders the round", func(t *testing.T) {
		// The override names the current id; matching the legacy-named contributor through its alias
		// raises it into the first round, so aaa installs last and wins instead.
		root := merge(t, `{"overrideFeatureInstallOrder": ["`+host+`/features/renamed:1"], "features": {`+features+`}}`)
		if got := sharedEnv(t, root); got != "aaa" {
			t.Errorf("SHARED = %q, want aaa", got)
		}
	})
}

// TestInstallSequence covers the contributor resolution installSequence performs before ordering
// (merge.go's resolveAll), asserted at install-order precision rather than through the merged tree:
// identical requests for the same Feature collapse to one node, while requests that differ only in
// options stay distinct and order by the specification's options comparison. The ordering algorithm
// itself is covered by TestInstallOrder, and the dependsOn/installsAfter/override wiring by the
// merged-JSON tests above.
func TestInstallSequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		features map[string]string
		want     []string
	}{
		{
			// ./c is pulled in by both ./a and ./b with identical options, so resolveAll deduplicates it
			// to a single contributor installed once.
			name: "collapses identical requests",
			src:  `{"features": {"./a": {}, "./b": {}}}`,
			features: map[string]string{
				"a": `{"id": "a", "dependsOn": {"./c": {}}}`,
				"b": `{"id": "b", "dependsOn": {"./c": {}}}`,
				"c": `{"id": "c"}`,
			},
			want: []string{"./c{}", "./a{}", "./b{}"},
		},
		{
			// The same ./b requested with different options is a distinct contributor each time, and the
			// round of them is ordered by the specification's options comparison.
			name: "keeps option-distinct requests distinct",
			src:  `{"features": {"./a": {"optA": "a", "optB": "b"}, "./b": {"optA": "a", "optB": "b"}}}`,
			features: map[string]string{
				"a": `{"id": "a", "dependsOn": {"./b": {"optA": "a", "optB": "a"}, "./c": {}}}`,
				"b": `{"id": "b"}`,
				"c": `{"id": "c", "dependsOn": {"./b": {"optA": "b", "optB": "a"}, "./d": {}, "./e": {}}}`,
				"d": `{"id": "d", "dependsOn": {"./b": {"optA": "b", "optB": "b"}}}`,
				"e": `{"id": "e", "dependsOn": {"./b": {}}}`,
			},
			want: []string{
				"./b{}",
				"./b{optA=a,optB=a}",
				"./b{optA=a,optB=b}",
				"./b{optA=b,optB=a}",
				"./b{optA=b,optB=b}",
				"./d{}",
				"./e{}",
				"./c{}",
				"./a{optA=a,optB=b}",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveOrder(t, tt.src, tt.features)
			assertOrder(t, got, tt.want)
		})
	}
}

func TestInstallSequence_InstallsAfterNotPulledIn(t *testing.T) {
	t.Parallel()

	// b's soft dependency on ./c is resolved so it can be matched, but a soft dependency is never
	// installed on its own: ./c is not otherwise part of the merge, so it must not appear as a
	// contributor even though its metadata is fetched.
	got := resolveOrder(t,
		`{"features": {"./b": {}}}`,
		map[string]string{
			"b": `{"id": "b", "installsAfter": ["./c"], "containerEnv": {"SHARED": "b"}}`,
			"c": `{"id": "c", "containerEnv": {"SHARED": "c"}}`,
		})
	assertOrder(t, got, []string{"./b{}"})
}

// TestInstallSequence_Override covers applyOverride's contribution: "overrideFeatureInstallOrder"
// raises listed Features into earlier rounds by list position, and naming a Feature absent from the
// merge changes nothing. The alias-matching branch is covered by
// TestMerge_OverrideFeatureInstallOrderOCILegacyAlias.
func TestInstallSequence_Override(t *testing.T) {
	t.Parallel()

	// All three Features are independent, so without the override they order by resource id alone.
	features := map[string]string{
		"a": `{"id": "a"}`,
		"b": `{"id": "b"}`,
		"c": `{"id": "c"}`,
	}
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			// ./c is listed first (highest priority) and ./b second; each installs in its own earlier
			// round, leaving the unlisted ./a for last.
			name: "listed features move into earlier rounds by position",
			src:  `{"overrideFeatureInstallOrder": ["./c", "./b"], "features": {"./a": {}, "./b": {}, "./c": {}}}`,
			want: []string{"./c{}", "./b{}", "./a{}"},
		},
		{
			// ./c is fetchable but not part of the merge, so naming it matches no contributor.
			name: "listing a feature absent from the merge is a no-op",
			src:  `{"overrideFeatureInstallOrder": ["./c"], "features": {"./a": {}, "./b": {}}}`,
			want: []string{"./a{}", "./b{}"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveOrder(t, tt.src, features)
			assertOrder(t, got, tt.want)
		})
	}
}

// TestInstallSequence_IgnoresMalformedShapes covers the type guards on "features" and
// "overrideFeatureInstallOrder": a value of the wrong JSON type is ignored, not an error.
func TestInstallSequence_IgnoresMalformedShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			// "features" that is not an object contributes nothing.
			name: "features is not an object",
			src:  `{"features": []}`,
			want: []string{},
		},
		{
			// "overrideFeatureInstallOrder" that is not an array is ignored, leaving the natural order.
			name: "override is not an array",
			src:  `{"overrideFeatureInstallOrder": {}, "features": {"./a": {}, "./b": {}}}`,
			want: []string{"./a{}", "./b{}"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveOrder(t, tt.src, map[string]string{"a": `{"id": "a"}`, "b": `{"id": "b"}`})
			assertOrder(t, got, tt.want)
		})
	}
}

func TestMerge_AnchorsPointAtFeatureKey(t *testing.T) {
	t.Parallel()

	src := `{
  "name": "test",
  "features": {
    "./f": {}
  }
}`
	root := mergeSrc(t, src, map[string]string{
		"f": `{"id": "f", "privileged": true, "containerEnv": {"KEY": "value"}}`,
	})
	anchor := strings.Index(src, `"./f"`)
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
			t.Errorf("%s StartOffset = %d, want %d (the feature key)", ptr, v.StartOffset, anchor)
		}
	}

	// The user's own nodes keep their original positions.
	if v := root.Find("/name"); v == nil || v.StartOffset != strings.Index(src, `"test"`) {
		t.Errorf("/name StartOffset changed: %+v", v)
	}
}

// mergeSrc parses src as a devcontainer.json, writes each named feature under a temporary
// directory (referenced as "./<name>"), and merges. It returns the merged tree and the source, so
// callers can locate anchors in it.
func mergeSrc(t *testing.T, src string, features map[string]string) *hujson.Value {
	t.Helper()
	dir := t.TempDir()
	for name, content := range features {
		writeLocalFeature(t, dir, name, content)
	}
	root, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse devcontainer.json: %v", err)
	}
	if err := Merge(t.Context(), NewFetcher(), openRoot(t, dir), ".", &root); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return &root
}

// mergeFiles parses src as a devcontainer.json, writes each named plain file (e.g. a Dockerfile
// or a Compose file, possibly in a subdirectory) under a temporary directory, and merges with that
// directory as both the root and the config directory.
func mergeFiles(t *testing.T, src string, files map[string]string) *hujson.Value {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, files)
	root, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse devcontainer.json: %v", err)
	}
	if err := Merge(t.Context(), NewFetcher(), openRoot(t, dir), ".", &root); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return &root
}

// writeFiles writes each named file under dir, creating intermediate directories.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// assertJSON compares the merged tree, reduced to standard JSON, against want.
func assertJSON(t *testing.T, root *hujson.Value, want string) {
	t.Helper()
	clone := root.Clone()
	clone.Standardize()
	var got, wantVal any
	if err := json.Unmarshal(clone.Pack(), &got); err != nil {
		t.Fatalf("unmarshal merged tree: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if diff := cmp.Diff(wantVal, got); diff != "" {
		t.Errorf("merged configuration mismatch (-want +got):\n%s\n%s", diff, clone.Pack())
	}
}

// resolveOrder writes each named Feature under a temp directory (referenced as "./<name>"), resolves
// the devcontainer.json in src through installSequence, and returns the contributors in installation
// order. It asserts on the resolved contributors directly, at a finer grain than the merged tree.
func resolveOrder(t *testing.T, src string, features map[string]string) []*contributor {
	t.Helper()
	dir := t.TempDir()
	for name, content := range features {
		writeLocalFeature(t, dir, name, content)
	}
	root, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse devcontainer.json: %v", err)
	}
	ordered, err := installSequence(t.Context(), NewFetcher(), openRoot(t, dir), ".", &root)
	if err != nil {
		t.Fatalf("installSequence: %v", err)
	}
	return ordered
}
