---
title: decolint
---

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
quieter. See [Getting started](getting-started.md).

## Why decolint

- **A `devcontainer.json` is container runtime configuration, and nobody reviews
  it that way.** `--privileged` and a bind-mounted Docker socket read like
  ordinary setup lines, and they ship to every teammate and every CI run that
  rebuilds the container.
- **It lints what actually runs, not just what you wrote.** The Features you
  reference and the base image you name contribute configuration of their own.
  With [`-merge`](getting-started.md#4-lint-what-actually-runs), decolint
  resolves them the way the real tooling does and lints the result.
- **A silently ignored property is a mistake decolint names.** GitHub Codespaces
  drops `bind` mounts and rejects `host:port` entries without reporting
  anything; the container just comes up wrong.
- **It goes where your other linters already are.** Findings carry a line and
  column, and come out as text, JSON, GitHub Actions annotations, or SARIF.
- **Every finding is explained.** Each rule has a [page here](rules/) covering
  why it exists and the configuration it accepts and rejects, and the SARIF
  output links to it, so a code scanning alert carries the reasoning with it.

## Next steps

- [Getting started](getting-started.md) — install decolint, choose what it
  reports, and wire it into CI.
- [Rules](rules/) — every rule, with the reasoning behind it and examples of the
  configuration it accepts and rejects.
- [README](https://github.com/bare-devcontainer/decolint#reference) — the full
  reference for flags, the config file, output formats and merging.
