---
title: require-cap-drop-all
category: security
platforms: []
file_types: [devcontainer]
description: >-
  require a "--cap-drop=ALL" entry in a devcontainer.json's "runArgs",
  dropping every Linux capability
---

## Why

Container runtimes grant a default set of capabilities that a dev container almost never uses: raw network
access, changing file ownership, or binding privileged ports. Dropping all of them and adding back only
what the workload needs ("capAdd") means a process that is compromised inherits no privilege the project
never asked for.

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
  "runArgs": ["--cap-drop=ALL"],
  "capAdd": ["CHOWN", "SETUID", "SETGID"]
}
```

`runArgs` is the only place this can be expressed: devcontainer.json has a
`capAdd` property but no `capDrop` one, so dropping capabilities means passing
the flag to the container runtime. Add back through `capAdd` whatever the
workload actually needs.

## References

- <https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties>
- <https://docs.docker.com/engine/security/#linux-kernel-capabilities>
