---
title: pin-feature-version
category: reproducibility
platforms: []
file_types: [devcontainer]
description: >-
  disallow a Feature reference without an explicit version or with the
  "latest" version
---

## Why

A Feature reference with no version resolves to "latest", so the container installs whatever the Feature's
author published most recently — the tooling it sets up can change under the project without the
devcontainer.json changing at all. Features are published under their full version as well as
"major.minor" and "major" tags, so a reference can be pinned as tightly as the project wants.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/go": {}
  }
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/go:1.3.2": {}
  }
}
```

## References

- <https://containers.dev/implementors/features-distribution/#versioning>
- <https://containers.dev/implementors/features/#referencing-a-feature>
