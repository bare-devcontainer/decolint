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

The documentation site in [`docs/`](docs/) is built with
[Hugo](https://gohugo.io/), pinned to the version the Makefile names:

```console
make site         # build into docs/public
make site-serve   # serve with live reload
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

The rule itself carries only the one-line `Description` of what it
checks; the reasoning and the examples live on the documentation site
(see [Documenting a rule](#documenting-a-rule) below), which every
finding links to.

A rule's default severity is not set individually; it comes entirely
from its category (see `categoryDefaultSeverities` in
[`rules.go`](rules/rules.go)) — only `CategoryCorrectness` runs by
default, at `error`. Pick the category that matches the problem the
rule reports, not the severity you'd like it to have.

```go
package rules

import "github.com/bare-devcontainer/decolint/linter"

var MyRule = &linter.Rule{
	ID:          "my-rule",
	Description: "...",
	Category:    linter.CategoryCorrectness,
	FileTypes:   []linter.FileType{linter.Devcontainer},
	Platforms:   nil, // applies to every platform
	Paths:       []string{"/mounts/*"},
	Check:       checkMyRule,
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

## Documenting a rule

Every rule has a page under [`docs/content/rules/`](docs/content/rules/),
named after its ID, which is where the reasoning and the examples live.
It is what the SARIF output, and so every code scanning alert, links
to, so write it for the user who just hit the finding.

````markdown
---
title: my-rule
category: correctness
platforms: []
file_types: [devcontainer]
description: >-
  disallow ...
---

## Why

What goes wrong in the configuration this reports, and what to do
instead.

## Bad

```jsonc
{ ... }
```

## Good

```jsonc
{ ... }
```

## References

- <https://containers.dev/implementors/json_reference/>
````

The front matter must match the rule's Go declaration, and the tests
in [`rules/doc_test.go`](rules/doc_test.go) enforce it: they lint the
Bad example and require it to report the rule, lint the Good example
and require it to report nothing, and check that every rule has a page
and every page at least one `https` reference.

Two things to know when writing the examples:

- An example needing more than one file names each with a
  ``### `path` `` heading before its block; without one, a block is
  linted as the rule's own file type. Set `example_dir` in the front
  matter when the rule reads the name of the directory it is in.
- An example whose Bad and Good differ in something other than file
  contents — a permission bit, or a missing file — cannot be linted.
  Write it in whatever form shows the difference, set
  `example_verify: false`, and add the rule to `unverifiableExamples`
  in the test.

The rule index and the sidebar are built from the pages themselves, so
there is no list to update by hand. Preview the site with `make
site-serve`, which needs [Hugo](https://gohugo.io/) at the version the
[Makefile](Makefile) names.

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
