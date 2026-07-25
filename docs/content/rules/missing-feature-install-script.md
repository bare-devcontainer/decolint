---
title: missing-feature-install-script
category: correctness
platforms: []
file_types: [feature]
description: >-
  disallow a Feature directory without the required `install.sh` install
  script
example_verify: false
---

## Why

A Feature is distributed as its metadata file plus the "install.sh" the tooling runs inside the container,
which is where the Feature does all of its work. A directory without one publishes a Feature that
installs nothing, and the omission only surfaces when someone builds a container with it.

## Bad

```text
src/node/
└── devcontainer-feature.json
```

## Good

```text
src/node/
├── devcontainer-feature.json
└── install.sh
```

The name is fixed: the tooling runs `install.sh` and nothing else, so an
install script under any other name is never executed.

## References

- <https://containers.dev/implementors/features/#folder-structure>
- <https://containers.dev/implementors/features/#invoking-installsh>
