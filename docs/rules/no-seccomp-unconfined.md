---
title: no-seccomp-unconfined
category: security
platforms: []
file_types: [devcontainer, feature]
description: >-
  disallow disabling seccomp confinement via a devcontainer.json's or
  Feature's "securityOpt" property, or a "--security-opt seccomp=unconfined"
  entry in a devcontainer.json's "runArgs"
---

*{{ page.description }}*

## Why

"seccomp=unconfined" turns off syscall filtering entirely, exposing the whole kernel API — including the
calls the default profile blocks precisely because they have been used to break out of containers. The
setting is most often copied from debugger instructions, where granting the "SYS_PTRACE" capability is
enough on current runtimes.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "securityOpt": ["seccomp=unconfined"]
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
- <https://docs.docker.com/engine/security/seccomp/>
