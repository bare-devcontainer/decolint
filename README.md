# decolint

[![CI](https://github.com/bare-devcontainer/decolint/actions/workflows/ci.yml/badge.svg)](https://github.com/bare-devcontainer/decolint/actions/workflows/ci.yml)
[![Attestation Checks](https://github.com/bare-devcontainer/decolint/actions/workflows/attest-check.yml/badge.svg)](https://github.com/bare-devcontainer/decolint/actions/workflows/attest-check.yml)

📖 **[Documentation](https://bare-devcontainer.github.io/decolint/)** — getting
started, every rule, and the full reference.

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

That run has every category and both target platforms enabled; the defaults are
quieter. See [Set up your
project](https://bare-devcontainer.github.io/decolint/getting-started/#set-up-your-project).

## Why decolint

- **A `devcontainer.json` is container runtime configuration, and nobody
  reviews it that way.** `--privileged` and a bind-mounted Docker socket read
  like ordinary setup lines, and they ship to every teammate and every CI run
  that rebuilds the container.
- **It lints what actually runs, not just what you wrote.** Features and the
  base image contribute configuration of their own, and decolint resolves them
  the way the real tooling does.
- **A silently ignored property is a mistake decolint names.** GitHub
  Codespaces drops `bind` mounts and rejects `host:port` entries without
  reporting anything; the container just comes up wrong.
- **It goes where your other linters already are.** Findings carry a line and
  column, and come out as text, JSON, GitHub Actions annotations, or SARIF.
- **One static binary.** No Node.js, no Docker daemon, no project
  dependencies.

## Lint what actually runs

Your `devcontainer.json` is only part of the configuration. The Features it
references and the base image it names contribute their own, and the tooling
merges all of it before the container starts. Take a file that is careful about
everything visible in it — pinned by digest, capabilities dropped, no new
privileges:

```jsonc
// .devcontainer/devcontainer.json
{
  "name": "api",
  "image": "mcr.microsoft.com/devcontainers/go:1.24@sha256:8de3d5b3a3ce235671c7649f0b910414158a220d18cbd2714a4446cc0cc6acd3",
  "runArgs": ["--cap-drop=ALL"],
  "securityOpt": ["no-new-privileges"]
}
```

Linted as written it reports one problem, and that one is wrong: the base image
sets a non-root user, so `require-non-root` never applied. Merging replaces it
with four that are real:

```console
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

None of those is in the file above: the base image disables seccomp and
installs two unpinned VS Code extensions. Each is reported at the property that
pulled it in. Turn it on with `-merge`, or `"merge": true` in your config; see
[Lint what actually
runs](https://bare-devcontainer.github.io/decolint/getting-started/#4-lint-what-actually-runs)
for what gets resolved and what does not.
<!-- decolint:end-page -->

## Try it

Run it against your own repository, without installing anything:

```console
docker run --rm -v "$PWD:/workspace" ghcr.io/bare-devcontainer/decolint
```

## Install

Download a signed binary from the
[releases page](https://github.com/bare-devcontainer/decolint/releases), or
build from source:

```console
GOEXPERIMENT=jsonv2 go install github.com/bare-devcontainer/decolint/cmd/decolint@latest
```

`GOEXPERIMENT=jsonv2` is required because decolint uses the still experimental
`encoding/json/v2` standard library package. The container image is published for
`linux/amd64` and `linux/arm64`. See [Getting
started](https://bare-devcontainer.github.io/decolint/getting-started/#install)
for the image tags available and how to verify a release artifact.

## What it checks

Every rule belongs to one category, which sets its severity. Only `correctness`
runs without configuration; the rest are `off` until you enable them:

<!-- decolint:categories -->
| Category | Default | Rules |
| --- | --- | --- |
| [`correctness`](https://bare-devcontainer.github.io/decolint/rules/#correctness) | `error` | 13 |
| [`security`](https://bare-devcontainer.github.io/decolint/rules/#security) | `off` | 8 |
| [`reproducibility`](https://bare-devcontainer.github.io/decolint/rules/#reproducibility) | `off` | 4 |
| [`style`](https://bare-devcontainer.github.io/decolint/rules/#style) | `off` | 2 |
<!-- /decolint:categories -->

## Documentation

- [Getting started](https://bare-devcontainer.github.io/decolint/getting-started/)
  — install decolint, turn on the checks you want, name the platforms you
  target, lint the configuration your Features and base image actually produce,
  and wire it into CI as annotations or SARIF.
- [Rules](https://bare-devcontainer.github.io/decolint/rules/) — every rule, with
  why it exists, the configuration it accepts and rejects, and the part of the
  Dev Container specification it comes from.
- [Reference](https://bare-devcontainer.github.io/decolint/reference/) — every
  flag, config file member, output format, and exit code.

## Contributing

Rules are plain Go code and new ones are easy to add; see
[CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and a
walkthrough of adding a rule.

## License

[MIT](LICENSE)
