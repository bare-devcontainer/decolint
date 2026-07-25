---
title: no-app-port
category: style
platforms: []
file_types: [devcontainer]
description: >-
  disallow the legacy "appPort" property in favor of "forwardPorts"
---

## Why

"appPort" publishes the port the way Docker does: it is fixed when the container is created, and the
application has to listen on all interfaces rather than just "localhost" to be reachable. A forwarded
port instead looks like "localhost" to the application and can be changed without recreating the
container, which is why the reference recommends "forwardPorts" in most cases.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "appPort": [3000]
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "forwardPorts": [3000]
}
```

## References

- <https://containers.dev/implementors/json_reference/#image-or-dockerfile-specific-properties>
- <https://containers.dev/implementors/json_reference/#publishing-vs-forwarding-ports>
