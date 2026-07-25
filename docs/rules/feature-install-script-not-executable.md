---
title: feature-install-script-not-executable
category: correctness
platforms: []
file_types: [feature]
description: >-
  disallow a Feature's `install.sh` that lacks executable permission bits
example_verify: false
---

*{{ page.description }}*

## Why

The specification has the installing tool invoke "install.sh" directly rather than through a shell, so
that the script's own shebang selects the interpreter. That requires the execute bit: without it the
Feature fails to install when a container is built. Run "chmod +x install.sh" and commit the mode change.

## Bad

```console
$ ls -l install.sh
-rw-r--r-- 1 user user 214 Jan 1 00:00 install.sh
```

## Good

```console
$ chmod +x install.sh
$ ls -l install.sh
-rwxr-xr-x 1 user user 214 Jan 1 00:00 install.sh
```

Git records the executable bit, so committing the mode change is what makes
the fix stick. On Windows, where the filesystem has no executable bit, set it
in the index directly: `git update-index --chmod=+x install.sh`.

## References

- <https://containers.dev/implementors/features/#invoking-installsh>
