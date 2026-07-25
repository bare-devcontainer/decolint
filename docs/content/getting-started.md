---
title: Getting started
---

This page takes you from an empty shell to decolint running in CI. Every
setting it mentions is documented in full in the
[README](https://github.com/bare-devcontainer/decolint#readme).

## Install

A prebuilt binary is the quickest way to start. Download one from the
[releases page](https://github.com/bare-devcontainer/decolint/releases); the
artifacts are signed and carry build provenance.

To run it as a container instead:

```console
docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint
```

Or build it from source, which needs Go 1.26 or newer:

```console
GOEXPERIMENT=jsonv2 go install github.com/bare-devcontainer/decolint/cmd/decolint@latest
```

`GOEXPERIMENT=jsonv2` is required because decolint uses the still experimental
`encoding/json/v2` package.

## Run it

Point decolint at a directory, or at nothing to lint the current one:

```console
$ decolint
.devcontainer/devcontainer.json:3:12: error: "build" is missing "dockerfile" (missing-build-dockerfile)
Found 1 error and 0 warnings.
```

decolint works out what each directory is from its layout and lints the
configuration files it finds:

- a **dev container definition** — `.devcontainer/devcontainer.json`,
  `.devcontainer.json`, or `.devcontainer/<folder>/devcontainer.json`
- a **Feature** — `devcontainer-feature.json`
- a **Template** — `devcontainer-template.json`, plus the dev container
  configuration the template ships

It exits `0` when nothing was reported at `error` severity, `1` when something
was, and `2` if it could not do its job — a file that does not parse, say.

## Choosing what to report

Every rule belongs to one of four categories, and only `correctness` is
enabled out of the box:

| Category | Default | Reports |
| --- | --- | --- |
| `correctness` | `error` | configuration that is invalid or does not behave as written |
| `security` | `off` | container runtime privileges and hardening |
| `reproducibility` | `off` | unpinned versions or digests that let the environment drift |
| `style` | `off` | discouraged or legacy configuration that still works |

Turn the others on in a config file. `decolint -init` writes one listing every
rule, ready to edit:

```jsonc
// .decolint.jsonc
{
  "categories": {
    "security": "error",
    "reproducibility": "warn"
  },
  "rules": {
    "pin-image-digest": "off"
  }
}
```

`rules` overrides individual rules and wins over their category. Both accept
`error`, `warn` and `off`. The [rule reference](rules/) lists what each rule
checks and why.

Some rules apply only to a particular platform — Codespaces ignores `bind`
mounts, for instance, which is worth reporting only if you use Codespaces.
Those rules stay off until you name the platform:

```console
decolint -platform=vscode,codespaces
```

## Lint what actually runs

A container's configuration is not only what its own file says: the base image
and every Feature it references contribute their own, and the tooling merges
them all. A Feature that sets `privileged: true` is invisible to a linter that
reads only `devcontainer.json`.

`-merge` resolves the base image and every referenced Feature, applies the
specification's merge logic, and lints the result:

```console
decolint -merge
```

This one reaches the network, so it belongs in CI rather than in a pre-commit
hook. A Feature or image that cannot be fetched is an error.

## In CI

On GitHub Actions, the `github` format turns findings into inline annotations
on the pull request diff:

```yaml
- run: decolint -format=github -deny-warnings .
```

`-deny-warnings` makes `warn` findings fail the job too; without it only
`error` findings do.

To collect findings as alerts in the repository's Security tab instead, write
a SARIF log and upload it:

```yaml
- run: decolint -merge -format=sarif . > decolint.sarif
  continue-on-error: true # decolint exits 1 on findings; the upload must still run
- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: decolint.sarif
```

Each alert links back to the rule's page here, so whoever picks it up gets the
reasoning without leaving the alert.

## Silencing a finding

When a rule is right in general but wrong for one line, suppress it in the
configuration file itself rather than turning the rule off everywhere:

```jsonc
{
  // decolint-ignore-line no-image-latest
  "image": "mcr.microsoft.com/devcontainers/base:latest"
}
```

`decolint-ignore-line` covers the line it is on, `decolint-ignore-next-line`
the line after it, and `decolint-ignore-file` the whole file. Naming rule IDs
after the directive limits it to those rules; naming none suppresses
everything on that line.

## Next steps

- [Rules](rules/) — what each rule checks, why, and what it accepts.
- [README](https://github.com/bare-devcontainer/decolint#readme) — the full
  reference for flags, the config file, output formats and merging.
