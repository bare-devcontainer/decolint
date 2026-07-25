---
title: no-cap-add-all
category: security
platforms: []
file_types: [devcontainer, feature]
description: >-
  disallow granting all Linux capabilities via an "ALL" entry in the
  "capAdd" property, or a "--cap-add=ALL" entry in a devcontainer.json's
  "runArgs"
---

*{{ page.description }}*

## Why

Linux capabilities split root's powers into units a container can be granted individually, and the runtime
withholds the dangerous ones by default. "ALL" hands them all over, including capabilities such as
"SYS_ADMIN" and "SYS_MODULE" that let a process reconfigure the host kernel and escape the container.
"capAdd" exists to name the one or two a workload actually needs, e.g. "SYS_PTRACE" for a debugger.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "capAdd": ["ALL"]
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "capAdd": ["SYS_PTRACE"]
}
```

## References

- <https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties>
- <https://docs.docker.com/engine/security/#linux-kernel-capabilities>
