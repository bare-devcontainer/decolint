---
title: pin-extension-version
category: reproducibility
platforms: [vscode, codespaces]
file_types: [devcontainer]
description: >-
  disallow a "customizations.vscode.extensions" entry without an explicit
  pinned version
---

*{{ page.description }}*

## Why

An extension ID on its own installs whatever the marketplace publishes at the moment the container is
created, so two developers on the same devcontainer.json can end up with different formatters, linters, or
language server versions — and an extension update can change the environment without any commit.
Appending a version ("publisher.name@1.2.3") makes the editor tooling as pinned as the rest of the image.

## Bad

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "customizations": {
    "vscode": {
      "extensions": ["golang.go"]
    }
  }
}
```

## Good

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "customizations": {
    "vscode": {
      "extensions": ["golang.go@0.50.0"]
    }
  }
}
```

## References

- <https://github.com/devcontainers/spec/blob/main/docs/specs/supporting-tools.md#visual-studio-code>
- <https://code.visualstudio.com/docs/configure/extensions/extension-marketplace>
