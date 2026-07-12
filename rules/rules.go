// Package rules provides the built-in lint rules bundled with decolint. See [linter.Rule] for the
// fields a rule declares.
//
// To add a new rule, declare a [linter.Rule] value in a new file in this package and register it in
// RegisterRules.
package rules

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// Registration pairs a built-in rule with the severity it's registered at by default.
type Registration struct {
	// Rule is the built-in rule.
	Rule *linter.Rule
	// DefaultSeverity is the severity Rule is registered at unless overridden.
	DefaultSeverity linter.Severity
}

// builtinRules lists the built-in rules, in a deterministic order (alphabetically by rule ID), along
// with their default severities.
var builtinRules = []Registration{
	{IDDirMismatch, linter.SeverityError},
	{InvalidSemver, linter.SeverityError},
	{MissingBuildDockerfile, linter.SeverityError},
	{MissingComposeService, linter.SeverityError},
	{MissingContainerDef, linter.SeverityError},
	{MissingRequiredProps, linter.SeverityError},
	{MissingWorkspaceMountFolder, linter.SeverityError},
	{NoAppPort, linter.SeverityWarn},
	{NoBindMount, linter.SeverityWarn},
	{NoCapAddAll, linter.SeverityWarn},
	{NoDockerSocketMount, linter.SeverityWarn},
	{NoHostPortFormat, linter.SeverityError},
	{NoImageLatest, linter.SeverityWarn},
	{NoPrivilegedContainer, linter.SeverityWarn},
	{NoSeccompOverride, linter.SeverityOff},
	{NoSeccompUnconfined, linter.SeverityWarn},
	{PinExtensionVersion, linter.SeverityWarn},
	{PinFeatureVersion, linter.SeverityWarn},
	{PinImageDigest, linter.SeverityOff},
	{RequireCapDropAll, linter.SeverityOff},
	{RequireNoNewPrivileges, linter.SeverityOff},
	{RequireNonRoot, linter.SeverityOff},
}

// Overrides carries the user-supplied severity overrides that RegisterRules applies on top of the
// built-in defaults.
type Overrides struct {
	// Categories maps a category name (see linter.ParseCategory), matched case-insensitively, to
	// the severity every rule in that category is registered at, unless Rules overrides that rule
	// individually. A category severity also applies to rules that are off by default.
	Categories map[string]linter.Severity
	// Rules maps a rule ID to the severity that rule is registered at. It takes precedence over
	// Categories.
	Rules map[string]linter.Severity
}

// SeverityFor returns the severity reg is registered at under o: the per-rule override if present,
// otherwise the override for the rule's category, otherwise reg's default severity.
func (o Overrides) SeverityFor(reg Registration) linter.Severity {
	severity := reg.DefaultSeverity
	// Category names are matched by parsing rather than by map lookup so that the
	// case-insensitivity ParseCategory promises also holds for config keys. Keys are visited in
	// sorted order to keep the result deterministic if several spellings of one category appear.
	for _, name := range slices.Sorted(maps.Keys(o.Categories)) {
		if c, err := linter.ParseCategory(name); err == nil && c == reg.Rule.Category {
			severity = o.Categories[name]
		}
	}
	if s, ok := o.Rules[reg.Rule.ID]; ok {
		severity = s
	}
	return severity
}

// RegisterRules registers the built-in rules whose target platform matches platforms on l, in a
// deterministic order, at their default severities, unless overrides names a rule's ID or its
// category, in which case that severity is used instead (see Overrides.SeverityFor).
//
// A rule is registered if it declares no target platforms (applies to all platforms), or if any of
// the platforms it targets is in platforms. If platforms is empty, only rules with no target
// platforms are registered.
//
// RegisterRules returns an error if overrides.Rules contains a key that does not match any
// built-in rule ID, or if overrides.Categories contains a key that does not name a category. An
// override for a rule that exists but is filtered out by platforms is not an error: overriding a
// platform-scoped rule that hasn't been enabled is a legitimate no-op, not a typo.
func RegisterRules(l *linter.Linter, platforms []linter.Platform, overrides Overrides) error {
	if unknown := unknownOverrides(overrides.Rules); len(unknown) > 0 {
		return fmt.Errorf("unknown rule ID(s) in config: %s", strings.Join(unknown, ", "))
	}
	if unknown := unknownCategories(overrides.Categories); len(unknown) > 0 {
		return fmt.Errorf("unknown category name(s) in config: %s", strings.Join(unknown, ", "))
	}

	for _, reg := range builtinRules {
		if !platformEnabled(reg.Rule.Platforms, platforms) {
			continue
		}
		l.RegisterRule(reg.Rule, overrides.SeverityFor(reg))
	}
	return nil
}

// Builtin returns the built-in rules and their default severities, in the same deterministic order
// as RegisterRules uses. It is a copy of the internal registry; callers may not mutate the built-in
// rules through it.
func Builtin() []Registration {
	return slices.Clone(builtinRules)
}

// unknownOverrides returns the sorted set of keys in overrides that do not match any built-in rule
// ID, regardless of platform: a rule that exists but is filtered out by platforms is still known,
// since overriding a platform-scoped rule that hasn't been enabled is a legitimate no-op, not a typo.
func unknownOverrides(overrides map[string]linter.Severity) []string {
	var unknown []string
	for id := range overrides {
		if !slices.ContainsFunc(builtinRules, func(reg Registration) bool { return reg.Rule.ID == id }) {
			unknown = append(unknown, id)
		}
	}
	slices.Sort(unknown)
	return unknown
}

// unknownCategories returns the sorted set of keys in overrides that do not name a category (see
// linter.ParseCategory).
func unknownCategories(overrides map[string]linter.Severity) []string {
	var unknown []string
	for name := range overrides {
		if _, err := linter.ParseCategory(name); err != nil {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	return unknown
}

// platformEnabled reports whether a rule targeting rulePlatforms should run given the user-selected
// platforms. A rule with no target platforms applies to all platforms and always runs; otherwise it
// runs if any of rulePlatforms is present in selected.
func platformEnabled(rulePlatforms, selected []linter.Platform) bool {
	if len(rulePlatforms) == 0 {
		return true
	}
	for _, p := range rulePlatforms {
		if slices.Contains(selected, p) {
			return true
		}
	}
	return false
}
