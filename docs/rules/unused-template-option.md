---
title: unused-template-option
category: style
platforms: []
file_types: [template]
description: >-
  disallow a Template option that no file in the Template references
example_dir: dotnet
---

*{{ page.description }}*

## Why

An option only takes effect where a file substitutes it as "${templateOption:name}". One that nothing
references is still presented to the user when the Template is applied, so it asks a question whose
answer changes nothing — usually a leftover from a removed file or a renamed reference.

## Bad

### `devcontainer-template.json`

```jsonc
{
  "id": "dotnet",
  "version": "1.0.0",
  "name": "C# (.NET)",
  "options": {
    "imageVariant": {
      "type": "string",
      "proposals": ["8.0", "9.0"],
      "default": "9.0"
    }
  }
}
```

### `.devcontainer/devcontainer.json`

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/dotnet:9.0"
}
```

## Good

### `devcontainer-template.json`

```jsonc
{
  "id": "dotnet",
  "version": "1.0.0",
  "name": "C# (.NET)",
  "options": {
    "imageVariant": {
      "type": "string",
      "proposals": ["8.0", "9.0"],
      "default": "9.0"
    }
  }
}
```

### `.devcontainer/devcontainer.json`

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/dotnet:${templateOption:imageVariant}"
}
```

## References

- <https://containers.dev/implementors/templates/#the-options-property>
- <https://containers.dev/implementors/templates/#option-resolution>
