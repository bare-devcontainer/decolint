---
title: decolint
---

decolint is a linter for Dev Container configuration files. It reads
`devcontainer.json`, `devcontainer-feature.json` and
`devcontainer-template.json`, checks them against the
[Dev Container specification](https://containers.dev/), and reports what is
invalid, unsafe, or unreproducible — before anyone waits for a container to
build.

```console
$ decolint .
.devcontainer/devcontainer.json:4:3: error: devcontainer.json must define only one of "image", "build", or "dockerComposeFile" (conflicting-container-def)
Found 1 error and 0 warnings.
```

## Why decolint

- **It reads the same files the tooling does.** Comments and trailing commas
  are parsed as the specification requires, and findings point at the exact
  line and column of the offending value.
- **It runs without Docker.** No image is pulled and no container is built, so
  it fits in a pre-commit hook or the first job of a pipeline. Rules that
  resolve Features against a registry are the only ones that reach the network.
- **It reports what your project cares about.** Rules are grouped into
  [categories](getting-started.md#choosing-what-to-report) — correctness,
  security, reproducibility and style — and only correctness is on by default.
- **It fits into code scanning.** The SARIF output uploads to GitHub Code
  Scanning, so findings land as annotations on the pull request that
  introduced them.

## Next steps

- [Getting started](getting-started.md) — install decolint, run it, and wire it
  into CI.
- [Rules](rules/) — every rule, with the reasoning behind it and examples of
  the configuration it accepts and rejects.
