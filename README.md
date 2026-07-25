# decolint

[![CI](https://github.com/bare-devcontainer/decolint/actions/workflows/ci.yml/badge.svg)](https://github.com/bare-devcontainer/decolint/actions/workflows/ci.yml)
[![Attestation Checks](https://github.com/bare-devcontainer/decolint/actions/workflows/attest-check.yml/badge.svg)](https://github.com/bare-devcontainer/decolint/actions/workflows/attest-check.yml)

decolint is a linter for [Dev Container](https://containers.dev/) configuration files. The following file types are supported:

- Dev Container definition (`devcontainer.json`)
- Feature (`devcontainer-feature.json`)
- Template (`devcontainer-template.json`)

It checks for common mistakes, security issues, and best practices in these files. See [Rules](#rules) for the list of checks decolint performs.

A container's final configuration comes not only from its own file but also from the base image and the Features it uses. With [`-merge`](#merging), decolint resolves all of these as they would be at runtime and lints the fully merged configuration.

## Installation

decolint can be installed as a prebuilt binary, as a container image, or
from source with Go.

### Prebuilt binary (Recommended)

Download a prebuilt binary from the
[releases page](https://github.com/bare-devcontainer/decolint/releases).
Release artifacts are signed and carry build provenance; see
[Verifying release artifacts](#verifying-release-artifacts).

### Docker

```console
docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint [directory ...]
```

Images are published for `linux/amd64` and `linux/arm64` and tagged
`latest`, `<major>`, `<major>.<minor>`, and `<major>.<minor>.<patch>`.
They carry the same build provenance attestation as the binaries; see
[Verifying release artifacts](#verifying-release-artifacts).

### Install with Go

```console
GOEXPERIMENT=jsonv2 go install github.com/bare-devcontainer/decolint/cmd/decolint@latest
```

`GOEXPERIMENT=jsonv2` is required because decolint uses the still
experimental `encoding/json/v2` standard library package.

## Usage

```console
decolint [directory ...]
```

Each directory is detected as one of the following based on its
layout, and the configuration files it contains are linted:

- **Dev container definition** — the `devcontainer.json` files at the
  locations defined by the devcontainer specification:
  `.devcontainer/devcontainer.json`, `.devcontainer.json`, and
  `.devcontainer/<folder>/devcontainer.json`
- **Feature** (contains `devcontainer-feature.json`) — that file
- **Template** (contains `devcontainer-template.json`) — that file,
  plus the dev container configuration the template ships

With no arguments, the current directory is linted.

### Configuration

Every linting setting can be declared in the [config file](#config-file).
The five scalar settings additionally have a command-line flag, which
overrides the config file when given:

| Setting | Config member | Flag |
| --- | --- | --- |
| [Target platforms](#target-platforms) | `platforms` | `-platform` |
| [Merge features](#merging) | `merge` | `-merge` |
| [Schema validation](#schema-validation) | `schema` | `-schema` |
| [Deny warnings](#exit-codes) | `denyWarnings` | `-deny-warnings` |
| [Output format](#output-formats) | `format` | `-format` |
| [Category severities](#rule-categories) | `categories` | — |
| [Rule severities](#rules) | `rules` | — |

`-merge` and `-deny-warnings` override in either direction when given
explicitly — e.g. `-merge=false` disables merging even if the config
file sets `"merge": true`. Category and rule severities are config-file
only.

The remaining flags perform a one-off action and exit; run
`decolint -help` for the full list:

| Flag | Action |
| --- | --- |
| `-config <path>` | use the config file at `<path>` instead of [auto-discovery](#config-file) |
| `-init` | write a new `.decolint.jsonc` listing every rule at its default severity |
| `-rules` | print the built-in rules as a Markdown table |
| `-version` | print version information |
| `-help` | print usage |

### Target platforms

Each rule optionally targets specific platforms (`vscode`,
`codespaces`, ...); a rule with no target platform applies to every
platform and always runs. By default, only those platform-agnostic
rules run; pass `-platform` with a comma-separated list to also run
rules scoped to specific platforms:

```console
decolint -platform=vscode,codespaces
```

### Merging

The [Features](https://containers.dev/implementors/features/) a
`devcontainer.json` references, and the base image it names, both
contribute configuration of their own, which the Dev Container tooling
merges into the effective configuration following the specification's
[merge logic](https://containers.dev/implementors/spec/#merge-logic). By
default decolint lints only the raw file, so an issue introduced by a
Feature or inherited from the base image (say, one that sets
`privileged: true` or bind-mounts the Docker socket) goes unnoticed.

Enable merging to lint the merged configuration instead:

```console
decolint -merge
```

This fetches every referenced Feature and resolves the base image,
including any metadata in the base image's
[`devcontainer.metadata`](https://containers.dev/implementors/spec/#image-metadata)
label. A Feature or image that cannot be fetched is an error (exit code
2).

Merging also resolves the `${...}`
[variables](https://containers.dev/implementors/json_reference/#variables-in-devcontainerjson)
in the `devcontainer.json` first, so the reference a Feature or image is
fetched by, and the values the rules see, match what the real tooling
would use:

- `${localEnv:NAME}` (and `${env:NAME}`) resolves from the config
  file's [`localEnv`](#config-file) map only — decolint never reads
  environment variables. A name missing from the map resolves to the
  default in `${localEnv:NAME:default}`, or to the empty string.
- `${localWorkspaceFolder}` resolves to the linted directory's absolute
  path, `${containerWorkspaceFolder}` to the configuration's
  `workspaceFolder` (defaulting to `/workspaces/<folder name>`, or `/`
  for Docker Compose); each has a `...Basename` variant.
- `${devcontainerId}` resolves to a fixed placeholder with the format
  of a real id, since the real value exists only once a container is
  created.
- Anything else, including `${containerEnv:NAME}`, resolves to the
  empty string.

A few limits apply:

- For Docker Compose, `extends` and `include` are resolved as `docker
  compose config` would, and later files override earlier ones. Compose
  interpolation uses its own bare `${NAME}` syntax — not
  devcontainer.json's `${localEnv:NAME}` — but reads the same values from
  the config file's [`localEnv`](#config-file) map: an unset variable
  resolves to its default (`${VAR:-default}`) or the empty string, and a
  `${VAR:?}` requirement on an unset variable is an error. Compose
  profiles, the `COMPOSE_FILE` environment variable, and `.env` files are
  not applied.
- Registries are accessed anonymously, so a private image that cannot be
  pulled that way counts as a fetch failure.

### Schema validation

Alongside the rules, decolint validates each file against the official
[Dev Container JSON Schema](https://containers.dev/implementors/json_reference/):
`devcontainer.json` and Feature (`devcontainer-feature.json`) files are
checked for the things the rules do not — misspelled or unknown property
names, wrong value types, and out-of-range enum values:

```
.devcontainer/devcontainer.json:8:3: error: unknown property "forwardPort"; did you mean "forwardPorts"? (schema-validation)
```

Schema findings are always reported as errors under the rule ID
`schema-validation`. They are not affected by category or rule severity
overrides, and [ignore comments](#suppressing-findings) do not suppress
them; choose the variant, or turn validation off entirely, instead:

- `main` (default) — validates `devcontainer.json` against the schema
  including the Visual Studio Code and GitHub Codespaces extensions, so
  their properties (such as `customizations.vscode`) are accepted.
- `base` — validates against the specification's base schema only, so
  editor-specific properties are reported as unknown.
- `off` — disables schema validation.

```console
decolint -schema=base
```

The file is validated as written, before any [merging](#merging) or
variable substitution, so `${...}` placeholders in string values never
trip it. Templates are not schema-validated, but the `devcontainer.json`
a template ships is.

### Output formats

By default, findings are printed one per line, with the rule's
severity (`error` or `warn`):

```
.devcontainer/devcontainer.json:4:12: warn: image "ubuntu:latest" uses the "latest" tag; pin a specific version (no-image-latest)
```

Select a different output format to change this:

- `text` (default) — the one-line-per-finding format shown above.
- `json` — a JSON array of finding objects, for scripting:
  ```json
  [{"path":".devcontainer/devcontainer.json","line":4,"col":12,"ruleId":"no-image-latest","message":"image \"ubuntu:latest\" uses the \"latest\" tag; pin a specific version","severity":"warn"}]
  ```
- `github` — [GitHub Actions workflow
  commands](https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#setting-a-notice-message),
  so findings show up as inline annotations on pull request diffs when
  `decolint` is run from a GitHub Actions workflow:
  ```
  ::warning file=.devcontainer/devcontainer.json,line=4,col=12,title=no-image-latest::image "ubuntu:latest" uses the "latest" tag; pin a specific version
  ```
- `sarif` — a [SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
  log, for upload to [GitHub Code
  Scanning](https://docs.github.com/en/code-security/code-scanning/integrating-with-code-scanning/uploading-a-sarif-file-to-github)
  so findings appear as alerts in the repository's Security tab:
  ```yaml
  - run: decolint -merge -format=sarif . > decolint.sarif
    continue-on-error: true # decolint exits 1 on findings; the upload must still run
  - uses: github/codeql-action/upload-sarif@v3
    with:
      sarif_file: decolint.sarif
  ```
  Run `decolint` from the repository root, so that the paths in the log
  resolve to files in the repository.

Every format reports paths relative to the directory `decolint` runs in,
whichever way the linted directory was named on the command line. A file
outside that directory is reported with its absolute path.

### Exit codes

- `0` — no `error`-severity findings (there may still be `warn`
  findings)
- `1` — at least one `error`-severity finding was reported
- `2` — an error occurred (e.g. a file could not be parsed)

Enable deny-warnings (the `-deny-warnings` flag or `"denyWarnings":
true`) to also fail (exit code 1) on `warn`-severity findings. Exit
codes are unaffected by the output format.

### Config file

Every setting from the [Configuration](#configuration) table can be
declared per project in a JSON/JSONC config file. Run `decolint -init`
to generate a starting `.decolint.jsonc` in the current directory,
ready to edit:

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

`categories` sets the severity (`error`, `warn`, or `off`) of every
rule in a [category](#rule-categories) at once; `rules` sets an
individual rule's severity and takes precedence over its category.
`localEnv` maps names to the values `${localEnv:NAME}` resolves to when
[merging](#merging); it is also the environment Compose-file `${NAME}`
interpolation reads (note the differing [syntax](#merging)):

```jsonc
{
  "localEnv": { "USERPROFILE": "/home/user" }
}
```

The remaining members mirror their flags: `platforms`, `merge`,
`schema`, `denyWarnings`, and `format` (see the
[Configuration](#configuration) table).

For the strictest configuration, enable every category:

```jsonc
// .decolint.jsonc
{
  "categories": {
    "security": "error",
    "reproducibility": "error",
    "style": "error"
  }
}
```

decolint looks for `.decolint.jsonc`, then `.decolint.json`, in the current
directory; the first one found is used. Pass `-config <path>` to use a
file at a different location instead. It is an error (exit code 2) if
`-config` points at a file that doesn't exist or fails to parse, or if
the config references an unknown rule ID or category name.

## Rules

Every rule belongs to one [category](#rule-categories), which sets its
severity unless overridden by a [config file](#config-file). A rule
can also optionally target specific platforms (see [Target
platforms](#target-platforms)); a rule with no target platform applies
to all platforms.

### Rule categories

Only `correctness` runs by default; the rest are `off` until enabled:

- `correctness` (default `error`) — the configuration is invalid or
  does not behave as written.
- `security` (default `off`) — container runtime privileges and
  hardening.
- `reproducibility` (default `off`) — unpinned versions or digests
  that make the environment change over time.
- `style` (default `off`) — discouraged or legacy configuration that
  still works.

<!-- Keep this table sorted by Category (correctness, security, reproducibility, style), then ID, in that priority order. -->
| ID | Category | Platform | Description |
| --- | --- | --- | --- |
| `conflicting-container-def` | `correctness` | (all) | disallow a devcontainer.json that defines more than one of `image`, `build`, or `dockerComposeFile` |
| `feature-install-script-not-executable` | `correctness` | (all) | disallow a Feature's `install.sh` that lacks executable permission bits |
| `id-dir-mismatch` | `correctness` | (all) | disallow a Feature's or Template's `id` that does not match the name of its containing directory |
| `invalid-semver` | `correctness` | (all) | disallow a Feature's or Template's `version` that is not a valid semantic version |
| `missing-build-dockerfile` | `correctness` | (all) | disallow a devcontainer.json `build` object that is missing `dockerfile` |
| `missing-compose-service` | `correctness` | (all) | disallow a devcontainer.json that sets `dockerComposeFile` without `service` |
| `missing-container-def` | `correctness` | (all) | disallow a devcontainer.json that defines none of `image`, `build`, or `dockerComposeFile` |
| `missing-feature-install-script` | `correctness` | (all) | disallow a Feature directory without the required `install.sh` install script |
| `missing-required-props` | `correctness` | (all) | disallow a Feature's or Template's metadata that is missing a required property (`id`, `version`, or `name`) |
| `missing-workspace-mount-folder` | `correctness` | (all) | disallow a devcontainer.json using `image` or `build` that sets only one of `workspaceMount` or `workspaceFolder` |
| `no-bind-mount` | `correctness` | `codespaces` | disallow `bind` type entries in `mounts`, which GitHub Codespaces silently ignores except for the Docker socket |
| `no-host-port-format` | `correctness` | `codespaces` | disallow `host:port` entries in `forwardPorts` and `portsAttributes`, which GitHub Codespaces does not support |
| `undefined-template-option` | `correctness` | (all) | disallow a `${templateOption:...}` reference to an option not declared in devcontainer-template.json |
| `no-cap-add-all` | `security` | (all) | disallow granting all Linux capabilities via an `ALL` entry in a devcontainer.json's or Feature's `capAdd` property, or a `--cap-add=ALL` entry in a devcontainer.json's `runArgs` |
| `no-docker-socket-mount` | `security` | (all) | disallow bind-mounting the host's Docker socket via a devcontainer.json's `mounts` or `runArgs`, which grants the container root-equivalent control over the host |
| `no-privileged-container` | `security` | (all) | disallow running the container in privileged mode via a devcontainer.json's or Feature's `privileged` property, or a `--privileged` entry in a devcontainer.json's `runArgs` |
| `no-seccomp-override` | `security` | (all) | disallow overriding the container runtime's default seccomp profile via a devcontainer.json's or Feature's `securityOpt` property, or a `--security-opt seccomp=...` entry in a devcontainer.json's `runArgs` |
| `no-seccomp-unconfined` | `security` | (all) | disallow disabling seccomp confinement via a devcontainer.json's or Feature's `securityOpt` property, or a `--security-opt seccomp=unconfined` entry in a devcontainer.json's `runArgs` |
| `require-cap-drop-all` | `security` | (all) | require an `ALL` entry in a devcontainer.json's `capDrop` property, or a `--cap-drop=ALL` entry in `runArgs`, dropping every Linux capability |
| `require-no-new-privileges` | `security` | (all) | require `no-new-privileges` to be set via a devcontainer.json's `securityOpt` property, or a `--security-opt no-new-privileges...` entry in `runArgs` |
| `require-non-root` | `security` | (all) | require `remoteUser` or, if unset, `containerUser` to be set to a non-root user |
| `no-image-latest` | `reproducibility` | (all) | disallow container images without an explicit tag or with the `latest` tag |
| `pin-extension-version` | `reproducibility` | `vscode`, `codespaces` | disallow a `customizations.vscode.extensions` entry without an explicit pinned version |
| `pin-feature-version` | `reproducibility` | (all) | disallow a Feature reference without an explicit version or with the `latest` version |
| `pin-image-digest` | `reproducibility` | (all) | disallow an `image` property that does not pin the image by content digest (e.g. `image@sha256:...`) |
| `no-app-port` | `style` | (all) | disallow the legacy `appPort` property in favor of `forwardPorts` |
| `unused-template-option` | `style` | (all) | disallow a Template option that no file in the Template references |

## Suppressing findings

Findings can be suppressed with comments in the configuration files:

- `decolint-ignore-line` — suppress findings on the same line, typically
  as a trailing comment
- `decolint-ignore-next-line` — suppress findings on the next line
- `decolint-ignore-file` — suppress findings in the whole file

Each directive optionally takes rule IDs, separated by commas or
spaces; omitting them suppresses all rules. Block comments
(`/* ... */`) work the same way.

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
containing the [cosign](https://github.com/sigstore/cosign) keyless
signature (signed via GitHub Actions OIDC) and its Rekor transparency
log entry.

To verify a downloaded binary:

```console
cosign verify-blob \
  --bundle decolint-<version>-checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/bare-devcontainer/decolint/\.github/workflows/release\.yml@.*$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  decolint-<version>-checksums.txt

sha256sum --ignore-missing -c decolint-<version>-checksums.txt
```

The first command confirms the checksums file was signed by this
repository's release workflow; the second confirms the downloaded
binary matches a checksum in that file.

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

## Contributing

Rules are plain Go code and new ones are easy to add; see
[CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and a
walkthrough of adding a rule.

## License

[MIT](LICENSE)
