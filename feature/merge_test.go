package feature

import (
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tailscale/hujson"
)

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

func TestMergeNoFeatures(t *testing.T) {
	t.Parallel()

	src := `{"image": "ubuntu:24.04"}`
	root := mergeSrc(t, src, nil)
	assertJSON(t, root, src)
}

func TestMergeBooleanOr(t *testing.T) {
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

func TestMergeUnionArrays(t *testing.T) {
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

func TestMergeContainerEnv(t *testing.T) {
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

func TestMergeMounts(t *testing.T) {
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

func TestMergeCustomizations(t *testing.T) {
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

func TestMergeLifecycleHooks(t *testing.T) {
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
}

func TestMergeDependsOn(t *testing.T) {
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

func TestMergeAnchorsDeclaredDependencyAtOwnKey(t *testing.T) {
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

func TestMergeDependsOnCycle(t *testing.T) {
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

func TestMergeFetchFailure(t *testing.T) {
	t.Parallel()

	root, err := hujson.Parse([]byte(`{"features": {"./missing": {}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := Merge(t.Context(), NewFetcher(), openRoot(t, t.TempDir()), ".", &root); err == nil {
		t.Error("Merge with an unresolvable feature: got nil error")
	}
}

func TestMergeInstallsAfter(t *testing.T) {
	t.Parallel()

	// b declares installsAfter a, so a installs first and b wins the conflict even though b is
	// declared first.
	root := mergeSrc(t,
		`{"features": {"./b": {}, "./a": {}}}`,
		map[string]string{
			"a": `{"id": "a", "containerEnv": {"SHARED": "a"}}`,
			"b": `{"id": "b", "installsAfter": ["a"], "containerEnv": {"SHARED": "b"}}`,
		})
	assertJSON(t, root, `{
	  "features": {"./b": {}, "./a": {}},
	  "containerEnv": {"SHARED": "b"}
	}`)
}

func TestMergeOverrideFeatureInstallOrder(t *testing.T) {
	t.Parallel()

	// The override moves b to the front, so a installs later and wins the conflict.
	root := mergeSrc(t,
		`{
		  "overrideFeatureInstallOrder": ["b"],
		  "features": {"./a": {}, "./b": {}}
		}`,
		map[string]string{
			"a": `{"id": "a", "containerEnv": {"SHARED": "a"}}`,
			"b": `{"id": "b", "containerEnv": {"SHARED": "b"}}`,
		})
	assertJSON(t, root, `{
	  "overrideFeatureInstallOrder": ["b"],
	  "features": {"./a": {}, "./b": {}},
	  "containerEnv": {"SHARED": "a"}
	}`)
}

func TestMergeAnchorsPointAtFeatureKey(t *testing.T) {
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
