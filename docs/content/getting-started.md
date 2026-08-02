---
title: Getting started
description: 'Install decolint, choose what it reports, and wire it into CI.'
toc: true
---

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
[Verifying release artifacts](reference.md#verifying-release-artifacts).

### Container image

```console
docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint [directory ...]
```

Images are published for `linux/amd64` and `linux/arm64` and tagged `latest`,
`<major>`, `<major>.<minor>`, and `<major>.<minor>.<patch>`. They carry the
same build provenance attestation as the binaries; see
[Verifying release artifacts](reference.md#verifying-release-artifacts).

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
[What decolint lints](reference.md#what-decolint-lints).

Run it on the same file the demo above found nine problems in, and it comes
back clean:

```console
$ decolint
Config: none (defaults; run "decolint --init" to create .decolint.jsonc)
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
category. Run `decolint --rules` to see every rule with the severity your
config gives it, or `decolint --init` to generate a config that lists all of
them explicitly. The full list is under [Rules](reference.md#rules), and everything the
file accepts is under [Config file](reference.md#config-file).

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

$ decolint --merge .
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

Turn it on for good with `"merge": true`, or pass `--merge` per run. It fetches
every referenced Feature and resolves the base image, so it needs network
access; see [Merging](reference.md#merging) for what gets resolved and what does not.

### 5. Fix it, or say why not

When a finding is deliberate, suppress it in the configuration file itself so
the reason lives next to the line:

```jsonc
{
  // decolint-ignore-next-line no-image-latest
  "image": "ubuntu:latest"
}
```

See [Suppressing findings](reference.md#suppressing-findings) for file- and line-scoped
directives.

## Add it to CI

decolint exits `1` when it reports an `error`, which is all a CI job needs to
fail. Add `--format=github` and the findings also appear as annotations on the
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
      - run: docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint --format=github .
```

Only findings become annotations; the files that were linted go to a collapsed
group in the run log, so a clean file does not annotate the diff. Warnings do
not fail the build on their own; add `--deny-warnings` to make them count. See
[Exit codes](reference.md#exit-codes).

To have findings tracked as alerts in the repository's Security tab instead,
emit [SARIF](reference.md#output-formats) and upload it. The upload writes to code
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
      - run: docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint --format=sarif . > decolint.sarif
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
Config: none (defaults; run "decolint --init" to create .decolint.jsonc)
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
