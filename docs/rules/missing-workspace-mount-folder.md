---
title: missing-workspace-mount-folder
category: correctness
platforms: []
file_types: [devcontainer]
description: >-
  disallow a devcontainer.json using "image" or "build" that sets only one
  of "workspaceMount" or "workspaceFolder"
---

*{{ page.description }}*

## Why

The two properties describe opposite ends of the same override: "workspaceMount" says where the source
code is mounted, "workspaceFolder" says which path inside the container the tooling opens. The reference
documents each as requiring the other, because setting one alone either mounts the source somewhere
nothing opens, or opens a path nothing is mounted at.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "workspaceMount": "source=${localWorkspaceFolder},target=/srv/app,type=bind"
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "workspaceMount": "source=${localWorkspaceFolder},target=/srv/app,type=bind",
  "workspaceFolder": "/srv/app"
}
```

## References

- <https://containers.dev/implementors/json_reference/#image-or-dockerfile-specific-properties>
- <https://containers.dev/implementors/spec/#workspacefolder-and-workspacemount>
