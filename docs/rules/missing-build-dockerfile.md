---
title: missing-build-dockerfile
category: correctness
platforms: []
file_types: [devcontainer]
description: >-
  disallow a devcontainer.json "build" object that is missing "dockerfile"
---

*{{ page.description }}*

## Why

"build.dockerfile" is the only required member of "build": it locates, relative to the devcontainer.json,
the Dockerfile the image is built from. The other members ("context", "args", "target", ...) only shape a
build that "dockerfile" defines, so without it there is nothing to build.

## Bad

```jsonc
{
  "name": "my project",
  "build": {
    "context": ".."
  }
}
```

## Good

```jsonc
{
  "name": "my project",
  "build": {
    "dockerfile": "Dockerfile",
    "context": ".."
  }
}
```

## References

- <https://containers.dev/implementors/json_reference/#image-or-dockerfile-specific-properties>
- <https://containers.dev/implementors/spec/#dockerfile-based>
