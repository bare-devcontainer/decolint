---
title: no-image-latest
category: reproducibility
platforms: []
file_types: [devcontainer]
description: >-
  disallow container images without an explicit tag or with the "latest" tag
---

*{{ page.description }}*

## Why

A reference with no tag resolves to "latest", and "latest" is just the tag a publisher moves as they
release. Either way the configuration says "whatever is current", so the same devcontainer.json builds a
different environment next month, and a build that broke cannot be reproduced from the file alone. Name
the version the project was tested against.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:latest"
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04"
}
```

## References

- <https://containers.dev/implementors/json_reference/#image-or-dockerfile-specific-properties>
