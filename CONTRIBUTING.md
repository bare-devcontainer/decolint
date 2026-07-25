# Contributing

## Development

decolint is written in Go (the required version is in
[`go.mod`](go.mod)) and uses the still experimental
`encoding/json/v2` standard library package, so every `go` command
needs `GOEXPERIMENT=jsonv2`. The [Makefile](Makefile) sets it for you:

```console
make build   # build ./bin/decolint
make test    # go test ./...
make lint    # golangci-lint
make run ARGS="-format=json path/to/dir"
```

## Adding a rule

Rules are plain Go code. Declare a
[`linter.Rule`](linter/rule.go) value in a new file under
[`rules/`](rules/) and add it to the `builtinRuleList` slice in
[`rules.go`](rules/rules.go).

A rule declares the kinds of configuration files it applies to
(`linter.Devcontainer`, `linter.Feature`, `linter.Template`), the
category it belongs to (`linter.CategoryCorrectness`,
`linter.CategorySecurity`, `linter.CategoryReproducibility`, or
`linter.CategoryStyle`; every rule must declare exactly one), the
target platform(s) it applies to (`linter.PlatformVSCode`,
`linter.PlatformCodespaces`, ...; a nil or empty value means the rule
applies to every platform), and the JSON Pointer paths it wants to
inspect. The engine traverses the syntax tree once per matching file
and calls `Check` for every value matching one of the paths; a `*`
segment matches any object member name or array index, and the empty
string matches the document root.

Besides the one-line `Description` of what it checks, every rule
carries a `LongDescription` explaining why the configuration it
reports is a problem, and at least one `References` URL pointing at
the specification, documentation, or implementation that justifies it.
Both are shown by `decolint -explain <rule-id>` and in the SARIF
output, so write them for the user who just hit the finding.

A rule's default severity is not set individually; it comes entirely
from its category (see `categoryDefaultSeverities` in
[`rules.go`](rules/rules.go)) — only `CategoryCorrectness` runs by
default, at `error`. Pick the category that matches the problem the
rule reports, not the severity you'd like it to have.

```go
package rules

import "github.com/bare-devcontainer/decolint/linter"

var MyRule = &linter.Rule{
	ID:              "my-rule",
	Description:     "...",
	LongDescription: "...",
	References:      []string{"https://containers.dev/implementors/json_reference/"},
	Category:        linter.CategoryCorrectness,
	FileTypes:       []linter.FileType{linter.Devcontainer},
	Platforms:       nil, // applies to every platform
	Paths:           []string{"/mounts/*"},
	Check:           checkMyRule,
}

func checkMyRule(ctx *linter.Context, node *linter.Node) []linter.Finding {
	// node.Value is the HuJSON value at node.Pointer. Set each
	// finding's Offset to the offending value's StartOffset so the
	// engine can resolve it to a line and column.
	return nil
}
```

The existing rules in [`rules/`](rules/) are good references,
including for the table-driven tests each rule ships with. When a new
rule lands, also add a row for it to the table in
[README.md](README.md#rules).

When implementing or reviewing rules, consult the Dev Container
specification at [containers.dev](https://containers.dev/) to confirm
the behavior matches the spec:

- [`devcontainer.json` reference](https://containers.dev/implementors/json_reference/)
- [Features specification](https://containers.dev/implementors/features/)
- [Templates specification](https://containers.dev/implementors/templates/)

## Pull requests

PR titles must follow the [Conventional
Commits](https://www.conventionalcommits.org/) format with one of the
types `feat`, `fix`, `ci`, `chore`, `test`, or `docs`, e.g.
`feat(cli): add new feature`. Keep external dependencies to a minimum.
