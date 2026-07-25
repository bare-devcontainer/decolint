---
title: no-seccomp-override
category: security
platforms: []
file_types: [devcontainer, feature]
description: >-
  disallow overriding the container runtime's default seccomp profile via a
  devcontainer.json's or Feature's "securityOpt" property, or a
  "--security-opt seccomp=..." entry in a devcontainer.json's "runArgs"
---

*{{ page.description }}*

## Why

The runtime's default seccomp profile blocks the syscalls containers do not need, several of which have
featured in container escapes. Pointing "seccomp" at a profile of your own replaces that default
wholesale, and a hand-written profile is rarely reviewed as carefully or updated as the kernel gains new
syscalls. Keep the default unless the workload provably needs more, and review the replacement if it
does.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "securityOpt": ["seccomp=./seccomp.json"]
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "capAdd": ["SYS_PTRACE"]
}
```

Leaving `securityOpt` unset keeps the runtime's default seccomp profile,
which already allows what a development container normally does.

## References

- <https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties>
- <https://docs.docker.com/engine/security/seccomp/>
