package dockerargs

import (
	"encoding/csv"
	"regexp"
	"strings"
)

// NetworkHost is the network mode that puts the container in the host's network namespace.
const NetworkHost = "host"

// networkFieldList matches a "--network" value docker/cli reads as a field list rather than as a
// network name. It is docker/cli's own regexp, applied unanchored as it applies it, so one unspaced
// "key=value" anywhere makes the whole value a list: "name = host" names a network, where
// "alias=web, name = host " is a list whose "name" field holds one.
var networkFieldList = regexp.MustCompile(`\w+=\w+(,\w+=\w+)*`)

// NetworkTarget returns the network a "--network" or "--net" value names. Docker takes either the
// network itself ("host") or a comma-separated field list in which "name" holds it
// ("name=host,alias=web"), the fields being a CSV record as a mount entry's are. It lower-cases a
// field's key and value and trims the space around them.
//
// A field list Docker rejects for a reason of its own — an unknown field key, an address that does
// not parse, a field written without a key or without a value — is read here for whatever its
// "name" field holds rather than treated as naming nothing. The value already fails to start the
// container, and reading it lets a rule name the network the author asked for instead of falling
// silent on it, as [IsTrue] reads a boolean flag Docker would reject.
//
// The result is "" for a list naming no network at all and for one the CSV reader cannot read,
// neither of which says what was meant. A list naming several yields the last, which is the one
// Docker is left holding.
func NetworkTarget(value string) string {
	if !networkFieldList.MatchString(value) {
		return value
	}
	fields, err := csv.NewReader(strings.NewReader(value)).Read()
	if err != nil {
		return ""
	}
	target := ""
	for _, field := range fields {
		key, name, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(key)) == "name" {
			target = strings.ToLower(strings.TrimSpace(name))
		}
	}
	return target
}
