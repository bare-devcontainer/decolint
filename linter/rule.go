package linter

import (
	"encoding/json/jsontext"
	"fmt"
	"strings"

	"github.com/tailscale/hujson"
)

// FileType identifies the kind of configuration file being linted.
type FileType string

const (
	// Devcontainer is a devcontainer.json file.
	Devcontainer FileType = "devcontainer"
	// Feature is a devcontainer-feature.json file.
	Feature FileType = "feature"
	// Template is a devcontainer-template.json file.
	Template FileType = "template"
)

// Platform identifies a target platform a rule is scoped to.
type Platform int

const (
	// PlatformVSCode marks a rule specific to Visual Studio Code's Dev Containers extension.
	PlatformVSCode Platform = iota
	// PlatformCodespaces marks a rule specific to GitHub Codespaces.
	PlatformCodespaces
)

// String returns the platform's name, as used in the -platform flag and in output.
func (p Platform) String() string {
	switch p {
	case PlatformVSCode:
		return "vscode"
	case PlatformCodespaces:
		return "codespaces"
	default:
		return "unknown"
	}
}

// ParsePlatform parses a platform name, matched case-insensitively, into a Platform. It returns an
// error if name does not name a known platform.
func ParsePlatform(name string) (Platform, error) {
	switch strings.ToLower(name) {
	case "vscode":
		return PlatformVSCode, nil
	case "codespaces":
		return PlatformCodespaces, nil
	default:
		return 0, fmt.Errorf("unknown platform %q (want one of: vscode, codespaces)", name)
	}
}

// Context carries everything a rule needs to inspect a single configuration file.
type Context struct {
	// Path is the path of the file being linted.
	Path string
	// Type is the kind of configuration file being linted.
	Type FileType
	// Src is the raw content of the file.
	Src []byte
	// Root is the HuJSON syntax tree parsed from Src. It preserves comments and byte offsets into Src.
	Root *hujson.Value
}

// Severity indicates how a finding should be treated: whether it's reported as an error or a
// warning, or not reported at all. It is specified when a rule is registered on a Linter.
// Severities are ordered from least to most severe, so they can be compared directly (e.g. to rank
// findings or apply a fail threshold).
type Severity int

const (
	// SeverityOff disables a rule; it produces no findings.
	SeverityOff Severity = iota
	// SeverityWarn marks a finding as a warning.
	SeverityWarn
	// SeverityError marks a finding as an error.
	SeverityError
)

// String returns the severity's name, as used in output and configuration files.
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarn:
		return "warn"
	default:
		return "off"
	}
}

// MarshalJSONTo encodes the severity as its name (see String), for use with encoding/json/v2.
func (s Severity) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(jsontext.String(s.String()))
}

// UnmarshalJSONFrom decodes a severity from its name (see ParseSeverity), for use with
// encoding/json/v2.
func (s *Severity) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if tok.Kind() != '"' {
		return fmt.Errorf("unmarshal severity: unexpected token kind %q", tok.Kind())
	}

	severity, err := ParseSeverity(tok.String())
	if err != nil {
		return fmt.Errorf("unmarshal severity: %w", err)
	}
	*s = severity
	return nil
}

// ParseSeverity parses a severity name, matched case-insensitively, into a Severity. It returns an
// error if name does not name a known severity.
func ParseSeverity(name string) (Severity, error) {
	switch strings.ToLower(name) {
	case "off":
		return SeverityOff, nil
	case "warn":
		return SeverityWarn, nil
	case "error":
		return SeverityError, nil
	default:
		return 0, fmt.Errorf("unknown severity %q (want one of: off, warn, error)", name)
	}
}

// Node is a single value reached by the engine's traversal.
type Node struct {
	// Pointer is the JSON Pointer (RFC 6901) of the value, e.g. "/image" or "/mounts/0". It is "" for
	// the document root.
	Pointer string
	// Value is the HuJSON value at Pointer.
	Value *hujson.Value
}

// Finding is a single problem reported by a rule.
type Finding struct {
	// Message describes the problem in a human-readable way.
	Message string
	// Offset is the byte offset into Context.Src where the problem is located, typically the
	// StartOffset of the offending value.
	Offset int
}

// Rule is a single lint rule.
//
// A rule declares the kinds of configuration files it applies to via FileTypes and the JSON Pointer
// paths it is interested in via Paths. The lint engine traverses the HuJSON syntax tree of each
// matching file exactly once and calls Check for every value matching one of its paths. The syntax
// tree preserves comments and byte offsets, so findings can point at the exact location of the
// offending value.
type Rule struct {
	// ID is the unique identifier of the rule, used in output and in ignore directives (e.g.
	// "no-image-latest").
	ID string
	// Description is a short human-readable description of what the rule checks.
	Description string
	// FileTypes are the kinds of configuration files this rule applies to. The rule is only run
	// against files of these types.
	FileTypes []FileType
	// Platforms are the target platforms this rule applies to. A nil or empty value means the rule
	// applies to every platform and always runs, regardless of which platforms are selected when the
	// linter is configured.
	Platforms []Platform
	// Paths are the JSON Pointer patterns of the values this rule wants to inspect. A "*" segment
	// matches any object member name or array index (e.g. "/mounts/*"); the empty string matches the
	// document root.
	Paths []string
	// Check inspects one value matching Paths and returns any findings. It is called at most once per
	// rule for a given value, even if several patterns match it. Check must be safe for concurrent
	// use, since it may be called for multiple files.
	Check func(ctx *Context, node *Node) []Finding
}
