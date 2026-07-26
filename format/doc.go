// Package format implements the output formats decolint can write a lint report in: human-readable
// text, a JSON object, GitHub Actions workflow command annotations, and a SARIF 2.1.0 log. Every
// format reports the configuration files that were linted alongside the issues found in them; see
// [Report].
package format
