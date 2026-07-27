---
title: golden-rule
category: security
platforms: [codespaces]
file_types: [feature]
description: 'disallow "privileged"'
---

## Why

Why it matters.

## Bad

### `devcontainer-feature.json`

```jsonc
{
  "id": "node"
}
```

### `install.sh` (mode 0644)

```bash
#!/bin/sh
```

## Good

```jsonc
{}
```

A closing note.

## References

- <https://example.invalid/a>
- <https://example.invalid/b>
