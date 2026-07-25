---
title: invalid-semver
category: correctness
platforms: []
file_types: [feature, template]
description: >-
  disallow a Feature's or Template's "version" that is not a valid semantic
  version
---

## Why

Publishing a Feature or Template pushes it under tags derived from the "version" components: the full
version, "major.minor", and "major", so consumers can pin as loosely or as tightly as they want. A value
that is not valid semver has no such components, leaving nothing to derive those tags from.

## Bad

```jsonc
{
  "id": "node",
  "version": "1.0",
  "name": "Node.js"
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

- <https://containers.dev/implementors/features-distribution/#versioning>
- <https://semver.org/>
