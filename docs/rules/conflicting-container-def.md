---
title: conflicting-container-def
category: correctness
platforms: []
file_types: [devcontainer]
description: >-
  disallow a devcontainer.json that defines more than one of "image",
  "build", or "dockerComposeFile"
---

*{{ page.description }}*

## Why

The specification defines three mutually exclusive ways to create the container: from an image, from a
Dockerfile, or from a Docker Compose project. Which one wins when several are set is unspecified, so the
container that gets built depends on the tool rather than on the configuration. Keep the variant the
project actually uses and remove the others.

## Bad

```jsonc
{
  "name": "my project",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "build": {
    "dockerfile": "Dockerfile"
  }
}
```

## Good

```jsonc
{
  "name": "my project",
  "build": {
    "dockerfile": "Dockerfile"
  }
}
```

## References

- <https://containers.dev/implementors/spec/#orchestration-options>
- <https://containers.dev/implementors/json_reference/#scenario-specific-properties>
