package dockerargs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		argv []string
		want []Arg
	}{
		{"empty", nil, nil},
		{"operand only", []string{"ubuntu"}, nil},

		{"long flag with a joined value", []string{"--cap-drop=ALL"}, []Arg{
			{Flag: "cap-drop", Value: "ALL", Index: 0},
		}},
		{"long flag consuming the next entry", []string{"--cap-drop", "ALL"}, []Arg{
			{Flag: "cap-drop", Value: "ALL", Index: 1},
		}},
		{"long flag with an empty joined value", []string{"--cap-drop="}, []Arg{
			{Flag: "cap-drop", Value: "", Index: 0},
		}},
		{"long flag whose value holds an equals sign", []string{"--security-opt=seccomp=unconfined"}, []Arg{
			{Flag: "security-opt", Value: "seccomp=unconfined", Index: 0},
		}},
		{"long flag taking a value with nothing left to consume", []string{"--cap-drop"}, nil},

		{"boolean flag written bare", []string{"--privileged"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 0},
		}},
		{"boolean flag written with a value", []string{"--privileged=true"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 0},
		}},
		{"boolean flag turned off", []string{"--privileged=false"}, []Arg{
			{Flag: "privileged", Value: "false", Index: 0},
		}},
		{"boolean flag does not consume the next entry", []string{"--privileged", "--cap-drop=ALL"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 0},
			{Flag: "cap-drop", Value: "ALL", Index: 1},
		}},

		{"consumed entry names no flag", []string{"--label", "--cap-drop=ALL"}, []Arg{
			{Flag: "label", Value: "--cap-drop=ALL", Index: 1},
		}},
		{"entry after a consumed one names a flag again", []string{"--label", "x", "--privileged"}, []Arg{
			{Flag: "label", Value: "x", Index: 1},
			{Flag: "privileged", Value: "true", Index: 2},
		}},

		{"shorthand with a joined value", []string{"-v/var/run/docker.sock:/x"}, []Arg{
			{Flag: "volume", Value: "/var/run/docker.sock:/x", Index: 0},
		}},
		{"shorthand consuming the next entry", []string{"-v", "/a:/b"}, []Arg{
			{Flag: "volume", Value: "/a:/b", Index: 1},
		}},
		{"shorthand with an equals-separated value", []string{"-v=/a:/b"}, []Arg{
			{Flag: "volume", Value: "/a:/b", Index: 0},
		}},
		{"lone equals is an ordinary shorthand value", []string{"-v="}, []Arg{
			{Flag: "volume", Value: "=", Index: 0},
		}},
		{"run of shorthands ending in one that takes a value", []string{"-itv", "/var/run/docker.sock:/x"}, []Arg{
			{Flag: "interactive", Value: "true", Index: 0},
			{Flag: "tty", Value: "true", Index: 0},
			{Flag: "volume", Value: "/var/run/docker.sock:/x", Index: 1},
		}},
		{"run of shorthands ending in a joined value", []string{"-itv/a:/b"}, []Arg{
			{Flag: "interactive", Value: "true", Index: 0},
			{Flag: "tty", Value: "true", Index: 0},
			{Flag: "volume", Value: "/a:/b", Index: 0},
		}},
		{"run of boolean shorthands", []string{"-it"}, []Arg{
			{Flag: "interactive", Value: "true", Index: 0},
			{Flag: "tty", Value: "true", Index: 0},
		}},
		{"shorthand turned off", []string{"-t=false"}, []Arg{
			{Flag: "tty", Value: "false", Index: 0},
		}},

		{"a flag is its name, not its spelling", []string{"-v", "/a:/b", "--volume=/c:/d"}, []Arg{
			{Flag: "volume", Value: "/a:/b", Index: 1},
			{Flag: "volume", Value: "/c:/d", Index: 2},
		}},
		{"net is its own flag, not a short form of network", []string{"--net=host", "--network=none"}, []Arg{
			{Flag: "net", Value: "host", Index: 0},
			{Flag: "network", Value: "none", Index: 1},
		}},

		{"unrecognized long flag is read both ways", []string{"--not-a-flag", "--privileged"}, []Arg{
			{Flag: "not-a-flag", Value: "true", Index: 0},
			{Flag: "not-a-flag", Value: "--privileged", Index: 1},
			{Flag: "privileged", Value: "true", Index: 1},
		}},
		{"unrecognized long flag with a joined value is unambiguous", []string{"--not-a-flag=x", "--privileged"}, []Arg{
			{Flag: "not-a-flag", Value: "x", Index: 0},
			{Flag: "privileged", Value: "true", Index: 1},
		}},
		{"unrecognized shorthand names no flag but is read both ways", []string{"-Z", "--privileged"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 1},
		}},
		{"unrecognized shorthand does not hide the rest of its run", []string{"-Zv", "/a:/b"}, []Arg{
			{Flag: "volume", Value: "/a:/b", Index: 1},
		}},
		{"unrecognized shorthand joined to a value", []string{"-Zv/a:/b"}, []Arg{
			{Flag: "volume", Value: "/a:/b", Index: 0},
		}},

		{"parsing continues past the image name", []string{"ubuntu", "--privileged"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 1},
		}},
		{"parsing continues past a terminator", []string{"--", "--privileged"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 1},
		}},

		{"a lone dash is an operand", []string{"-", "--privileged"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 1},
		}},
		{"an empty entry is an operand", []string{"", "--privileged"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 1},
		}},
		{"a malformed long flag names none", []string{"---privileged", "--=x", "--privileged"}, []Arg{
			{Flag: "privileged", Value: "true", Index: 2},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, Parse(tt.argv)); diff != "" {
				t.Errorf("Parse(%q) mismatch (-want +got):\n%s", tt.argv, diff)
			}
		})
	}
}

func TestIsTrue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"t", true},
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"f", false},
		{"", true},
		{"yes", true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			if got := IsTrue(tt.value); got != tt.want {
				t.Errorf("IsTrue(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
