---
title: Reference
description: 'Every flag, config file member, and output format decolint supports.'
toc: true
---

## What decolint lints

```console
decolint [directory ...]
```

Each directory is detected as one of the following based on its layout, and
the configuration files it contains are linted:

- **Dev container definition** — the `devcontainer.json` files at the
  locations defined by the devcontainer specification:
  `.devcontainer/devcontainer.json`, `.devcontainer.json`, and
  `.devcontainer/<folder>/devcontainer.json`
- **Feature** (contains `devcontainer-feature.json`) — that file
- **Template** (contains `devcontainer-template.json`) — that file, plus the
  dev container configuration the template ships

With no arguments, the current directory is linted.

## Configuration

Every linting setting can be declared in the [config file](#config-file). The
four scalar settings additionally have a command-line flag, which overrides
the config file when given:

| Setting | Config member | Flag |
| --- | --- | --- |
| [Target platforms](#target-platforms) | `platforms` | `-p`, `--platform` |
| [Merge features](#merging) | `merge` | `-m`, `--merge` |
| [Deny warnings](#exit-codes) | `denyWarnings` | `--deny-warnings` |
| [Output format](#output-formats) | `format` | `-f`, `--format` |
| [Category severities](#rule-categories) | `categories` | — |
| [Rule severities](#rules) | `rules` | — |

`--merge` and `--deny-warnings` override in either direction when given
explicitly — e.g. `--merge=false` disables merging even if the config file sets
`"merge": true`. Category and rule severities are config-file only, and
[color](#color) is set by the `--color` flag only.

The remaining flags perform a one-off action and exit; run `decolint --help`
for the full list:

| Flag | Action |
| --- | --- |
| `-c`, `--config <path>` | use the config file at `<path>` instead of [auto-discovery](#config-file) |
| `--init` | write a new `.decolint.jsonc` listing every rule at its default severity |
| `--rules` | print the built-in rules as a Markdown table |
| `-v`, `--version` | print version information |
| `-h`, `--help` | print usage |

## Config file

decolint looks for `.decolint.jsonc`, then `.decolint.json`, in the current
directory; the first one found is used. Pass `--config <path>` to use a file at
a different location instead. It is an error (exit code 2) if `--config` points
at a file that doesn't exist or fails to parse, or if the config references an
unknown rule ID or category name.

`categories` sets the severity (`error`, `warn`, or `off`) of every rule in a
[category](#rule-categories) at once; `rules` sets an individual rule's
severity and takes precedence over its category:

```jsonc
// .decolint.jsonc
{
  "platforms": ["vscode"],
  "categories": {
    "security": "error"
  },
  "rules": {
    "no-image-latest": "error",
    "pin-image-digest": "warn",
    "require-non-root": "off"
  }
}
```

`localEnv` maps names to the values `${localEnv:NAME}` resolves to when
[merging](#merging); it is also the environment Compose-file `${NAME}`
interpolation reads (note the differing [syntax](#merging)):

```jsonc
{
  "localEnv": { "USERPROFILE": "/home/user" }
}
```

The remaining members mirror their flags: `platforms`, `merge`,
`denyWarnings`, and `format` (see the [Configuration](#configuration) table).

## Target platforms

Each rule optionally targets specific platforms (`vscode`, `codespaces`); a
rule with no target platform applies to every platform and always runs. By
default, only those platform-agnostic rules run; pass `--platform` with a
comma-separated list to also run rules scoped to specific platforms:

```console
decolint --platform=vscode,codespaces
```

## Merging

The [Features](https://containers.dev/implementors/features/) a
`devcontainer.json` references, and the base image it names, both contribute
configuration of their own, which the Dev Container tooling merges into the
effective configuration following the specification's
[merge logic](https://containers.dev/implementors/spec/#merge-logic). By
default decolint lints only the raw file, so an issue introduced by a Feature
or inherited from the base image (say, one that sets `privileged: true` or
bind-mounts the Docker socket) goes unnoticed.

Enable merging to lint the merged configuration instead:

```console
decolint --merge
```

This fetches every referenced Feature and resolves the base image, including
any metadata in the base image's
[`devcontainer.metadata`](https://containers.dev/implementors/spec/#image-metadata)
label. A Feature or image that cannot be fetched is an error (exit code 2).

Merging also resolves the `${...}`
[variables](https://containers.dev/implementors/json_reference/#variables-in-devcontainerjson)
in the `devcontainer.json` first, so the reference a Feature or image is
fetched by, and the values the rules see, match what the real tooling would
use:

- `${localEnv:NAME}` (and `${env:NAME}`) resolves from the config file's
  [`localEnv`](#config-file) map only — decolint never reads environment
  variables. A name missing from the map resolves to the default in
  `${localEnv:NAME:default}`, or to the empty string.
- `${localWorkspaceFolder}` resolves to the linted directory's absolute path,
  `${containerWorkspaceFolder}` to the configuration's `workspaceFolder`
  (defaulting to `/workspaces/<folder name>`, or `/` for Docker Compose); each
  has a `...Basename` variant.
- `${devcontainerId}` resolves to a fixed placeholder with the format of a
  real id, since the real value exists only once a container is created.
- Anything else, including `${containerEnv:NAME}`, resolves to the empty
  string.

A few limits apply:

- For Docker Compose, `extends` and `include` are resolved as `docker compose
  config` would, and later files override earlier ones. Compose interpolation
  uses its own bare `${NAME}` syntax — not devcontainer.json's
  `${localEnv:NAME}` — but reads the same values from the config file's
  [`localEnv`](#config-file) map: an unset variable resolves to its default
  (`${VAR:-default}`) or the empty string, and a `${VAR:?}` requirement on an
  unset variable is an error. Compose profiles, the `COMPOSE_FILE` environment
  variable, and `.env` files are not applied.
- Registries are accessed anonymously, so a private image that cannot be
  pulled that way counts as a fetch failure.

## Output formats

Every format reports the configuration files that were linted, including those
with no finding, alongside the findings themselves. The default `text` format
additionally names the config file the settings came from:

```
Config: .decolint.jsonc
Linted 1 file:
  .devcontainer/devcontainer.json (devcontainer)

.devcontainer/devcontainer.json:4:12: warn: image "ubuntu:latest" uses the "latest" tag; pin a specific version (no-image-latest)
Found 0 errors and 1 warning.
```

With no config file, that first line says so and how to create one. It is
specific to `text` — the machine-readable formats carry the linted files but
not the config path. Written to a terminal, the report is colored by severity;
see [Color](#color).

Select a different output format to change this:

- `text` (default) — the format shown above.
- `json` — a JSON object of the linted files and the findings, for scripting:
  ```json
  {"files":[{"path":".devcontainer/devcontainer.json","type":"devcontainer"}],"issues":[{"path":".devcontainer/devcontainer.json","line":4,"col":12,"ruleId":"no-image-latest","message":"image \"ubuntu:latest\" uses the \"latest\" tag; pin a specific version","severity":"warn"}]}
  ```
- `github` — [GitHub Actions workflow
  commands](https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#setting-a-notice-message),
  so findings show up as inline annotations on pull request diffs. The linted
  files go into a collapsed group in the run log, so a file with no finding
  gets no annotation:
  ```
  ::group::decolint
  Linted 1 file:
    .devcontainer/devcontainer.json (devcontainer)
  ::endgroup::
  ::warning file=.devcontainer/devcontainer.json,line=4,col=12,title=no-image-latest::image "ubuntu:latest" uses the "latest" tag; pin a specific version
  ```
- `sarif` — a [SARIF
  2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
  log, for upload to [GitHub Code
  Scanning](https://docs.github.com/en/code-security/code-scanning/integrating-with-code-scanning/uploading-a-sarif-file-to-github)
  so findings appear as alerts in the repository's Security tab. The log
  declares every rule the run had enabled, so Code Scanning resolves the alerts
  of a rule that ran and found nothing, while leaving those of a rule you have
  turned off untouched.

Every format reports paths relative to the directory `decolint` runs in,
whichever way the linted directory was named on the command line. A file
outside that directory is reported with its absolute path.

## Color

The `text` format colors each finding by its severity when it is written to a
terminal, and writes plain text otherwise — so a report piped into another
command, or redirected to a file, stays free of escape sequences. The other
formats are never colored.

Pass `--color` to decide instead of leaving it to the terminal:

```console
decolint --color=always | less -R   # always color
decolint --color=never              # never color
```

Those two decide on their own. Under the default `--color=auto`, the `NO_COLOR`
and `FORCE_COLOR` environment variables apply instead: set `NO_COLOR` to any
non-empty value to turn color off, or `FORCE_COLOR` to color output that does
not go to a terminal, such as a CI log — `FORCE_COLOR=0` turns color off
instead. `NO_COLOR` wins over `FORCE_COLOR`.

## Exit codes

- `0` — no `error`-severity findings (there may still be `warn` findings)
- `1` — at least one `error`-severity finding was reported
- `2` — an error occurred (e.g. a file could not be parsed)

Enable deny-warnings (the `--deny-warnings` flag or `"denyWarnings": true`) to
also fail (exit code 1) on `warn`-severity findings. Exit codes are unaffected
by the output format.

## Rules

Every rule belongs to one [category](#rule-categories), which sets its
severity unless overridden by a [config file](#config-file). A rule can also
optionally target specific platforms (see [Target
platforms](#target-platforms)); a rule with no target platform applies to all
platforms.

The [rule reference](https://bare-devcontainer.github.io/decolint/rules/) lists
every rule by category, and each rule's page covers why it exists, the
configuration it accepts and rejects, and the specification it is based on. Run
`decolint --rules` to print the same list with the severity your configuration
gives each rule.

The [SARIF output](#output-formats) links every rule it reports to its page, so
the reasoning is one click away from the alert in GitHub Code Scanning.

### Rule categories

Only `correctness` runs by default; the rest are `off` until enabled:

- `correctness` (default `error`) — the configuration is invalid or does not
  behave as written.
- `security` (default `off`) — container runtime privileges and hardening.
- `reproducibility` (default `off`) — unpinned versions or digests that make
  the environment change over time.
- `style` (default `off`) — discouraged or legacy configuration that still
  works.


## Suppressing findings

Findings can be suppressed with comments in the configuration files:

- `decolint-ignore-line` — suppress findings on the same line, typically as a
  trailing comment
- `decolint-ignore-next-line` — suppress findings on the next line
- `decolint-ignore-file` — suppress findings in the whole file

Each directive optionally takes rule IDs, separated by commas or spaces;
omitting them suppresses all rules. Block comments (`/* ... */`) work the same
way.

```jsonc
// decolint-ignore-file no-app-port
{
  // decolint-ignore-next-line no-image-latest
  "image": "ubuntu:latest",
  "privileged": true // decolint-ignore-line
}
```

## Verifying release artifacts

Each [release](https://github.com/bare-devcontainer/decolint/releases)
includes a `decolint-<version>-checksums.txt` file listing the SHA-256
checksum of every binary, plus a
`decolint-<version>-checksums.txt.sigstore.json` file: a [Sigstore
bundle](https://github.com/sigstore/cosign/blob/main/specs/BUNDLE_SPEC.md)
containing the [cosign](https://github.com/sigstore/cosign) keyless signature
(signed via GitHub Actions OIDC) and its Rekor transparency log entry.

To verify a downloaded binary:

```console
cosign verify-blob \
  --bundle decolint-<version>-checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/bare-devcontainer/decolint/\.github/workflows/release\.yml@.*$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  decolint-<version>-checksums.txt

sha256sum --ignore-missing -c decolint-<version>-checksums.txt
```

The first command confirms the checksums file was signed by this repository's
release workflow; the second confirms the downloaded binary matches a checksum
in that file.

Each binary's provenance can also be verified with
[`gh attestation verify`](https://cli.github.com/manual/gh_attestation_verify),
using the [build
provenance](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds)
attested during the release:

```console
gh attestation verify decolint_<version>_<os>_<arch>.tar.gz \
  --repo bare-devcontainer/decolint
```

The container image carries the same kind of attestation:

```console
gh attestation verify oci://ghcr.io/bare-devcontainer/decolint:<version> \
  --repo bare-devcontainer/decolint
```
