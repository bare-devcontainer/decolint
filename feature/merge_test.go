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

func TestMergeMountsUnparsableTargetPassthrough(t *testing.T) {
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

func TestMergeOverrideFeatureInstallOrder(t *testing.T) {
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

// TestMergeOverrideFeatureInstallOrderOCILegacyAlias covers the alias branch of applyOverride: an
// "overrideFeatureInstallOrder" entry that names a renamed OCI Feature by its current id must still
// match a contributor requested under a legacy id, through the Feature's declared "legacyIds".
func TestMergeOverrideFeatureInstallOrderOCILegacyAlias(t *testing.T) {
	t.Parallel()

	host := startOCIRegistry(t)
	// The renamed Feature declares its legacy id and is published under both paths, so a reference by
	// either the current id ("renamed", used in the override) or the legacy id ("legacy", used in
	// "features") resolves to the same identity.
	renamed := archiveWithMetadata(t, `{"id": "renamed", "legacyIds": ["legacy"], "containerEnv": {"SHARED": "renamed"}}`, false)
	pushOCIFeature(t, host, "features/renamed", "1", renamed, false)
	pushOCIFeature(t, host, "features/legacy", "1", renamed, false)
	pushOCIFeature(t, host, "features/aaa", "1",
		archiveWithMetadata(t, `{"id": "aaa", "containerEnv": {"SHARED": "aaa"}}`, false), false)

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
