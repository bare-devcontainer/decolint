// Package rules provides the built-in lint rules bundled with decolint. See [linter.Rule] for the
// fields a rule declares.
//
// To add a new rule, declare a [linter.Rule] value in a new file in this package and register it in
// RegisterRules.
package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// ruleReg pairs a built-in rule with the severity it's registered at by default.
type ruleReg struct {
	rule     *linter.Rule
	severity linter.Severity
}

// builtinRules lists the built-in rules, in a deterministic order (alphabetically by rule ID), along
// with their default severities.
var builtinRules = []ruleReg{
	{CodespacesNoBindMount, linter.Warn},
	{CodespacesNoHostPortFormat, linter.Error},
	{IDDirMismatch, linter.Error},
	{InvalidSemver, linter.Error},
	{MissingBuildDockerfile, linter.Error},
	{MissingComposeService, linter.Error},
	{MissingContainerDef, linter.Error},
	{MissingRequiredProps, linter.Error},
	{MissingWorkspaceMountFolder, linter.Error},
	{NoAppPort, linter.Warn},
	{NoCapAddAll, linter.Warn},
	{NoDockerSocketMount, linter.Warn},
	{NoImageLatest, linter.Warn},
	{NoPrivilegedContainer, linter.Warn},
	{NoSeccompOverride, linter.Off},
	{NoSeccompUnconfined, linter.Warn},
	{PinExtensionVersion, linter.Warn},
	{PinFeatureVersion, linter.Warn},
	{PinImageDigest, linter.Off},
	{RequireCapDropAll, linter.Off},
	{RequireNoNewPrivileges, linter.Off},
	{RequireNonRoot, linter.Off},
}

// RegisterRules registers the built-in rules whose target platform matches platforms on l, in a
// deterministic order, at their default severities, unless overrides contains an entry for a rule's
// ID, in which case that severity is used instead.
//
// A rule is registered if it declares no target platforms (applies to all platforms), or if any of
// the platforms it targets is in platforms. If platforms is empty, only rules with no target
// platforms are registered.
//
// RegisterRules returns an error if overrides contains a key that does not match any built-in rule
// ID. An override for a rule that exists but is filtered out by platforms is not an error:
// overriding a platform-scoped rule that hasn't been enabled is a legitimate no-op, not a typo.
func RegisterRules(l *linter.Linter, platforms []linter.Platform, overrides map[string]linter.Severity) error {
	if unknown := unknownOverrides(overrides); len(unknown) > 0 {
		return fmt.Errorf("unknown rule ID(s) in config: %s", strings.Join(unknown, ", "))
	}

	for _, reg := range builtinRules {
		if !platformEnabled(reg.rule.Platforms, platforms) {
			continue
		}
		severity := reg.severity
		if s, ok := overrides[reg.rule.ID]; ok {
			severity = s
		}
		l.RegisterRule(reg.rule, severity)
	}
	return nil
}

// unknownOverrides returns the sorted set of keys in overrides that do not match any built-in rule
// ID, regardless of platform: a rule that exists but is filtered out by platforms is still known,
// since overriding a platform-scoped rule that hasn't been enabled is a legitimate no-op, not a typo.
func unknownOverrides(overrides map[string]linter.Severity) []string {
	var unknown []string
	for id := range overrides {
		if !slices.ContainsFunc(builtinRules, func(reg ruleReg) bool { return reg.rule.ID == id }) {
			unknown = append(unknown, id)
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
