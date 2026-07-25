---
title: undefined-template-option
category: correctness
platforms: []
file_types: [template]
description: >-
  disallow a `${templateOption:...}` reference to an option not declared in
  devcontainer-template.json
example_dir: dotnet
---

*{{ page.description }}*

## Why

Applying a Template replaces each "${templateOption:name}" with the value the user chose for the option of
that name. A reference to an option that "options" does not declare is never prompted for, and the
reference implementation substitutes the empty string for it, so a typo silently produces an empty value
in the applied files instead of an error.

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
  "image": "mcr.microsoft.com/devcontainers/dotnet:${templateOption:variant}"
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
- <https://github.com/devcontainers/cli>
