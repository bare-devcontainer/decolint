---
title: id-dir-mismatch
category: correctness
platforms: []
file_types: [feature, template]
description: >-
  disallow a Feature's or Template's "id" that does not match the name of
  its containing directory
example_dir: node
---

## Why

Both specifications require the "id" to match the name of the directory holding the metadata file, since
that directory name is what packaging and distribution address the artifact by. When the two disagree the
published reference does not resolve to what the directory contains; rename the directory or the "id" so
they agree.

## Bad

```jsonc
// src/node/devcontainer-feature.json
{
  "id": "nodejs",
  "version": "1.0.0",
  "name": "Node.js"
}
```

## Good

```jsonc
// src/node/devcontainer-feature.json
{
  "id": "node",
  "version": "1.0.0",
  "name": "Node.js"
}
```

## References

- <https://containers.dev/implementors/features/#devcontainer-featurejson-properties>
- <https://containers.dev/implementors/templates/#devcontainer-templatejson-properties>
