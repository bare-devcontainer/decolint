FROM ghcr.io/bare-devcontainer/golang:1.26-trixie@sha256:37eaf48886f6e9ac3a7327228af0c960d416cfdd9000bac2a34c26206853b779

USER root

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    --mount=type=bind,source=claude-code.asc,target=/tmp/claude-code.asc,ro \
    export DEBIAN_FRONTEND=noninteractive \
      && echo "deb [signed-by=/tmp/claude-code.asc] https://downloads.claude.ai/claude-code/apt/stable stable main" \
          | tee /etc/apt/sources.list.d/claude-code.list \
      && apt-get update \
      && apt-get --no-install-recommends install -y claude-code

USER dev

RUN mkdir -p /home/dev/.claude
