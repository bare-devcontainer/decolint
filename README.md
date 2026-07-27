# decolint

[![CI](https://github.com/bare-devcontainer/decolint/actions/workflows/ci.yml/badge.svg)](https://github.com/bare-devcontainer/decolint/actions/workflows/ci.yml)
[![Attestation Checks](https://github.com/bare-devcontainer/decolint/actions/workflows/attest-check.yml/badge.svg)](https://github.com/bare-devcontainer/decolint/actions/workflows/attest-check.yml)

<!-- decolint:page=_index -->
decolint is a linter for [Dev Container](https://containers.dev/) configuration
files: `devcontainer.json`, `devcontainer-feature.json`, and
`devcontainer-template.json`. It reports mistakes, container privileges, and
unpinned versions that the Dev Container tooling itself accepts without a word.

Take a `devcontainer.json` that opens cleanly in VS Code and builds without
complaint:

```jsonc
// .devcontainer/devcontainer.json
{
  "name": "api",
  "image": "mcr.microsoft.com/devcontainers/go:latest",
  "features": {
    "ghcr.io/devcontainers/features/docker-in-docker": {}
  },
  "runArgs": ["--privileged"],
  "mounts": [
    "source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"
  ],
  "forwardPorts": ["db:5432"]
}
```

```console
$ decolint .
Config: .decolint.jsonc
Linted 1 file:
  .devcontainer/devcontainer.json (devcontainer)

.devcontainer/devcontainer.json:1:1: error: "ALL" is not set via "runArgs", leaving the container with its default Linux capabilities (require-cap-drop-all)
.devcontainer/devcontainer.json:1:1: error: "no-new-privileges" is not set via "securityOpt" or "runArgs", allowing container processes to gain additional privileges (require-no-new-privileges)
.devcontainer/devcontainer.json:1:1: error: neither "remoteUser" nor "containerUser" is set, so the container defaults to running as root (require-non-root)
.devcontainer/devcontainer.json:3:12: error: image "mcr.microsoft.com/devcontainers/go:latest" uses the "latest" tag; pin a specific version (no-image-latest)
.devcontainer/devcontainer.json:3:12: error: image "mcr.microsoft.com/devcontainers/go:latest" is not pinned by digest; add an "@sha256:..." digest (pin-image-digest)
.devcontainer/devcontainer.json:5:5: error: feature "ghcr.io/devcontainers/features/docker-in-docker" has no explicit version; pin a specific version (pin-feature-version)
.devcontainer/devcontainer.json:7:15: error: "runArgs" contains "--privileged", disabling the container's isolation from the host (no-privileged-container)
.devcontainer/devcontainer.json:9:5: error: "mounts" entry bind-mounts the Docker socket, which grants the container root-equivalent control over the host (no-docker-socket-mount)
.devcontainer/devcontainer.json:11:20: error: "forwardPorts" entry "db:5432" uses "host:port" format; Codespaces only supports a bare port number (no-host-port-format)
Found 9 errors and 0 warnings.
```

That run has every category and both target platforms enabled; the defaults
are quieter. See [Set up your project](#set-up-your-project).

## Why decolint

- **A `devcontainer.json` is container runtime configuration, and nobody
  reviews it that way.** `--privileged` and a bind-mounted Docker socket read
  like ordinary setup lines, and they ship to every teammate and every CI run
  that rebuilds the container.
- **It lints what actually runs, not just what you wrote.** The Features you
  reference and the base image you name contribute configuration of their own.
  With [`-merge`](#4-lint-what-actually-runs), decolint resolves them the way
  the real tooling does and lints the result.
- **A silently ignored property is a mistake decolint names.** GitHub
  Codespaces drops `bind` mounts and rejects `host:port` entries without
  reporting anything; the container just comes up wrong.
- **It goes where your other linters already are.** Findings carry a line and
  column, and come out as text, JSON, GitHub Actions annotations, or SARIF.
- **One static binary.** No Node.js, no Docker daemon, no project
  dependencies.
<!-- decolint:end-page -->

<!-- decolint:page=getting-started -->
## Try it

Run it against your own repository, without installing anything:

```console
docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint
```

By default only the `correctness` checks run — configuration that is invalid
or does not behave as written. The security, pinning, and style checks are
opt-in, and one config file away.

## Install

### Prebuilt binary (recommended)

Download a binary from the
[releases page](https://github.com/bare-devcontainer/decolint/releases).
Release artifacts are signed and carry build provenance; see
[Verifying release artifacts](#verifying-release-artifacts).

### Container image

```console
docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint [directory ...]
```

Images are published for `linux/amd64` and `linux/arm64` and tagged `latest`,
`<major>`, `<major>.<minor>`, and `<major>.<minor>.<patch>`. They carry the
same build provenance attestation as the binaries; see
[Verifying release artifacts](#verifying-release-artifacts).

### Go

```console
GOEXPERIMENT=jsonv2 go install github.com/bare-devcontainer/decolint/cmd/decolint@latest
```

`GOEXPERIMENT=jsonv2` is required because decolint uses the still experimental
`encoding/json/v2` standard library package.

## Set up your project

### 1. Run it

With no arguments the current directory is linted; name directories to lint
those instead. Whatever you point it at is detected as a dev container
definition, a Feature, or a Template — see
[What decolint lints](#what-decolint-lints).

Run it on the same file the demo above found nine problems in, and it comes
back clean:

```console
$ decolint
Config: none (defaults; run "decolint -init" to create .decolint.jsonc)
Linted 1 file:
  .devcontainer/devcontainer.json (devcontainer)

Found 0 errors and 0 warnings.
```

The header says why: no config file, so only the defaults are in effect. It
also lists what was covered, which is the other thing worth checking when a
run reports nothing.

### 2. Turn on the checks you want

The defaults are `correctness` alone. Everything else — the container
privileges, the unpinned versions, the legacy properties — is off until you
ask for it. Write a `.decolint.jsonc` in your repository root:

```jsonc
// .decolint.jsonc
{
  "categories": {
    "correctness": "error",
    "security": "error",
    "reproducibility": "error",
    "style": "error"
  }
}
```

That is the strictest setting. Start narrower if a whole category is more than
you want today, and set individual rules where a category is close but not
right:

```jsonc
{
  "categories": {
    "security": "error"
  },
  "rules": {
    "no-image-latest": "error",
    "require-non-root": "off"
  }
}
```

Every severity is `error`, `warn`, or `off`, and a `rules` entry beats its
category. Run `decolint -rules` to see every rule with the severity your
config gives it, or `decolint -init` to generate a config that lists all of
them explicitly. The full list is under [Rules](#rules), and everything the
file accepts is under [Config file](#config-file).

### 3. Name your platform

Some rules only make sense on a particular platform — Codespaces ignoring
`bind` mounts, VS Code pinning extension versions — and those stay off until
you say which platforms you target:

```jsonc
{
  "platforms": ["vscode", "codespaces"]
}
```

### 4. Lint what actually runs

Your `devcontainer.json` is not the whole configuration. Features and the base
image contribute their own, and the tooling merges it all together before the
container starts. Take a file that is careful about all of it — pinned by
digest, capabilities dropped, no new privileges:

```jsonc
// .devcontainer/devcontainer.json
{
  "name": "api",
  "image": "mcr.microsoft.com/devcontainers/go:1.24@sha256:8de3d5b3a3ce235671c7649f0b910414158a220d18cbd2714a4446cc0cc6acd3",
  "runArgs": ["--cap-drop=ALL"],
  "securityOpt": ["no-new-privileges"]
}
```

Linted as written it reports one problem. Merging replaces it with four:

```console
$ decolint .
Config: .decolint.jsonc
Linted 1 file:
  .devcontainer/devcontainer.json (devcontainer)

.devcontainer/devcontainer.json:1:1: error: neither "remoteUser" nor "containerUser" is set, so the container defaults to running as root (require-non-root)
Found 1 error and 0 warnings.

$ decolint -merge .
Downloading image metadata(mcr.microsoft.com/devcontainers/go:1.24@sha256:8de3d5b3a3ce235671c7649f0b910414158a220d18cbd2714a4446cc0cc6acd3)
Config: .decolint.jsonc
Linted 1 file:
  .devcontainer/devcontainer.json (devcontainer)

.devcontainer/devcontainer.json:3:3: error: "securityOpt" overrides the default seccomp profile (no-seccomp-override)
.devcontainer/devcontainer.json:3:3: error: "securityOpt" contains "seccomp=unconfined", disabling the container's syscall filtering (no-seccomp-unconfined)
.devcontainer/devcontainer.json:3:3: error: extension "golang.Go" has no explicit version; pin a specific version (pin-extension-version)
.devcontainer/devcontainer.json:3:3: error: extension "dbaeumer.vscode-eslint" has no explicit version; pin a specific version (pin-extension-version)
Found 4 errors and 0 warnings.
```

The one reported without merging was wrong: the base image sets a non-root
user, so `require-non-root` never applied. The four reported with merging are
in nothing you wrote — the image disables seccomp and installs two unpinned VS
Code extensions. Findings that come from merged content are reported at the
property that pulled it in, here the `image` line.

Turn it on for good with `"merge": true`, or pass `-merge` per run. It fetches
every referenced Feature and resolves the base image, so it needs network
access; see [Merging](#merging) for what gets resolved and what does not.

### 5. Fix it, or say why not

When a finding is deliberate, suppress it in the configuration file itself so
the reason lives next to the line:

```jsonc
{
  // decolint-ignore-next-line no-image-latest
  "image": "ubuntu:latest"
}
```

See [Suppressing findings](#suppressing-findings) for file- and line-scoped
directives.

## Add it to CI

decolint exits `1` when it reports an `error`, which is all a CI job needs to
fail. Add `-format=github` and the findings also appear as annotations on the
pull request diff:

```yaml
name: devcontainer
on: pull_request
permissions: {}

jobs:
  decolint:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v7
        with:
          persist-credentials: false
      - run: docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint -format=github .
```

Only findings become annotations; the files that were linted go to a collapsed
group in the run log, so a clean file does not annotate the diff. Warnings do
not fail the build on their own; add `-deny-warnings` to make them count. See
[Exit codes](#exit-codes).

To have findings tracked as alerts in the repository's Security tab instead,
emit [SARIF](#output-formats) and upload it. The upload writes to code
scanning, so this job adds `security-events: write` to the permissions above:

```yaml
  decolint-sarif:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@v7
        with:
          persist-credentials: false
      - run: docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint -format=sarif . > decolint.sarif
        continue-on-error: true # decolint exits 1 on findings; the upload must still run
      - uses: github/codeql-action/upload-sarif@v4
        with:
          sarif_file: decolint.sarif
```

A pull request from a fork gets a read-only token whatever the job asks for,
so the upload cannot succeed there; skip the job on fork pull requests, or run
it only on pushes to your default branch.

Run decolint from the repository root, so the reported paths resolve to files
in the repository.

## Linting a Feature or Template

If you publish a
[Feature](https://containers.dev/implementors/features/) or a
[Template](https://containers.dev/implementors/templates/), the mistakes that
cost the most are the ones your consumers find after you have shipped: an `id`
that does not match its directory, a version that is not semver, an
`install.sh` that was committed without its executable bit. Point decolint at
the directory — these are `correctness` rules, so they need no configuration:

```console
$ decolint src/go-tools
Config: none (defaults; run "decolint -init" to create .decolint.jsonc)
Linted 1 file:
  src/go-tools/devcontainer-feature.json (feature)

src/go-tools/devcontainer-feature.json:1:1: error: "install.sh" is not executable (mode 0644); run "chmod +x install.sh" (feature-install-script-not-executable)
src/go-tools/devcontainer-feature.json:2:9: error: id "gotools" does not match containing directory "go-tools" (id-dir-mismatch)
src/go-tools/devcontainer-feature.json:3:14: error: version "1.0" is not a valid semantic version (see https://semver.org/) (invalid-semver)
Found 3 errors and 0 warnings.
```

A Template directory is linted the same way, and the dev container
configuration the Template ships is linted along with it, including its
`${templateOption:...}` references.
<!-- decolint:end-page -->

---

# Reference

<!-- decolint:page=reference -->
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
| [Target platforms](#target-platforms) | `platforms` | `-platform` |
| [Merge features](#merging) | `merge` | `-merge` |
| [Deny warnings](#exit-codes) | `denyWarnings` | `-deny-warnings` |
| [Output format](#output-formats) | `format` | `-format` |
| [Category severities](#rule-categories) | `categories` | — |
| [Rule severities](#rules) | `rules` | — |

`-merge` and `-deny-warnings` override in either direction when given
explicitly — e.g. `-merge=false` disables merging even if the config file sets
`"merge": true`. Category and rule severities are config-file only, and
[color](#color) is set by the `-color` flag only.

The remaining flags perform a one-off action and exit; run `decolint -help`
for the full list:

| Flag | Action |
| --- | --- |
| `-config <path>` | use the config file at `<path>` instead of [auto-discovery](#config-file) |
| `-init` | write a new `.decolint.jsonc` listing every rule at its default severity |
| `-rules` | print the built-in rules as a Markdown table, or as JSON with `-format=json` |
| `-version` | print version information |
| `-help` | print usage |

`-rules` only recognizes `-format`'s `text` (the default) and `json`; the
JSON catalog carries every field a rule's page does (rationale, references,
example, docs address) plus the severity your config file currently gives
it. `-format=github` and `-format=sarif` describe lint findings, not the
rule catalog, so they are an error with `-rules`.

## Config file

decolint looks for `.decolint.jsonc`, then `.decolint.json`, in the current
directory; the first one found is used. Pass `-config <path>` to use a file at
a different location instead. It is an error (exit code 2) if `-config` points
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
default, only those platform-agnostic rules run; pass `-platform` with a
comma-separated list to also run rules scoped to specific platforms:

```console
decolint -platform=vscode,codespaces
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
decolint -merge
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

Pass `-color` to decide instead of leaving it to the terminal:

```console
decolint -color=always | less -R   # always color
decolint -color=never              # never color
```

Those two decide on their own. Under the default `-color=auto`, the `NO_COLOR`
and `FORCE_COLOR` environment variables apply instead: set `NO_COLOR` to any
non-empty value to turn color off, or `FORCE_COLOR` to color output that does
not go to a terminal, such as a CI log — `FORCE_COLOR=0` turns color off
instead. `NO_COLOR` wins over `FORCE_COLOR`.

## Exit codes

- `0` — no `error`-severity findings (there may still be `warn` findings)
- `1` — at least one `error`-severity finding was reported
- `2` — an error occurred (e.g. a file could not be parsed)

Enable deny-warnings (the `-deny-warnings` flag or `"denyWarnings": true`) to
also fail (exit code 1) on `warn`-severity findings. Exit codes are unaffected
by the output format.

## Rules

Every rule belongs to one [category](#rule-categories), which sets its
severity unless overridden by a [config file](#config-file). A rule can also
optionally target specific platforms (see [Target
platforms](#target-platforms)); a rule with no target platform applies to all
platforms.

Every rule has a page on the [documentation
site](https://bare-devcontainer.github.io/decolint/rules/) covering
why it exists, the configuration it accepts and rejects, and the
specification it is based on. Each rule ID in the table below links to
its page.

The [SARIF output](#output-formats) links every rule it reports to the
same page, so the reasoning is one click away from the alert in GitHub
Code Scanning.

### Rule categories

Only `correctness` runs by default; the rest are `off` until enabled:

- `correctness` (default `error`) — the configuration is invalid or does not
  behave as written.
- `security` (default `off`) — container runtime privileges and hardening.
- `reproducibility` (default `off`) — unpinned versions or digests that make
  the environment change over time.
- `style` (default `off`) — discouraged or legacy configuration that still
  works.

<!-- decolint:rules-table -->
| ID | Category | Platform | Description |
| --- | --- | --- | --- |
| [`conflicting-container-def`](https://bare-devcontainer.github.io/decolint/rules/conflicting-container-def/) | `correctness` | (all) | disallow a devcontainer.json that defines more than one of "image", "build", or "dockerComposeFile" |
| [`feature-install-script-not-executable`](https://bare-devcontainer.github.io/decolint/rules/feature-install-script-not-executable/) | `correctness` | (all) | disallow a Feature's `install.sh` that lacks executable permission bits |
| [`id-dir-mismatch`](https://bare-devcontainer.github.io/decolint/rules/id-dir-mismatch/) | `correctness` | (all) | disallow a Feature's or Template's "id" that does not match the name of its containing directory |
| [`invalid-semver`](https://bare-devcontainer.github.io/decolint/rules/invalid-semver/) | `correctness` | (all) | disallow a Feature's or Template's "version" that is not a valid semantic version |
| [`missing-build-dockerfile`](https://bare-devcontainer.github.io/decolint/rules/missing-build-dockerfile/) | `correctness` | (all) | disallow a devcontainer.json "build" object that is missing "dockerfile" |
| [`missing-compose-service`](https://bare-devcontainer.github.io/decolint/rules/missing-compose-service/) | `correctness` | (all) | disallow a devcontainer.json that sets "dockerComposeFile" without "service" |
| [`missing-container-def`](https://bare-devcontainer.github.io/decolint/rules/missing-container-def/) | `correctness` | (all) | disallow a devcontainer.json that defines none of "image", "build", or "dockerComposeFile" |
| [`missing-feature-install-script`](https://bare-devcontainer.github.io/decolint/rules/missing-feature-install-script/) | `correctness` | (all) | disallow a Feature directory without the required `install.sh` install script |
| [`missing-required-props`](https://bare-devcontainer.github.io/decolint/rules/missing-required-props/) | `correctness` | (all) | disallow a Feature's or Template's metadata that is missing a required property ("id", "version", or "name") |
| [`missing-workspace-mount-folder`](https://bare-devcontainer.github.io/decolint/rules/missing-workspace-mount-folder/) | `correctness` | (all) | disallow a devcontainer.json using "image" or "build" that sets only one of "workspaceMount" or "workspaceFolder" |
| [`no-bind-mount`](https://bare-devcontainer.github.io/decolint/rules/no-bind-mount/) | `correctness` | `codespaces` | disallow "bind" type entries in "mounts", which GitHub Codespaces silently ignores except for the Docker socket |
| [`no-host-port-format`](https://bare-devcontainer.github.io/decolint/rules/no-host-port-format/) | `correctness` | `codespaces` | disallow "host:port" entries in "forwardPorts" and "portsAttributes", which GitHub Codespaces does not support |
| [`undefined-template-option`](https://bare-devcontainer.github.io/decolint/rules/undefined-template-option/) | `correctness` | (all) | disallow a `${templateOption:...}` reference to an option not declared in devcontainer-template.json |
| [`no-cap-add-all`](https://bare-devcontainer.github.io/decolint/rules/no-cap-add-all/) | `security` | (all) | disallow granting all Linux capabilities via an "ALL" entry in the "capAdd" property, or a "--cap-add=ALL" entry in a devcontainer.json's "runArgs" |
| [`no-docker-socket-mount`](https://bare-devcontainer.github.io/decolint/rules/no-docker-socket-mount/) | `security` | (all) | disallow bind-mounting the host's Docker socket via a devcontainer.json's "mounts" or "runArgs", which grants the container root-equivalent control over the host |
| [`no-privileged-container`](https://bare-devcontainer.github.io/decolint/rules/no-privileged-container/) | `security` | (all) | disallow running the container in privileged mode via the "privileged" property or a "--privileged" entry in "runArgs" |
| [`no-seccomp-override`](https://bare-devcontainer.github.io/decolint/rules/no-seccomp-override/) | `security` | (all) | disallow overriding the container runtime's default seccomp profile via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt seccomp=..." entry in a devcontainer.json's "runArgs" |
| [`no-seccomp-unconfined`](https://bare-devcontainer.github.io/decolint/rules/no-seccomp-unconfined/) | `security` | (all) | disallow disabling seccomp confinement via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt seccomp=unconfined" entry in a devcontainer.json's "runArgs" |
| [`require-cap-drop-all`](https://bare-devcontainer.github.io/decolint/rules/require-cap-drop-all/) | `security` | (all) | require a "--cap-drop=ALL" entry in a devcontainer.json's "runArgs", dropping every Linux capability |
| [`require-no-new-privileges`](https://bare-devcontainer.github.io/decolint/rules/require-no-new-privileges/) | `security` | (all) | require "no-new-privileges" to be set via a devcontainer.json's "securityOpt" property, or a "--security-opt no-new-privileges..." entry in "runArgs" |
| [`require-non-root`](https://bare-devcontainer.github.io/decolint/rules/require-non-root/) | `security` | (all) | require "remoteUser" or, if unset, "containerUser" to be set to a non-root user |
| [`no-image-latest`](https://bare-devcontainer.github.io/decolint/rules/no-image-latest/) | `reproducibility` | (all) | disallow container images without an explicit tag or with the "latest" tag |
| [`pin-extension-version`](https://bare-devcontainer.github.io/decolint/rules/pin-extension-version/) | `reproducibility` | `vscode`, `codespaces` | disallow a "customizations.vscode.extensions" entry without an explicit pinned version |
| [`pin-feature-version`](https://bare-devcontainer.github.io/decolint/rules/pin-feature-version/) | `reproducibility` | (all) | disallow a Feature reference without an explicit version or with the "latest" version |
| [`pin-image-digest`](https://bare-devcontainer.github.io/decolint/rules/pin-image-digest/) | `reproducibility` | (all) | disallow an "image" property that does not pin the image by content digest (e.g. "image@sha256:...") |
| [`no-app-port`](https://bare-devcontainer.github.io/decolint/rules/no-app-port/) | `style` | (all) | disallow the legacy "appPort" property in favor of "forwardPorts" |
| [`unused-template-option`](https://bare-devcontainer.github.io/decolint/rules/unused-template-option/) | `style` | (all) | disallow a Template option that no file in the Template references |
<!-- /decolint:rules-table -->

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
<!-- decolint:end-page -->

## Contributing

Rules are plain Go code and new ones are easy to add; see
[CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and a
walkthrough of adding a rule.

## License

[MIT](LICENSE)
