# decolint

[![CI](https://github.com/bare-devcontainer/decolint/actions/workflows/ci.yml/badge.svg)](https://github.com/bare-devcontainer/decolint/actions/workflows/ci.yml)
[![Attestation Checks](https://github.com/bare-devcontainer/decolint/actions/workflows/attest-check.yml/badge.svg)](https://github.com/bare-devcontainer/decolint/actions/workflows/attest-check.yml)

decolint is a linter for [Dev Container](https://containers.dev/) configuration files. Following file types are supported:

- Dev Container definition (`devcontainer.json`)
- Feature (`devcontainer-feature.json`)
- Template (`devcontainer-template.json`)

It checks for common mistakes, security issues, and best practices in these files. See [Rules](#rules) for the list of checks decolint performs.

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

decolint supports the following flags; run `decolint -help` for the full
list.

| Flag | Description |
| --- | --- |
| `-platform` | comma-separated list of platforms to also lint against (see [Target platforms](#target-platforms)) |
| `-format` | output format: `text` (default), `json`, or `github` (see [Output formats](#output-formats)) |
| `-deny-warnings` | also exit non-zero on `warn`-severity findings (see [Exit codes](#exit-codes)) |
| `-config` | path to a config file overriding rule and category severities (see [Config file](#config-file)) |
| `-rules` | print the available rules |
| `-init` | write a new `.decolint.jsonc` listing every rule at its default severity (see [Config file](#config-file)) |

### Target platforms

Each rule optionally targets specific platforms (`vscode`,
`codespaces`, ...); a rule with no target platform applies to every
platform and always runs. By default, only those platform-agnostic
rules run; pass `-platform` with a comma-separated list to also run
rules scoped to specific platforms:

```console
decolint -platform=vscode,codespaces
```

### Output formats

By default, findings are printed one per line, with the rule's
severity (`error` or `warn`):

```
.devcontainer/devcontainer.json:4:12: warn: image "ubuntu:latest" uses the "latest" tag; pin a specific version (no-image-latest)
```

Pass `-format` to select a different output format:

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

### Exit codes

- `0` — no `error`-severity findings (there may still be `warn`
  findings)
- `1` — at least one `error`-severity finding was reported
- `2` — an error occurred (e.g. a file could not be parsed)

Pass `-deny-warnings` to also fail (exit code 1) on `warn`-severity
findings. Exit codes are unaffected by `-format`.

### Config file

Rule and category severities can be overridden per project with a
JSON/JSONC config file. Run `decolint -init` to generate a starting
`.decolint.jsonc` in the current directory, ready to edit:

```jsonc
// .decolint.jsonc
{
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
| `id-dir-mismatch` | `correctness` | (all) | disallow a Feature's or Template's `id` that does not match the name of its containing directory |
| `invalid-semver` | `correctness` | (all) | disallow a Feature's or Template's `version` that is not a valid semantic version |
| `missing-build-dockerfile` | `correctness` | (all) | disallow a devcontainer.json `build` object that is missing `dockerfile` |
| `missing-compose-service` | `correctness` | (all) | disallow a devcontainer.json that sets `dockerComposeFile` without `service` |
| `missing-container-def` | `correctness` | (all) | disallow a devcontainer.json that defines none of `image`, `build`, or `dockerComposeFile` |
| `missing-required-props` | `correctness` | (all) | disallow a Feature's or Template's metadata that is missing a required property (`id`, `version`, or `name`) |
| `missing-workspace-mount-folder` | `correctness` | (all) | disallow a devcontainer.json using `image` or `build` that sets only one of `workspaceMount` or `workspaceFolder` |
| `no-bind-mount` | `correctness` | `codespaces` | disallow `bind` type entries in `mounts`, which GitHub Codespaces silently ignores except for the Docker socket |
| `no-host-port-format` | `correctness` | `codespaces` | disallow `host:port` entries in `forwardPorts` and `portsAttributes`, which GitHub Codespaces does not support |
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
