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
[Hugo](https://gohugo.io/). It is a tool dependency in
[`go.mod`](go.mod), so there is nothing to install and the version is
pinned with the rest of them:

```console
make docs         # build into docs/public
make docs-serve   # serve with live reload at http://localhost:1313/
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

A rule's default severity is not set individually; it comes entirely
from its category (see `categoryDefaultSeverities` in
[`rules.go`](rules/rules.go)) — only `CategoryCorrectness` runs by
default, at `error`. Pick the category that matches the problem the
rule reports, not the severity you'd like it to have.

Besides the short `Description`, a rule carries the reasoning and an
example directly on the [`linter.Rule`](linter/rule.go) value —
nothing about a rule lives in a separate file:

```go
package rules

import "github.com/bare-devcontainer/decolint/linter"

var MyRule = &linter.Rule{
	ID:          "my-rule",
	Description: "...",
	LongDescription: `What goes wrong in the configuration this reports, and what to do
instead.`,
	References: []string{"https://containers.dev/implementors/json_reference/"},
	Category:   linter.CategoryCorrectness,
	FileTypes:  []linter.FileType{linter.Devcontainer},
	Platforms:  nil, // applies to every platform
	Paths:      []string{"/mounts/*"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: "devcontainer.json", Content: `{ ... }
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: "devcontainer.json", Content: `{ ... }
`},
			},
		},
	},
	Check: checkMyRule,
}

func checkMyRule(ctx *linter.Context, node *linter.Node) []linter.Finding {
	// node.Value is the HuJSON value at node.Pointer. Set each
	// finding's Offset to the offending value's StartOffset so the
	// engine can resolve it to a line and column.
	return nil
}
```

`LongDescription` is Markdown; write it for the user who just hit the
finding, since it is what the rule's own page on the documentation
site is built from (see below). The SARIF output does not repeat it —
each alert links to the rule's page instead, so a reader who wants the
reasoning is one click away rather than seeing it duplicated inline.

`Example` is machine-checked, not just illustrative:
[`rules/doc_test.go`](rules/doc_test.go) lints `Bad` with the rule as
the only one active and requires a finding, then lints `Good` and
requires none. `Snippet.Files` is one directory: the file named after
the rule's first `FileTypes` entry (`devcontainer.json`,
`devcontainer-feature.json`, or `devcontainer-template.json`) is the
one linted, and any other files are context a rule reads from the
directory (e.g. a Template's other files, for a
`${templateOption:...}` reference). Set `Snippet.DirName` when the
rule reads the directory's own name (`id-dir-mismatch`), and a file's
`Mode` when the rule reads permission bits (`install.sh`'s executable
bit) — `Bad` and `Good` can then differ in mode alone, with identical
content. `Example.Note` is optional Markdown shown after `Good`, for
context the two snippets alone don't convey.

The existing rules in [`rules/`](rules/) are good references,
including for the table-driven tests each rule ships with.

## Where documentation lives

The README and the site divide by what the reader is trying to decide:
the README covers whether to use decolint at all, and the site covers
how. So the walkthroughs and the reference are the site's, and adding
to them is an edit to [`docs/content/`](docs/content/) — Getting
started, Reference, and the rule index's `_index.md` are hand-written
there.

The rest is generated from `rules/*.go` and `README.md` by
[`cmd/docgen`](cmd/docgen/), run as part of `make docs` (see
[Development](#development) above) and standalone as `make
docs-content`:

- **A page per rule**, under `docs/content/rules/` on the published
  site. A new rule or a changed
  `LongDescription`/`Example`/`References` needs no follow-up edit
  anywhere else.
- **The site's landing page**, from the part of `README.md` between
  `<!-- decolint:page=_index -->` and `<!-- decolint:end-page -->`, so
  the pitch the two share has one source. Everything outside those
  markers is README-only. A link inside them must resolve to a heading
  inside them too; point anywhere else at its published address
  (`https://bare-devcontainer.github.io/decolint/...`), which is what
  the generator's dead-anchor check leaves you.
- **The README's category summary**, between the
  `<!-- decolint:categories -->` markers.

Nothing generated is hand-edited. CI's `docs` job runs `make
docs-content` and fails if that changes `README.md`, which is what
catches a generator or a rule declaration that drifted from the other.

The layout of everything the generator writes lives in
[`cmd/docgen/templates/`](cmd/docgen/templates/), so changing how a
page looks is an edit to a template rather than to Go code.

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
