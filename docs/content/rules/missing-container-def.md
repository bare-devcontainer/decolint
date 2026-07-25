---
title: missing-container-def
category: correctness
platforms: []
file_types: [devcontainer]
description: >-
  disallow a devcontainer.json that defines none of "image", "build", or
  "dockerComposeFile"
---

## Why

Every dev container is created from exactly one of "image", "build", or "dockerComposeFile", and each of
the three is required in its own scenario. A configuration that sets none of them describes no container
at all, so no tool can create one from it.

## Bad

```jsonc
{
  "name": "my project",
  "forwardPorts": [3000]
}
```

## Good

```jsonc
{
  "name": "my project",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "forwardPorts": [3000]
}
```

## References

- <https://containers.dev/implementors/spec/#orchestration-options>
- <https://containers.dev/implementors/json_reference/#scenario-specific-properties>
