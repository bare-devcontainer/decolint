---
title: no-privileged-container
category: security
platforms: []
file_types: [devcontainer, feature]
description: >-
  disallow running the container in privileged mode via the "privileged"
  property or a "--privileged" entry in "runArgs"
---

## Why

A privileged container gets every Linux capability, unconfined seccomp and LSM profiles, and access to all
host devices. That removes essentially every boundary between the container and the host, so any code
running in it — including a compromised dependency pulled in by the project's own build — can take over
the machine. Docker-in-Docker is the usual reason it is set; a Feature that provides it, or the specific
capabilities and devices the workload needs, is a far narrower grant.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "privileged": true
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "capAdd": ["SYS_PTRACE"]
}
```

The good example grants only the capability a debugger needs. Reach for the
narrowest grant that works: `capAdd` for a capability, `--device` in `runArgs`
for a device, and the docker-in-docker Feature rather than privileged mode for
nested containers.

## References

- <https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties>
- <https://docs.docker.com/engine/security/#docker-daemon-attack-surface>
