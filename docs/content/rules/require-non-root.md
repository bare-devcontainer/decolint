---
title: require-non-root
category: security
platforms: []
file_types: [devcontainer]
description: >-
  require "remoteUser" or, if unset, "containerUser" to be set to a non-root
  user
---

## Why

"remoteUser" defaults to whatever user the container runs as, which for most images is root. Everything
the developer's session drives then runs as root: lifecycle scripts, terminals, and the language servers
and build tools the editor starts, so a compromised dependency runs with full control of the container.
Naming an unprivileged user — as the specification's own images do — costs nothing and contains it.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "remoteUser": "root"
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "remoteUser": "vscode"
}
```

`remoteUser` is what lifecycle scripts and the editor's remote session run as,
and it wins over `containerUser`; a rule that finds neither reports the
container's default, which is `root` for most images.

## References

- <https://containers.dev/implementors/json_reference/#remoteuser>
- <https://containers.dev/implementors/spec/#users>
