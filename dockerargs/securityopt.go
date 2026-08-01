package dockerargs

import "strings"

// Seccomp profile names that stand for a built-in profile rather than for the path of one to load.
// Docker matches them exactly, so a differently cased spelling names a file.
const (
	// SeccompProfileDefault is the runtime's own default profile — the one a container that is
	// given no seccomp option at all runs under.
	SeccompProfileDefault = "builtin"
	// SeccompProfileUnconfined turns syscall filtering off.
	SeccompProfileUnconfined = "unconfined"
)

// AppArmorProfileUnconfined removes the container's AppArmor profile, as
// [SeccompProfileUnconfined] removes its seccomp one.
const AppArmorProfileUnconfined = "unconfined"

// noNewPrivileges is the one security option that may be written without a value.
const noNewPrivileges = "no-new-privileges"

// SecurityOpt is one "securityOpt" entry, equivalently one value given to "--security-opt".
type SecurityOpt struct {
	// Key names the option, e.g. "seccomp" or "no-new-privileges". Docker matches it
	// case-sensitively, so it is kept as written.
	Key string
	// Value is what the entry gives the option, which for a bare "no-new-privileges" is the "true"
	// it stands for on its own.
	Value string
}

// ParseSecurityOpt reads s the way Docker splits it:
//
//   - the key and the value are separated by the first "=", or, in an entry holding none, by the
//     first ":";
//   - "no-new-privileges" is the one option that may be written bare, standing for "true".
//
// ok is false for an entry Docker rejects outright, which is any other one left without a value.
func ParseSecurityOpt(s string) (opt SecurityOpt, ok bool) {
	key, value, split := strings.Cut(s, "=")
	if !split && key != noNewPrivileges {
		key, value, split = strings.Cut(s, ":")
	}
	if key == noNewPrivileges {
		if !split {
			value = "true"
		}
		return SecurityOpt{Key: key, Value: value}, true
	}
	if !split || value == "" {
		return SecurityOpt{}, false
	}
	return SecurityOpt{Key: key, Value: value}, true
}
