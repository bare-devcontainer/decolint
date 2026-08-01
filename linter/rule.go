package linter

import (
	"encoding/json/jsontext"
	"fmt"
	"io/fs"
	"strings"

	"github.com/bare-devcontainer/decolint/dockerargs"
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

// MarshalJSONTo encodes the platform as its name (see String), for use with encoding/json/v2.
func (p Platform) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := enc.WriteToken(jsontext.String(p.String())); err != nil {
		return fmt.Errorf("marshal platform: %w", err)
	}
	return nil
}

// UnmarshalJSONFrom decodes a platform from its name (see ParsePlatform), for use with
// encoding/json/v2.
func (p *Platform) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return fmt.Errorf("unmarshal platform: %w", err)
	}
	if tok.Kind() != '"' {
		return fmt.Errorf("unmarshal platform: unexpected token kind %q", tok.Kind())
	}

	platform, err := ParsePlatform(tok.String())
	if err != nil {
		return fmt.Errorf("unmarshal platform: %w", err)
	}
	*p = platform
	return nil
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

// Category classifies a rule by the kind of problem it reports. Every rule belongs to exactly one
// category, so users can adjust the severity of a whole class of rules at once.
type Category int

const (
	// The zero value is deliberately not a valid category, so a rule that forgets to declare one
	// maps to "unknown" and is caught by tests instead of silently landing in the first category.

	// CategoryCorrectness marks a rule that reports configuration that is invalid or does not
	// behave as written.
	CategoryCorrectness Category = iota + 1
	// CategorySecurity marks a rule that reports container runtime privilege and hardening issues.
	CategorySecurity
	// CategoryReproducibility marks a rule that reports unpinned versions or digests that make the
	// resulting environment change over time.
	CategoryReproducibility
	// CategoryStyle marks a rule that reports discouraged or legacy configuration that still works.
	CategoryStyle
)

// String returns the category's name, as used in configuration files and output.
func (c Category) String() string {
	switch c {
	case CategoryCorrectness:
		return "correctness"
	case CategorySecurity:
		return "security"
	case CategoryReproducibility:
		return "reproducibility"
	case CategoryStyle:
		return "style"
	default:
		return "unknown"
	}
}

// ParseCategory parses a category name, matched case-insensitively, into a Category. It returns an
// error if name does not name a known category.
func ParseCategory(name string) (Category, error) {
	switch strings.ToLower(name) {
	case "correctness":
		return CategoryCorrectness, nil
	case "security":
		return CategorySecurity, nil
	case "reproducibility":
		return CategoryReproducibility, nil
	case "style":
		return CategoryStyle, nil
	default:
		return 0, fmt.Errorf("unknown category %q (want one of: correctness, security, reproducibility, style)", name)
	}
}

// Context carries everything a rule needs to inspect a single configuration file.
type Context struct {
	// Path is the path of the file being linted, for reporting only: it is spelled however the
	// caller named the file, so a rule must not read anything into its shape. Use [Dir] for the
	// containing directory.
	Path string
	// Type is the kind of configuration file being linted.
	Type FileType
	// Root is the HuJSON syntax tree of the file. It preserves comments and byte offsets into the
	// original source.
	Root *hujson.Value
	// Dir describes the directory containing the file being linted.
	Dir Dir
}

// Dir is the directory a configuration file is linted in. Each field is independently optional, and
// a rule that needs one must return no findings when it is unset.
type Dir struct {
	// FS gives read access to the directory's contents, confined to the lint root. It is nil when
	// the caller has no backing filesystem, e.g. an in-memory document.
	FS fs.FS
	// Name is the directory's own name, taken from where the directory actually is rather than from
	// [Context.Path]. It is empty when the caller has no directory to name.
	Name string
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
	if err := enc.WriteToken(jsontext.String(s.String())); err != nil {
		return fmt.Errorf("marshal severity: %w", err)
	}
	return nil
}

// UnmarshalJSONFrom decodes a severity from its name (see ParseSeverity), for use with
// encoding/json/v2.
func (s *Severity) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return fmt.Errorf("unmarshal severity: %w", err)
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
	// Arg is the "docker run" flag occurrence the value was reached as, set only on a node the
	// "runArgs" traversal produced (see [Rule.Paths]) and nil on every other node. Value is then the
	// element the flag's value is written in, which is the flag's own element or the one after it.
	Arg *dockerargs.Arg
}

// Finding is a single problem reported by a rule.
type Finding struct {
	// Message describes the problem in a human-readable way.
	Message string
	// Offset is the byte offset into the original source where the problem is located, typically the
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
	// LongDescription explains why the rule exists: what goes wrong in the configuration it
	// reports, and what to do instead. It is Markdown, and is shown wherever the rule is
	// documented rather than merely named.
	LongDescription string
	// References are URLs to the specification, documentation, or implementation that justify the
	// rule, most authoritative first.
	References []string
	// Category is the [Category] this rule reports; every rule must declare exactly one.
	Category Category
	// FileTypes are the kinds of configuration files this rule applies to.
	FileTypes []FileType
	// Platforms are the target platforms this rule applies to. A nil or empty value means the rule
	// applies to every platform, regardless of which platforms are selected when the linter is
	// configured.
	Platforms []Platform
	// Paths are the JSON Pointer patterns of the values this rule wants to inspect. A "*" segment
	// matches any object member name or array index (e.g. "/mounts/*"); the empty string matches the
	// document root.
	//
	// A devcontainer.json's "runArgs" is traversed as the "docker run" argv it becomes, so its
	// elements are addressed by flag rather than by index: "/runArgs/--volume" matches once per
	// occurrence of that flag, whichever spelling the argv uses, and [Node.Arg] carries the value the
	// occurrence gives it. Nothing else is addressed under a devcontainer.json's "runArgs", so a
	// pattern matching there always arrives with [Node.Arg] set. A rule reporting a flag's absence
	// cannot be driven by that, since a flag that is not there is never matched; it inspects the
	// document root instead.
	Paths []string
	// Example shows the rule firing and not firing on realistic configuration. Tests lint both: Bad
	// must report the rule, Good must not.
	Example Example
	// Check inspects one value matching Paths and returns any findings. It is called once for a given
	// value, even if several patterns match it — except for a "runArgs" element naming several flags,
	// which it is called for once per flag, with [Node.Arg] telling the occurrences apart. Check must
	// be safe for concurrent use, since it may be called for multiple files.
	Check func(ctx *Context, node *Node) []Finding
}

// Example pairs configuration that trips a rule with configuration that doesn't, for the rule's
// documentation.
type Example struct {
	// Bad is configuration the rule reports.
	Bad Snippet
	// Good is configuration the rule does not report, typically Bad with the problem fixed.
	Good Snippet
	// Note is Markdown prose shown after Good, for context Bad and Good alone don't convey (e.g. why
	// Good is scoped the way it is). It is optional.
	Note string
}

// Snippet is the directory an example is linted in: one or more files, and optionally the
// directory's own name, for a rule that reads it (e.g. [Dir.Name]).
type Snippet struct {
	// Files are the snippet's files. Exactly one must have the path a rule's first FileType is
	// named at (e.g. "devcontainer.json" for [Devcontainer]); that is the file linted, and it is
	// also the one shown first. The rest are context a rule reads from the directory, e.g. a
	// devcontainer-template.json a devcontainer.json's ${templateOption:...} reference is checked
	// against.
	Files []ExampleFile
	// DirName is the directory's own name, for a rule that reads [Dir.Name] (e.g. a Feature's or
	// Template's id must match the directory containing it). It is empty when no rule needs it.
	DirName string
}

// ExampleFile is one file of a [Snippet].
type ExampleFile struct {
	// Path is the file's path, relative to the directory the example is linted in (e.g.
	// "devcontainer.json" or ".devcontainer/devcontainer.json").
	Path string
	// Content is the file's content.
	Content string
	// Mode is the file's permission bits. The zero value means a regular file at the default mode;
	// set it for a rule that inspects permissions (e.g. whether install.sh is executable).
	Mode fs.FileMode
}
