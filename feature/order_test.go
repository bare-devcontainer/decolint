package feature

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tailscale/hujson"
	"oras.land/oras-go/v2/registry"
)

func TestInstallOrder(t *testing.T) {
	t.Parallel()

	// node builds a minimal contributor whose KindLocal identity makes compareTo order nodes by ref,
	// so a round's order is deterministic.
	node := func(ref string) *contributor {
		return &contributor{ref: ref, kind: KindLocal, resolvedPath: ref}
	}

	tests := []struct {
		name    string
		build   func() []*contributor
		want    []string
		wantErr bool
	}{
		{
			name:  "independent nodes ordered by tiebreak",
			build: func() []*contributor { return []*contributor{node("c"), node("a"), node("b")} },
			want:  []string{"a{}", "b{}", "c{}"},
		},
		{
			name: "hard dependency precedes dependent",
			build: func() []*contributor {
				a, b := node("a"), node("b")
				a.dependsOn = []*contributor{b}
				return []*contributor{a, b}
			},
			want: []string{"b{}", "a{}"},
		},
		{
			name: "soft dependency orders after",
			build: func() []*contributor {
				a, b := node("a"), node("b")
				a.installsAfter = []*contributor{b}
				return []*contributor{a, b}
			},
			want: []string{"b{}", "a{}"},
		},
		{
			// A soft dependency on a Feature absent from the merge is dropped, so it neither gates nor
			// delays: a stays eligible in the first round alongside the independent z.
			name: "absent soft dependency ignored",
			build: func() []*contributor {
				a, z := node("a"), node("z")
				a.installsAfter = []*contributor{node("x")} // x is not among the nodes
				return []*contributor{a, z}
			},
			want: []string{"a{}", "z{}"},
		},
		{
			name: "hard chain spans rounds",
			build: func() []*contributor {
				a, b, c := node("a"), node("b"), node("c")
				a.dependsOn = []*contributor{b}
				b.dependsOn = []*contributor{c}
				return []*contributor{a, b, c}
			},
			want: []string{"c{}", "b{}", "a{}"},
		},
		{
			// roundPriority raises b into the first round and requeues the lower-priority a and c to the
			// next, even though a would otherwise sort first by tiebreak.
			name: "round priority raises node and requeues the rest",
			build: func() []*contributor {
				a, b, c := node("a"), node("b"), node("c")
				b.roundPriority = 1
				return []*contributor{a, b, c}
			},
			want: []string{"b{}", "a{}", "c{}"},
		},
		{
			// Priority never overrides a hard edge: b outranks a but depends on it, so a still installs
			// first.
			name: "round priority cannot precede own hard dependency",
			build: func() []*contributor {
				a, b := node("a"), node("b")
				b.roundPriority = 1
				b.dependsOn = []*contributor{a}
				return []*contributor{a, b}
			},
			want: []string{"a{}", "b{}"},
		},
		{
			name: "hard dependency cycle fails",
			build: func() []*contributor {
				a, b := node("a"), node("b")
				a.dependsOn = []*contributor{b}
				b.dependsOn = []*contributor{a}
				return []*contributor{a, b}
			},
			wantErr: true,
		},
		{
			name: "soft dependency cycle fails",
			build: func() []*contributor {
				a, b := node("a"), node("b")
				a.installsAfter = []*contributor{b}
				b.installsAfter = []*contributor{a}
				return []*contributor{a, b}
			},
			wantErr: true,
		},
		{
			// A round of differing source types is ordered by compareTo's cross-type fallback, the
			// requested reference string: "./local" < "reg.example.invalid/ns/x" < "tarball:z".
			name: "mixed source types ordered by reference",
			build: func() []*contributor {
				local := &contributor{ref: "./local", kind: KindLocal, resolvedPath: "./local"}
				tarball := &contributor{ref: "tarball:z", kind: KindTarball, tarballURI: "tarball:z"}
				oci := &contributor{ref: "reg.example.invalid/ns/x", kind: KindOCI, digest: "sha256:abc"}
				return []*contributor{tarball, oci, local}
			},
			want: []string{"./local{}", "reg.example.invalid/ns/x@sha256:abc{}", "tarball:z{}"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := installOrder(tt.build())
			if tt.wantErr {
				if err == nil {
					t.Fatal("installOrder: got nil error, want cycle error")
				}
				return
			}
			if err != nil {
				t.Fatalf("installOrder: %v", err)
			}
			assertOrder(t, got, tt.want)
		})
	}
}

func TestInstallOrder_OCIOrder(t *testing.T) {
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
		"reg.example.invalid/codspace/dependson/D@" + digestD + "{magicNumber=30}",
		"reg.example.invalid/codspace/dependson/E@" + digestE + "{magicNumber=50}",
		"reg.example.invalid/codspace/dependson/a@" + digestA + "{magicNumber=10}",
		"reg.example.invalid/codspace/dependson/A@" + digestA + "{magicNumber=40}",
		"reg.example.invalid/codspace/dependson/C@" + digestC + "{magicNumber=20}",
		"reg.example.invalid/codspace/dependson/b@" + digestB + "{magicNumber=400}",
	})
}

func TestInstallOrder_TarballOrder(t *testing.T) {
	t.Parallel()

	const host = "https://example.invalid/features/"
	b := tarballContrib(host+"b.tgz", map[string]string{"magicNumber": "400"})
	a := tarballContrib(host+"a.tgz", map[string]string{"magicNumber": "10"})
	c := tarballContrib(host+"c.tgz", map[string]string{"magicNumber": "20"})
	d := tarballContrib(host+"d.tgz", map[string]string{"magicNumber": "30"})
	e := tarballContrib(host+"e.tgz", map[string]string{"magicNumber": "50"})
	aDep := tarballContrib(host+"a.tgz", map[string]string{"magicNumber": "40"})

	b.dependsOn = []*contributor{c, d}
	a.dependsOn = []*contributor{e}
	c.dependsOn = []*contributor{aDep, e}
	aDep.dependsOn = []*contributor{e}

	got, err := installOrder([]*contributor{b, a, c, d, e, aDep})
	if err != nil {
		t.Fatalf("installOrder: %v", err)
	}
	assertOrder(t, got, []string{
		host + "d.tgz{magicNumber=30}",
		host + "e.tgz{magicNumber=50}",
		host + "a.tgz{magicNumber=10}",
		host + "a.tgz{magicNumber=40}",
		host + "c.tgz{magicNumber=20}",
		host + "b.tgz{magicNumber=400}",
	})
}

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
			ref:     "reg.example.invalid/" + repo,
			kind:    KindOCI,
			ociRef:  registry.Reference{Registry: "reg.example.invalid", Repository: repo, Reference: tag},
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
		{"tarball by uri", tarball("https://example.invalid/a.tgz", nil), tarball("https://example.invalid/b.tgz", nil), -1},
		{"tarball by options", tarball("https://example.invalid/a.tgz", map[string]string{"v": "1"}), tarball("https://example.invalid/a.tgz", map[string]string{"v": "2"}), -1},
		{"oci by resource id", oci("ns/a", "", "sha256:1", nil), oci("ns/b", "", "sha256:2", nil), -1},
		{"oci by tag", oci("ns/a", "1.0.0", "sha256:1", nil), oci("ns/a", "2.0.0", "sha256:2", nil), -1},
		// Tags compare lexicographically, not by semantic version: "10" sorts before "9" because
		// '1' < '9'. The 1.0.0/2.0.0 case above agrees under either interpretation, so this pins the
		// dictionary order the reference implementation actually uses.
		{"oci tag lexical not semver", oci("ns/a", "10", "sha256:1", nil), oci("ns/a", "9", "sha256:2", nil), -1},
		// "latest" is an ordinary tag string, ordered lexicographically against a numeric tag rather
		// than treated as newest: 'l' > '9', so it sorts after.
		{"oci tag latest sorts lexically after numeric", oci("ns/a", "latest", "sha256:1", nil), oci("ns/a", "9", "sha256:2", nil), 1},
		// An unversioned reference parses to the "latest" tag, so it participates in the tag comparison
		// rather than falling through to the digest tiebreak (which would order it before "1").
		{"oci unversioned normalized to latest", oci("ns/a", "latest", "sha256:1", nil), oci("ns/a", "1", "sha256:2", nil), 1},
		{"oci by digest tiebreak", oci("ns/a", "1", "sha256:1", nil), oci("ns/a", "1", "sha256:2", nil), -1},
		// A digest-pinned reference carries no tag (the digest's algorithm separator disqualifies it),
		// so it drops out of the tag comparison and ordering falls through to the digest tiebreak even
		// against a tagged reference for the same resource.
		{"oci digest-pinned vs tagged falls to digest", oci("ns/a", "sha256:1", "sha256:1", nil), oci("ns/a", "2", "sha256:2", nil), -1},
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

func TestOptionsCompareTo(t *testing.T) {
	t.Parallel()

	str := func(s string) optionValue { return optionValue{kind: kindString, str: s} }
	boolean := func(b bool) optionValue { return optionValue{kind: kindBool, b: b} }
	obj := func(m map[string]optScalar) optionValue { return optionValue{kind: kindObject, obj: m} }
	sScalar := func(s string) optScalar { return optScalar{kind: kindString, str: s} }
	bScalar := func(b bool) optScalar { return optScalar{kind: kindBool, b: b} }
	undef := optScalar{kind: kindUndefined}

	tests := []struct {
		name string
		a, b optionValue
		want int // expected sign
	}{
		{"string less", str("a"), str("b"), -1},
		{"string equal", str("a"), str("a"), 0},
		{"bool false before true", boolean(false), boolean(true), -1},
		{"bool equal", boolean(true), boolean(true), 0},
		{"object by size", obj(map[string]optScalar{"x": sScalar("1")}), obj(map[string]optScalar{}), 1},
		{"object by key", obj(map[string]optScalar{"a": sScalar("1")}), obj(map[string]optScalar{"b": sScalar("1")}), -1},
		{"object by string scalar", obj(map[string]optScalar{"a": sScalar("1")}), obj(map[string]optScalar{"a": sScalar("2")}), -1},
		{"object by bool scalar", obj(map[string]optScalar{"a": bScalar(false)}), obj(map[string]optScalar{"a": bScalar(true)}), -1},
		{"object equal", obj(map[string]optScalar{"a": sScalar("1")}), obj(map[string]optScalar{"a": sScalar("1")}), 0},
		{"undefined scalar sorts after defined", obj(map[string]optScalar{"a": undef}), obj(map[string]optScalar{"a": sScalar("1")}), 1},
		{"undefined scalars equal", obj(map[string]optScalar{"a": undef}), obj(map[string]optScalar{"a": undef}), 0},
		{"mismatched defined scalar kinds compare equal", obj(map[string]optScalar{"a": sScalar("1")}), obj(map[string]optScalar{"a": bScalar(true)}), 0},
		{"cross kind object before string", obj(map[string]optScalar{}), str("a"), -1},
		{"cross kind boolean before object", boolean(true), obj(map[string]optScalar{}), -1},
		{"cross kind boolean before string", boolean(true), str("a"), -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sign(optionsCompareTo(tt.a, tt.b)); got != tt.want {
				t.Errorf("optionsCompareTo(%s) sign = %d, want %d", tt.name, got, tt.want)
			}
			if got := sign(optionsCompareTo(tt.b, tt.a)); got != -tt.want {
				t.Errorf("optionsCompareTo reversed(%s) sign = %d, want %d", tt.name, got, -tt.want)
			}
		})
	}
}

func TestParseOptions(t *testing.T) {
	t.Parallel()

	parse := func(src string) optionValue {
		v, err := hujson.Parse([]byte(src))
		if err != nil {
			t.Fatalf("hujson.Parse(%q): %v", src, err)
		}
		return parseOptions(v)
	}

	if got := parse(`"x"`); got.kind != kindString || got.str != "x" {
		t.Errorf(`parseOptions("x") = %+v, want string "x"`, got)
	}
	if got := parse(`true`); got.kind != kindBool || !got.b {
		t.Errorf("parseOptions(true) = %+v, want boolean true", got)
	}
	// A non-scalar, non-object value (a bare number) is treated as the empty option set.
	if got := parse(`42`); got.kind != kindObject || len(got.obj) != 0 {
		t.Errorf("parseOptions(42) = %+v, want empty object", got)
	}

	obj := parse(`{"s": "v", "b": false, "n": [1], "z": null}`)
	if obj.kind != kindObject {
		t.Fatalf("parseOptions(object) kind = %d, want kindObject", obj.kind)
	}
	want := map[string]optScalar{
		"s": {kind: kindString, str: "v"},
		"b": {kind: kindBool, b: false},
		// A non-scalar member value normalizes to undefined.
		"n": {kind: kindUndefined},
		"z": {kind: kindUndefined},
	}
	if diff := cmp.Diff(want, obj.obj, cmp.AllowUnexported(optScalar{})); diff != "" {
		t.Errorf("parseOptions(object) mismatch (-want +got):\n%s", diff)
	}
}

func TestOCIRefHelpers(t *testing.T) {
	t.Parallel()

	if got := ociNamespace("a"); got != "" {
		t.Errorf(`ociNamespace("a") = %q, want ""`, got)
	}
	if got := ociID("a"); got != "a" {
		t.Errorf(`ociID("a") = %q, want "a"`, got)
	}
	if got := ociTag(registry.Reference{Reference: "sha256:abc"}); got != "" {
		t.Errorf(`ociTag(digest) = %q, want ""`, got)
	}
	if got := ociTag(registry.Reference{Reference: "1.2.3"}); got != "1.2.3" {
		t.Errorf(`ociTag(tag) = %q, want "1.2.3"`, got)
	}
}

func TestSatisfiesSoftDependency(t *testing.T) {
	t.Parallel()

	tarball := func(uri string) *contributor {
		return &contributor{kind: KindTarball, tarballURI: uri}
	}
	local := func(path string) *contributor {
		return &contributor{kind: KindLocal, resolvedPath: path}
	}
	oci := func(repo string, aliases []string) *contributor {
		return &contributor{
			kind:    KindOCI,
			ociRef:  registry.Reference{Registry: "reg.example.invalid", Repository: repo},
			aliases: aliases,
		}
	}

	tests := []struct {
		name       string
		node, soft *contributor
		want       bool
	}{
		{"different kinds never match", tarball("https://example.invalid/a.tgz"), local("./a"), false},
		{"tarball match", tarball("https://example.invalid/a.tgz"), tarball("https://example.invalid/a.tgz"), true},
		{"tarball mismatch", tarball("https://example.invalid/a.tgz"), tarball("https://example.invalid/b.tgz"), false},
		{"local match", local("./a"), local("./a"), true},
		{"local mismatch", local("./a"), local("./b"), false},
		{"oci exact resource", oci("ns/a", nil), oci("ns/a", nil), true},
		{"oci legacy alias", oci("ns/old", nil), oci("ns/new", []string{"old"}), true},
		{"oci no match", oci("ns/a", nil), oci("ns/b", []string{"c"}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := satisfiesSoftDependency(tt.node, tt.soft); got != tt.want {
				t.Errorf("satisfiesSoftDependency(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestDisplayID(t *testing.T) {
	t.Parallel()

	withID := &contributor{ref: "reg.example.invalid/ns/a", md: &Metadata{ID: "a"}}
	if got := withID.displayID(); got != "a" {
		t.Errorf("displayID with metadata id = %q, want %q", got, "a")
	}

	noMetadata := &contributor{ref: "reg.example.invalid/ns/a"}
	if got := noMetadata.displayID(); got != "reg.example.invalid/ns/a" {
		t.Errorf("displayID without metadata = %q, want %q", got, "reg.example.invalid/ns/a")
	}
}

// ociContrib builds an OCI contributor under reg.example.invalid/codspace/dependson. userID is the identifier as
// written in the referencing Feature (which may differ in case), repoID is the lowercase resource
// segment used for comparison, and the alias is the published metadata id (uppercase).
func ociContrib(userID, repoID, digest string, opts map[string]string) *contributor {
	return &contributor{
		ref:     "reg.example.invalid/codspace/dependson/" + userID,
		kind:    KindOCI,
		ociRef:  registry.Reference{Registry: "reg.example.invalid", Repository: "codspace/dependson/" + repoID},
		digest:  digest,
		aliases: []string{strings.ToUpper(repoID)},
		options: optionsOf(opts),
	}
}

// tarballContrib builds a tarball contributor requested by uri with the given options.
func tarballContrib(uri string, opts map[string]string) *contributor {
	return &contributor{ref: uri, kind: KindTarball, tarballURI: uri, options: optionsOf(opts)}
}

func optionsOf(m map[string]string) optionValue {
	obj := map[string]optScalar{}
	for k, v := range m {
		obj[k] = optScalar{kind: kindString, str: v}
	}
	return optionValue{kind: kindObject, obj: obj}
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
		if v.kind == kindString {
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
