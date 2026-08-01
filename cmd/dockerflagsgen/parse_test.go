package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/dockerargs"
	"github.com/spf13/pflag"
)

// TestParse checks dockerargs.Parse against the parser it models, on argvs built to hit the entry
// forms and the adjacencies that tell the two apart. It lives in this module because decolint does
// not depend on pflag: it is the argument parser docker/cli happens to use, not a contract the
// linter should carry a dependency for.
//
// The argvs use only flags dockerargs.RunFlags names and never the "--" terminator, the two places
// Parse is documented to part from pflag. Both readings of an unrecognized flag are a deliberate
// deviation as well — pflag rejects the argv outright — and are covered by the package's own tests.
func TestParse(t *testing.T) {
	const iterations = 20000
	rng := rand.New(rand.NewPCG(1, 2))

	for i := range iterations {
		argv := randomArgv(rng)
		want, err := pflagSets(argv)
		// Any other error stops pflag's parse partway, which would compare Parse against a
		// truncated reading instead of a whole one.
		if err != nil && !strings.Contains(err.Error(), "flag needs an argument") {
			t.Fatalf("argv %d %q: pflag stopped early: %v", i, argv, err)
		}

		var got []setCall
		for _, arg := range dockerargs.Parse(argv) {
			got = append(got, setCall{flag: arg.Flag, value: arg.Value})
		}

		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("argv %d %q:\n got %v\nwant %v", i, argv, got, want)
		}
	}
}

// setCall is one call pflag makes on a flag's value while parsing an argv.
type setCall struct{ flag, value string }

func (c setCall) String() string { return c.flag + "=" + c.value }

// recorder is a pflag.Value that accepts every value and records the call. Recording is what the
// comparison needs, and accepting keeps the real value types' own validation — which decolint does
// not model — out of the picture.
type recorder struct {
	flag  string
	typ   string
	calls *[]setCall
}

func (r *recorder) String() string { return "" }
func (r *recorder) Type() string   { return r.typ }

func (r *recorder) Set(value string) error {
	*r.calls = append(*r.calls, setCall{flag: r.flag, value: value})
	return nil
}

// pflagSets parses argv with a pflag.FlagSet built from dockerargs.RunFlags and returns the values
// it assigned, in order.
//
// The set stays interspersed, unlike "docker run"'s: stopping at the first operand is the behavior
// Parse deliberately drops, so leaving it on here would report that deviation as a difference on
// every argv that has one.
func pflagSets(argv []string) ([]setCall, error) {
	var calls []setCall
	fs := pflag.NewFlagSet("run", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for _, f := range dockerargs.RunFlags {
		fs.VarP(&recorder{flag: f.Name, typ: f.Type, calls: &calls}, f.Name, f.Shorthand, "")
		fs.Lookup(f.Name).NoOptDefVal = f.NoOptDefVal
	}
	return calls, fs.Parse(argv) //nolint:wrapcheck // the caller only classifies the error.
}

// values are the entries an argv can hold besides a flag: operands, and the strings that pose as a
// flag to a reader that does not know an earlier flag consumed them.
var values = []string{"", "x", "ALL", "=", "a=b", "-", "-v", "-itv", "--privileged", "--cap-drop=ALL"}

func randomArgv(rng *rand.Rand) []string {
	argv := make([]string, 0, 6)
	for range rng.IntN(6) + 1 {
		argv = append(argv, randomEntry(rng))
	}
	return argv
}

func randomEntry(rng *rand.Rand) string {
	f := dockerargs.RunFlags[rng.IntN(len(dockerargs.RunFlags))]
	value := values[rng.IntN(len(values))]

	forms := []string{
		"--" + f.Name,
		"--" + f.Name + "=" + value,
		value,
		randomShorthands(rng),
	}
	if f.Shorthand != "" {
		forms = append(forms, "-"+f.Shorthand)
		if f.TakesValue() {
			forms = append(forms, "-"+f.Shorthand+value, "-"+f.Shorthand+"="+value)
		} else {
			// Attaching anything else to a flag that takes no value leaves it to be read as more
			// shorthands, so "-i--privileged" reaches pflag's unknown-shorthand error and ends its
			// parse — one more place Parse deliberately reads on.
			forms = append(forms, "-"+f.Shorthand+"=true", "-"+f.Shorthand+"=false")
		}
	}
	return forms[rng.IntN(len(forms))]
}

// randomShorthands returns a run of shorthands in a single entry, "-itv" and the like.
func randomShorthands(rng *rand.Rand) string {
	var shorthands []string
	for _, f := range dockerargs.RunFlags {
		if f.Shorthand != "" {
			shorthands = append(shorthands, f.Shorthand)
		}
	}
	var b strings.Builder
	b.WriteString("-")
	for range rng.IntN(4) + 1 {
		b.WriteString(shorthands[rng.IntN(len(shorthands))])
	}
	return b.String()
}
