---
title: no-host-port-format
category: correctness
platforms: [codespaces]
file_types: [devcontainer]
description: >-
  disallow "host:port" entries in "forwardPorts" and "portsAttributes",
  which GitHub Codespaces does not support
---

## Why

The "host:port" form forwards a port from another container in a Docker Compose project (e.g. "db:5432")
rather than from the primary one. Codespaces documents that it does not support that variation of either
property, so the entry is ignored there and the port is not forwarded. A bare port number, which refers
to the primary container, works everywhere.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "forwardPorts": ["db:5432"]
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "forwardPorts": [5432]
}
```

## References

- <https://github.com/devcontainers/spec/blob/main/docs/specs/supporting-tools.md#github-codespaces>
- <https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties>
