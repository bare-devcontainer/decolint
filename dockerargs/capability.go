package dockerargs

import "strings"

// AllCapabilities is the pseudo-capability standing for every Linux capability.
const AllCapabilities = "ALL"

// Capability returns name — a capability written in "capAdd" or given to "--cap-add"/"--cap-drop" —
// as Docker matches it: upper-cased and prefixed with "CAP_", except for [AllCapabilities], which
// takes no prefix.
func Capability(name string) string {
	c := strings.ToUpper(name)
	if c == AllCapabilities || strings.HasPrefix(c, "CAP_") {
		return c
	}
	return "CAP_" + c
}
