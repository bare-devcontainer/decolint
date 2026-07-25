---
title: missing-required-props
category: correctness
platforms: []
file_types: [feature, template]
description: >-
  disallow a Feature's or Template's metadata that is missing a required
  property ("id", "version", or "name")
---

## Why

"id", "version", and "name" are the only properties either specification requires: the "id" addresses the
artifact, the "version" is what consumers pin to, and the "name" is what a user recognizes it by in a
list. Metadata missing any of them cannot be published as a usable Feature or Template.

## Bad

```jsonc
{
  "id": "node",
  "version": "1.0.0"
}
```

## Good

```jsonc
{
  "id": "node",
  "version": "1.0.0",
  "name": "Node.js"
}
```

## References

- <https://containers.dev/implementors/features/#devcontainer-featurejson-properties>
- <https://containers.dev/implementors/templates/#devcontainer-templatejson-properties>
