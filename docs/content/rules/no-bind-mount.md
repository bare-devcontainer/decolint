---
title: no-bind-mount
category: correctness
platforms: [codespaces]
file_types: [devcontainer]
description: >-
  disallow "bind" type entries in "mounts", which GitHub Codespaces silently
  ignores except for the Docker socket
---

## Why

A codespace runs on a machine in the cloud, where the host path a bind mount points at does not exist, so
Codespaces documents that it ignores "bind" mounts apart from the Docker socket. The mount is dropped
without an error and the container starts missing the data it expects. Volume mounts are honored, so use
"type=volume" for anything that only has to persist across rebuilds.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "mounts": [
    {
      "source": "${localWorkspaceFolder}/.cache",
      "target": "/home/vscode/.cache",
      "type": "bind"
    }
  ]
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "mounts": [
    {
      "source": "devcontainer-cache",
      "target": "/home/vscode/.cache",
      "type": "volume"
    }
  ]
}
```

## References

- <https://github.com/devcontainers/spec/blob/main/docs/specs/supporting-tools.md#github-codespaces>
- <https://containers.dev/implementors/spec/#mounts>
