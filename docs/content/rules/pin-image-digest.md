---
title: pin-image-digest
category: reproducibility
platforms: []
file_types: [devcontainer]
description: >-
  disallow an "image" property that does not pin the image by content digest
  (e.g. "image@sha256:...")
---

## Why

A tag is a mutable pointer: the publisher can move even a fully specified one to different bits, and a
registry can serve a different image for the same tag on a different day. A digest names the content
itself, so "image@sha256:..." always resolves to the exact image the project was tested with, and the
client verifies what it pulled against it.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04"
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04@sha256:2a1d1e1a4b0c3f8e5c8a1e0a6d3b7c9f4e2d1a0b9c8d7e6f5a4b3c2d1e0f9a8b"
}
```

## References

- <https://containers.dev/implementors/json_reference/#image-or-dockerfile-specific-properties>
- <https://github.com/opencontainers/image-spec/blob/main/descriptor.md#digests>
