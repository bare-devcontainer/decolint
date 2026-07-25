---
title: no-docker-socket-mount
category: security
platforms: []
file_types: [devcontainer]
description: >-
  disallow bind-mounting the host's Docker socket via a devcontainer.json's
  "mounts" or "runArgs", which grants the container root-equivalent control
  over the host
---

## Why

The Docker socket is the daemon's full API, and the daemon runs as root on the host. Anything that can
reach the socket can start a container that mounts the host's filesystem, so mounting it into the dev
container hands root-equivalent control of the host to every process inside — including code the
project's own build fetches. When the container genuinely needs Docker, a Docker-in-Docker Feature or a
rootless daemon keeps that access inside the container.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "mounts": [
    {
      "source": "/var/run/docker.sock",
      "target": "/var/run/docker.sock",
      "type": "bind"
    }
  ]
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/docker-in-docker:2.13.0": {}
  }
}
```

## References

- <https://containers.dev/implementors/json_reference/#general-devcontainerjson-properties>
- <https://docs.docker.com/engine/security/#docker-daemon-attack-surface>
