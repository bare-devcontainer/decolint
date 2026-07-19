package feature

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tailscale/hujson"
	"oras.land/oras-go/v2/registry"
)

// The install-order tests mirror the official devcontainers/cli suite
// (src/test/container-features/configs/feature-dependencies). Local (file-path) cases run through
// the real resolve-and-order path against on-disk fixtures; the OCI case is exercised as a pure
// installOrder call over hand-built nodes (identical to the graph the CLI resolves from published
// Features), so it needs no network. The CLI's v1 "github-repo" cases are omitted: decolint does not
// model that legacy source type.

// resolveOrder writes each named Feature under a temp directory (referenced as "./<name>"), resolves
// the devcontainer.json in src, and returns the contributors in installation order.
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

// itemKey identifies a contributor in an ordering assertion by its reference, OCI digest (when set),
// and string options, matching the (userFeatureId, canonicalId, options) tuples the CLI asserts.
func itemKey(c *contributor) string {
	key := c.ref
	if c.digest != "" {
		key += "@" + c.digest
	}
	var opts []string
	for k, v := range c.options.obj {
		if v.kind == 's' {
			opts = append(opts, k+"="+v.str)
		}
	}
	slices.Sort(opts)
	return key + "{" + strings.Join(opts, ",") + "}"
}

func assertOrder(t *testing.T, got []*contributor, want []string) {
	t.Helper()
	keys := make([]string, len(got))
	for i, c := range got {
		keys[i] = itemKey(c)
	}
	if diff := cmp.Diff(want, keys); diff != "" {
		t.Errorf("install order mismatch (-want +got):\n%s", diff)
	}
}

func TestInstallOrderInstallsAfterLocal(t *testing.T) {
	t.Parallel()

	got := resolveOrder(t,
		`{"features": {"./a": {}, "./c": {"magicNumber": "321"}}}`,
		map[string]string{
			"a": `{"id": "a", "installsAfter": ["./b", "./c"]}`,
			"b": `{"id": "b"}`,
			"c": `{"id": "c"}`,
		})
	assertOrder(t, got, []string{"./c{magicNumber=321}", "./a{}"})
}

func TestInstallOrderDependsOnLocal(t *testing.T) {
	t.Parallel()

	got := resolveOrder(t,
		`{"features": {"./a": {}}}`,
		map[string]string{
			"a": `{"id": "a", "dependsOn": {"./b": {"magicNumber": "50"}}}`,
			"b": `{"id": "b"}`,
		})
	assertOrder(t, got, []string{"./b{magicNumber=50}", "./a{}"})
}

// TestInstallOrderDependsOnLocalWithOptions covers round sorting by options: the same Feature
// requested with different options is a distinct contributor, and a round of them is ordered by the
// specification's options comparison.
func TestInstallOrderDependsOnLocalWithOptions(t *testing.T) {
	t.Parallel()

	got := resolveOrder(t,
		`{"features": {"./a": {"optA": "a", "optB": "b"}, "./b": {"optA": "a", "optB": "b"}}}`,
		map[string]string{
			"a": `{"id": "a", "dependsOn": {"./b": {"optA": "a", "optB": "a"}, "./c": {}}}`,
			"b": `{"id": "b"}`,
			"c": `{"id": "c", "dependsOn": {"./b": {"optA": "b", "optB": "a"}, "./d": {}, "./e": {}}}`,
			"d": `{"id": "d", "dependsOn": {"./b": {"optA": "b", "optB": "b"}}}`,
			"e": `{"id": "e", "dependsOn": {"./b": {}}}`,
		})
	assertOrder(t, got, []string{
		"./b{}",
		"./b{optA=a,optB=a}",
		"./b{optA=a,optB=b}",
		"./b{optA=b,optB=a}",
		"./b{optA=b,optB=b}",
		"./d{}",
		"./e{}",
		"./c{}",
		"./a{optA=a,optB=b}",
	})
}

func TestInstallOrderDependsOnAndInstallsAfterLocal(t *testing.T) {
	t.Parallel()

	got := resolveOrder(t,
		`{"features": {"./a": {}}}`,
		map[string]string{
			"a": `{"id": "a", "installsAfter": ["./b"], "dependsOn": {"./c": {"magicNumber": "321"}}}`,
			"b": `{"id": "b"}`,
			"c": `{"id": "c"}`,
		})
	assertOrder(t, got, []string{"./c{magicNumber=321}", "./a{}"})
}

func TestInstallOrderOverrideLocalSimple(t *testing.T) {
	t.Parallel()

	got := resolveOrder(t,
		`{"features": {"./a": {}}, "overrideFeatureInstallOrder": ["./c"]}`,
		map[string]string{
			"a": `{"id": "a", "dependsOn": {"./b": {}, "./c": {}, "./d": {}}}`,
			"b": `{"id": "b"}`,
			"c": `{"id": "c"}`,
			"d": `{"id": "d"}`,
		})
	assertOrder(t, got, []string{"./c{}", "./b{}", "./d{}", "./a{}"})
}

func TestInstallOrderOverrideLocalIntermediate(t *testing.T) {
	t.Parallel()

	got := resolveOrder(t,
		`{"features": {"./a": {}}, "overrideFeatureInstallOrder": ["./c", "./d"]}`,
		map[string]string{
			"a": `{"id": "a", "dependsOn": {"./b": {}, "./d": {}}, "installsAfter": ["./c"]}`,
			"b": `{"id": "b", "dependsOn": {"./c": {}}}`,
			"c": `{"id": "c"}`,
			"d": `{"id": "d"}`,
		})
	assertOrder(t, got, []string{"./c{}", "./d{}", "./b{}", "./a{}"})
}

// TestInstallOrderOverrideLocalRoundPriority covers roundPriority beating an independent, otherwise
// eligible Feature: c is installable in the first round but a (higher priority) and then b (which
// depends on a) install before it.
func TestInstallOrderOverrideLocalRoundPriority(t *testing.T) {
	t.Parallel()

	got := resolveOrder(t,
		`{"features": {"./b": {}, "./c": {}}, "overrideFeatureInstallOrder": ["./a", "./b", "./c"]}`,
		map[string]string{
			"a": `{"id": "a"}`,
			"b": `{"id": "b", "dependsOn": {"./a": {}}}`,
			"c": `{"id": "c"}`,
		})
	assertOrder(t, got, []string{"./a{}", "./b{}", "./c{}"})
}

func TestInstallOrderInstallsAfterCycle(t *testing.T) {
	t.Parallel()

	_, err := installSequenceOf(t,
		`{"features": {"./a": {}, "./b": {}, "./c": {}}}`,
		map[string]string{
			"a": `{"id": "a", "installsAfter": ["./b"]}`,
			"b": `{"id": "b", "installsAfter": ["./c"]}`,
			"c": `{"id": "c", "installsAfter": ["./a"]}`,
		})
	if err == nil {
		t.Fatal("installSequence with an installsAfter cycle: got nil error")
	}
}

func TestInstallOrderDependsOnCycle(t *testing.T) {
	t.Parallel()

	_, err := installSequenceOf(t,
		`{"features": {"./a": {}}}`,
		map[string]string{
			"a": `{"id": "a", "dependsOn": {"./b": {}}}`,
			"b": `{"id": "b", "dependsOn": {"./c": {"magicNumber": "50"}}}`,
			"c": `{"id": "c", "dependsOn": {"./a": {"magicNumber": "50"}}}`,
		})
	if err == nil {
		t.Fatal("installSequence with a dependsOn cycle: got nil error")
	}
}

// installSequenceOf resolves src against on-disk fixtures and returns the ordering result and error,
// for cases that assert on failure.
func installSequenceOf(t *testing.T, src string, features map[string]string) ([]*contributor, error) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range features {
		writeLocalFeature(t, dir, name, content)
	}
	root, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse devcontainer.json: %v", err)
	}
	return installSequence(t.Context(), NewFetcher(), openRoot(t, dir), ".", &root)
}

// TestInstallOrderDependsOnOCI ports the CLI's "valid dependsOn with published oci Features" case
// (dependsOn/oci-ab). The graph is the published codspace/dependson family:
//
//	a(A) -> E ;  b(B) -> C, D ;  c(C) -> A, E ;  d(D), e(E) -> (none)
//
// It exercises OCI comparison: two requests for the same canonical Feature (a@… with options 10 and
// 40) are distinct but adjacent, and a round is ordered by resource id then options.
func TestInstallOrderDependsOnOCI(t *testing.T) {
	t.Parallel()

	const (
		digestA = "sha256:932027ef71da186210e6ceb3294c3459caaf6b548d2b547d5d26be3fc4b2264a"
		digestB = "sha256:e7e6b52884ae7f349baf207ac59f78857ab64529c890b646bb0282f962bb2941"
		digestC = "sha256:db651708398b6d7af48f184c358728eaaf959606637133413cb4107b8454a868"
		digestD = "sha256:3795caa1e32ba6b30a08260039804eed6f3cf40811f0c65c118437743fa15ce8"
		digestE = "sha256:9f36f159c70f8bebff57f341904b030733adb17ef12a5d58d4b3d89b2a6c7d5a"
	)

	b := ociContrib("b", "b", digestB, map[string]string{"magicNumber": "400"})
	a := ociContrib("a", "a", digestA, map[string]string{"magicNumber": "10"})
	c := ociContrib("C", "c", digestC, map[string]string{"magicNumber": "20"})
	d := ociContrib("D", "d", digestD, map[string]string{"magicNumber": "30"})
	e := ociContrib("E", "e", digestE, map[string]string{"magicNumber": "50"})
	aDep := ociContrib("A", "a", digestA, map[string]string{"magicNumber": "40"})

	b.dependsOn = []*contributor{c, d}
	a.dependsOn = []*contributor{e}
	c.dependsOn = []*contributor{aDep, e}
	aDep.dependsOn = []*contributor{e}

	got, err := installOrder([]*contributor{b, a, c, d, e, aDep})
	if err != nil {
		t.Fatalf("installOrder: %v", err)
	}
	assertOrder(t, got, []string{
		"ghcr.io/codspace/dependson/D@" + digestD + "{magicNumber=30}",
		"ghcr.io/codspace/dependson/E@" + digestE + "{magicNumber=50}",
		"ghcr.io/codspace/dependson/a@" + digestA + "{magicNumber=10}",
		"ghcr.io/codspace/dependson/A@" + digestA + "{magicNumber=40}",
		"ghcr.io/codspace/dependson/C@" + digestC + "{magicNumber=20}",
		"ghcr.io/codspace/dependson/b@" + digestB + "{magicNumber=400}",
	})
}

// ociContrib builds an OCI contributor under ghcr.io/codspace/dependson. userID is the identifier as
// written in the referencing Feature (which may differ in case), repoID is the lowercase resource
// segment used for comparison, and the alias is the published metadata id (uppercase).
func ociContrib(userID, repoID, digest string, opts map[string]string) *contributor {
	return &contributor{
		ref:     "ghcr.io/codspace/dependson/" + userID,
		kind:    KindOCI,
		ociRef:  registry.Reference{Registry: "ghcr.io", Repository: "codspace/dependson/" + repoID},
		digest:  digest,
		aliases: []string{strings.ToUpper(repoID)},
		options: optionsOf(opts),
	}
}

func optionsOf(m map[string]string) optionValue {
	obj := map[string]optScalar{}
	for k, v := range m {
		obj[k] = optScalar{kind: 's', str: v}
	}
	return optionValue{kind: 'o', obj: obj}
}

// TestCompareTo covers each branch of the specification's ordering comparison, including the
// source-type-specific keys the local and OCI ordering tests do not reach on their own (tarball URI,
// OCI tag, digest tiebreak, and cross-type comparison by reference).
func TestCompareTo(t *testing.T) {
	t.Parallel()

	local := func(path string, opts map[string]string) *contributor {
		return &contributor{ref: path, kind: KindLocal, resolvedPath: path, options: optionsOf(opts)}
	}
	tarball := func(uri string, opts map[string]string) *contributor {
		return &contributor{ref: uri, kind: KindTarball, tarballURI: uri, options: optionsOf(opts)}
	}
	oci := func(repo, tag, digest string, opts map[string]string) *contributor {
		return &contributor{
			ref:     "reg.example.com/" + repo,
			kind:    KindOCI,
			ociRef:  registry.Reference{Registry: "reg.example.com", Repository: repo, Reference: tag},
			digest:  digest,
			options: optionsOf(opts),
		}
	}

	tests := []struct {
		name string
		a, b *contributor
		want int // expected sign of compareTo
	}{
		{"local by path", local("a", nil), local("b", nil), -1},
		{"local by options", local("a", map[string]string{"o": "x"}), local("a", map[string]string{"o": "y"}), -1},
		{"local equal", local("a", map[string]string{"o": "x"}), local("a", map[string]string{"o": "x"}), 0},
		{"tarball by uri", tarball("https://ex/a.tgz", nil), tarball("https://ex/b.tgz", nil), -1},
		{"tarball by options", tarball("https://ex/a.tgz", map[string]string{"v": "1"}), tarball("https://ex/a.tgz", map[string]string{"v": "2"}), -1},
		{"oci by resource id", oci("ns/a", "", "sha256:1", nil), oci("ns/b", "", "sha256:2", nil), -1},
		{"oci by tag", oci("ns/a", "1.0.0", "sha256:1", nil), oci("ns/a", "2.0.0", "sha256:2", nil), -1},
		{"oci by digest tiebreak", oci("ns/a", "1", "sha256:1", nil), oci("ns/a", "1", "sha256:2", nil), -1},
		{"oci equal digest and options", oci("ns/a", "1", "sha256:1", map[string]string{"o": "x"}), oci("ns/a", "9", "sha256:1", map[string]string{"o": "x"}), 0},
		{"cross type by reference", local("./a", nil), tarball("./b", nil), -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sign(compareTo(tt.a, tt.b)); got != tt.want {
				t.Errorf("compareTo(%s) sign = %d, want %d", tt.name, got, tt.want)
			}
			if got := sign(compareTo(tt.b, tt.a)); got != -tt.want {
				t.Errorf("compareTo reversed(%s) sign = %d, want %d", tt.name, got, -tt.want)
			}
		})
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}
