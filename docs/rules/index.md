---
title: Rules
---

decolint ships 27 rules. Each belongs to one category, which sets its severity
unless a [config file](../getting-started.md#choosing-what-to-report) overrides
it; only `correctness` is enabled out of the box. A rule that names a platform
runs only when that platform is selected with `-platform`.

## correctness

The configuration is invalid or does not behave as written. These run by default, at `error`.

| Rule | Platform | Checks |
| --- | --- | --- |
| [`conflicting-container-def`](conflicting-container-def.md) | (all) | disallow a devcontainer.json that defines more than one of "image", "build", or "dockerComposeFile" |
| [`feature-install-script-not-executable`](feature-install-script-not-executable.md) | (all) | disallow a Feature's `install.sh` that lacks executable permission bits |
| [`id-dir-mismatch`](id-dir-mismatch.md) | (all) | disallow a Feature's or Template's "id" that does not match the name of its containing directory |
| [`invalid-semver`](invalid-semver.md) | (all) | disallow a Feature's or Template's "version" that is not a valid semantic version |
| [`missing-build-dockerfile`](missing-build-dockerfile.md) | (all) | disallow a devcontainer.json "build" object that is missing "dockerfile" |
| [`missing-compose-service`](missing-compose-service.md) | (all) | disallow a devcontainer.json that sets "dockerComposeFile" without "service" |
| [`missing-container-def`](missing-container-def.md) | (all) | disallow a devcontainer.json that defines none of "image", "build", or "dockerComposeFile" |
| [`missing-feature-install-script`](missing-feature-install-script.md) | (all) | disallow a Feature directory without the required `install.sh` install script |
| [`missing-required-props`](missing-required-props.md) | (all) | disallow a Feature's or Template's metadata that is missing a required property ("id", "version", or "name") |
| [`missing-workspace-mount-folder`](missing-workspace-mount-folder.md) | (all) | disallow a devcontainer.json using "image" or "build" that sets only one of "workspaceMount" or "workspaceFolder" |
| [`no-bind-mount`](no-bind-mount.md) | `codespaces` | disallow "bind" type entries in "mounts", which GitHub Codespaces silently ignores except for the Docker socket |
| [`no-host-port-format`](no-host-port-format.md) | `codespaces` | disallow "host:port" entries in "forwardPorts" and "portsAttributes", which GitHub Codespaces does not support |
| [`undefined-template-option`](undefined-template-option.md) | (all) | disallow a `${templateOption:...}` reference to an option not declared in devcontainer-template.json |

## security

Container runtime privileges and hardening.

| Rule | Platform | Checks |
| --- | --- | --- |
| [`no-cap-add-all`](no-cap-add-all.md) | (all) | disallow granting all Linux capabilities via an "ALL" entry in the "capAdd" property, or a "--cap-add=ALL" entry in a devcontainer.json's "runArgs" |
| [`no-docker-socket-mount`](no-docker-socket-mount.md) | (all) | disallow bind-mounting the host's Docker socket via a devcontainer.json's "mounts" or "runArgs", which grants the container root-equivalent control over the host |
| [`no-privileged-container`](no-privileged-container.md) | (all) | disallow running the container in privileged mode via the "privileged" property or a "--privileged" entry in "runArgs" |
| [`no-seccomp-override`](no-seccomp-override.md) | (all) | disallow overriding the container runtime's default seccomp profile via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt seccomp=..." entry in a devcontainer.json's "runArgs" |
| [`no-seccomp-unconfined`](no-seccomp-unconfined.md) | (all) | disallow disabling seccomp confinement via a devcontainer.json's or Feature's "securityOpt" property, or a "--security-opt seccomp=unconfined" entry in a devcontainer.json's "runArgs" |
| [`require-cap-drop-all`](require-cap-drop-all.md) | (all) | require a "--cap-drop=ALL" entry in a devcontainer.json's "runArgs", dropping every Linux capability |
| [`require-no-new-privileges`](require-no-new-privileges.md) | (all) | require "no-new-privileges" to be set via a devcontainer.json's "securityOpt" property, or a "--security-opt no-new-privileges..." entry in "runArgs" |
| [`require-non-root`](require-non-root.md) | (all) | require "remoteUser" or, if unset, "containerUser" to be set to a non-root user |

## reproducibility

Unpinned versions or digests that let the environment drift between builds.

| Rule | Platform | Checks |
| --- | --- | --- |
| [`no-image-latest`](no-image-latest.md) | (all) | disallow container images without an explicit tag or with the "latest" tag |
| [`pin-extension-version`](pin-extension-version.md) | `vscode`, `codespaces` | disallow a "customizations.vscode.extensions" entry without an explicit pinned version |
| [`pin-feature-version`](pin-feature-version.md) | (all) | disallow a Feature reference without an explicit version or with the "latest" version |
| [`pin-image-digest`](pin-image-digest.md) | (all) | disallow an "image" property that does not pin the image by content digest (e.g. "image@sha256:...") |

## style

Discouraged or legacy configuration that still works.

| Rule | Platform | Checks |
| --- | --- | --- |
| [`no-app-port`](no-app-port.md) | (all) | disallow the legacy "appPort" property in favor of "forwardPorts" |
| [`unused-template-option`](unused-template-option.md) | (all) | disallow a Template option that no file in the Template references |
