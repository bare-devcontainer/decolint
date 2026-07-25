---
title: require-no-new-privileges
category: security
platforms: []
file_types: [devcontainer]
description: >-
  require "no-new-privileges" to be set via a devcontainer.json's
  "securityOpt" property, or a "--security-opt no-new-privileges..." entry
  in "runArgs"
---

*{{ page.description }}*

## Why

Without this option a process in the container can still gain privileges it was not started with, by
executing a setuid binary — which undercuts the point of running as a non-root user. Setting it raises the
kernel's "no_new_privs" bit, which every child process inherits and none can clear, so the container's
privileges can only ever shrink.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu"
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "securityOpt": ["no-new-privileges"]
}
```

## References

- <https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties>
- <https://docs.kernel.org/userspace-api/no_new_privs.html>
