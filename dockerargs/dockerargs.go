// Package dockerargs reads the parts of a devcontainer.json that Docker, rather than the
// devcontainer tooling, gives meaning to:
//
//   - "runArgs", which becomes the argv of the "docker run" command the tooling builds. [Parse] is
//     the single place that knows where a flag's value can be written, so its callers only have to
//     know the values they care about and never which entry of the array holds one.
//   - the values themselves, whose syntax is Docker's wherever they are written: a "securityOpt"
//     entry ([ParseSecurityOpt]), a capability name ([Capability]), a boolean ([IsTrue]).
package dockerargs

import (
	"strconv"
	"strings"

	"github.com/tailscale/hujson"
)

// Flag describes one flag "docker run" registers. The fields mirror pflag, whose parser docker/cli
// uses, closely enough that [Parse] can reproduce its reading of an argv.
type Flag struct {
	// Name is the flag's canonical long name, without the leading "--".
	Name string
	// Shorthand is the flag's one-character short name, without the leading "-", or "" for a flag
	// that has none.
	Shorthand string
	// Type names the kind of value the flag stores, e.g. "bool", "string" or "list".
	Type string
	// NoOptDefVal is the value the flag takes when written without one. A flag that requires a
	// value has none, so an empty NoOptDefVal is how pflag tells the two kinds apart.
	NoOptDefVal string
}

// TakesValue reports whether the flag has to be given a value, either in its own argv entry or by
// consuming the entry that follows.
func (f Flag) TakesValue() bool { return f.NoOptDefVal == "" }

// unknownNoOptDefVal is the value [Parse] gives a flag missing from [RunFlags] when it reads it as
// taking none. Every "docker run" flag that takes no value is a boolean that defaults to this, and
// a flag Docker has added since the table was generated is overwhelmingly likely to be one too.
const unknownNoOptDefVal = "true"

// Arg is one flag occurrence in an argv.
type Arg struct {
	// Flag is the flag's canonical long name, without the leading "--". Every spelling of a flag
	// reduces to it, so "-v", "--volume=x" and "--volume x" all yield "volume".
	Flag string
	// Value is what the argv gives the flag, which for a flag that takes no value is the value it
	// stands for on its own — see [Flag.NoOptDefVal].
	Value string
	// Index is the argv position of the entry Value was read from. That is the flag's own entry
	// whenever it carries the value ("--volume=x", "-vx", a bare "--privileged"), and the following
	// entry otherwise.
	Index int
}

var (
	// flagsByName indexes [RunFlags] by long name.
	flagsByName = func() map[string]Flag {
		m := make(map[string]Flag, len(RunFlags))
		for _, f := range RunFlags {
			m[f.Name] = f
		}
		return m
	}()

	// flagsByShorthand indexes the entries of [RunFlags] that have a shorthand by that shorthand.
	flagsByShorthand = func() map[byte]Flag {
		m := make(map[byte]Flag)
		for _, f := range RunFlags {
			if f.Shorthand != "" {
				m[f.Shorthand[0]] = f
			}
		}
		return m
	}()
)

// Parse returns every flag occurrence in argv, a "docker run" command line, ordered by the argv
// position each value was read from. It recognizes the entry forms pflag does:
//
//   - a value written in the flag's own entry, as "--flag=value", "-fvalue" or "-f=value";
//   - a flag that takes no value written bare, as "--flag" or "-f";
//   - a flag that takes one written bare, consuming the entry that follows;
//   - a run of shorthands in one entry, "-itv", ending at the first one that takes a value.
//
// An entry consumed as a value never names a flag itself.
//
// A flag missing from [RunFlags] is read both ways, as taking no value and as consuming the entry
// that follows: the table can only be older than Docker, never newer. Reading both costs at worst a
// finding Docker would not have seen, where trusting one reading would drop findings silently, on
// exactly the configurations a newly added flag appears in. An unrecognized shorthand names no flag
// to report, so it yields no Arg at all — only the two readings of the entries around it.
//
// Parse deliberately parts from Docker in two places, both of which stop Docker's parse where they
// appear: the "--" terminator, and the image name that ends the flags. It reads on instead. A
// "runArgs" holding either is already broken — it is spliced into an argv that goes on to name the
// image and the flags the devcontainer tooling adds itself, which the entry would displace — so
// reporting what the array says is more use to its author than falling silent on all of it.
func Parse(argv []string) []Arg {
	p := parser{argv: argv, starts: make([]bool, len(argv)+2)}
	p.starts[0] = true
	for i, s := range argv {
		if !p.starts[i] {
			continue
		}
		switch {
		case strings.HasPrefix(s, "--"):
			p.parseLong(i)
		case len(s) > 1 && s[0] == '-':
			p.parseShorthands(i)
		default:
			p.starts[i+1] = true
		}
	}
	return p.args
}

type parser struct {
	argv []string
	// starts[i] reports whether any reading of the argv reaches argv[i] as an entry of its own,
	// rather than as a value some earlier flag consumed. It has two entries of slack so that
	// consuming the last entry needs no bounds check.
	starts []bool
	args   []Arg
}

// parseLong reads argv[i], which starts with "--", as a long flag.
func (p *parser) parseLong(i int) {
	name, value, hasValue := strings.Cut(p.argv[i][2:], "=")
	if name == "" || name[0] == '-' {
		// "--", "--=x" and "---x" name no flag: pflag rejects the first as the end of the flags and
		// the others as malformed. Reading them as operands leaves the rest of the argv readable.
		p.starts[i+1] = true
		return
	}

	f, known := flagsByName[name]
	switch {
	case hasValue:
		p.emit(name, value, i)
		p.starts[i+1] = true
	case known && !f.TakesValue():
		p.emit(name, f.NoOptDefVal, i)
		p.starts[i+1] = true
	case known:
		p.consumeNext(name, i)
	default:
		p.emit(name, unknownNoOptDefVal, i)
		p.starts[i+1] = true
		p.consumeNext(name, i)
	}
}

// parseShorthands reads argv[i], which starts with a single "-", as a run of shorthands.
func (p *parser) parseShorthands(i int) {
	s := p.argv[i][1:]
	// reached[j] is starts for the run: whether any reading gets as far as s[j] still looking for a
	// shorthand. Its extra entry marks the run ending without consuming anything else.
	reached := make([]bool, len(s)+1)
	reached[0] = true

	for j := range len(s) {
		if !reached[j] {
			continue
		}
		f, known := flagsByShorthand[s[j]]
		rest := s[j+1:]
		switch {
		case len(rest) > 1 && rest[0] == '=':
			// "-f=value". A lone "=" is not this form but an ordinary one-character value.
			p.emit(f.Name, rest[1:], i)
			p.starts[i+1] = true
		case known && !f.TakesValue():
			p.emit(f.Name, f.NoOptDefVal, i)
			reached[j+1] = true
		case known:
			p.consumeRest(f.Name, rest, i)
		default:
			reached[j+1] = true
			p.consumeRest("", rest, i)
		}
	}

	if reached[len(s)] {
		p.starts[i+1] = true
	}
}

// consumeNext gives flag the entry after argv[i] as its value.
func (p *parser) consumeNext(flag string, i int) {
	if i+1 >= len(p.argv) {
		return // Docker rejects the argv outright; there is no value to report.
	}
	p.emit(flag, p.argv[i+1], i+1)
	p.starts[i+2] = true
}

// consumeRest gives flag the value a shorthand ending the run in argv[i] takes: rest, what is left
// of the run, or the entry after it when the run ends there.
func (p *parser) consumeRest(flag, rest string, i int) {
	if rest == "" {
		p.consumeNext(flag, i)
		return
	}
	p.emit(flag, rest, i)
	p.starts[i+1] = true
}

// emit records an occurrence of flag, or nothing at all for the "" of an unrecognized shorthand.
func (p *parser) emit(flag, value string, i int) {
	if flag == "" {
		return
	}
	p.args = append(p.args, Arg{Flag: flag, Value: value, Index: i})
}

// ParseArray returns every flag occurrence in arr, a "runArgs" array, as [Parse] reads the argv the
// array becomes; [Arg.Index] indexes arr.Elements. An element that is not a string, which the
// devcontainer tooling could not hand to docker at all, stands in as an empty entry so that the
// elements around it keep the positions docker would read them at.
func ParseArray(arr *hujson.Array) []Arg {
	argv := make([]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		if lit, ok := elem.Value.(hujson.Literal); ok && lit.Kind() == '"' {
			argv[i] = lit.String()
		}
	}
	return Parse(argv)
}

// IsTrue reports whether value turns on the boolean flag it was written for. Docker reads it with
// [strconv.ParseBool] and refuses to start the container on anything else; decolint reads anything
// else as turning the flag on, since the argv is already broken and the flag was plainly asked for.
func IsTrue(value string) bool {
	on, err := strconv.ParseBool(value)
	return err != nil || on
}
