---
title: missing-compose-service
category: correctness
platforms: []
file_types: [devcontainer]
description: >-
  disallow a devcontainer.json that sets "dockerComposeFile" without
  "service"
---

## Why

A Compose project usually defines several services, so naming the Compose file does not say which
container the tooling should attach to. The specification requires "service" to name that main container:
it is the one lifecycle scripts run in and the one editors connect to.

## Bad

```jsonc
{
  "name": "my project",
  "dockerComposeFile": "docker-compose.yml",
  "workspaceFolder": "/workspace"
}
```

## Good

```jsonc
{
  "name": "my project",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}
```

## References

- <https://containers.dev/implementors/spec/#docker-compose-based>
- <https://containers.dev/implementors/json_reference/#docker-compose-specific-properties>
