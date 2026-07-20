# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89
# check=error=true

FROM --platform=$BUILDPLATFORM golang:1.26.5-trixie@sha256:4ee9ffa999b4583ce281939cdff828763083610292f252279a0cee77473bd9a7 AS build

ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG REVISION=unknown

WORKDIR /src
RUN --mount=type=bind,source=go.mod,target=go.mod,readonly \
    --mount=type=bind,source=go.sum,target=go.sum,readonly \
    --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download
RUN --mount=type=bind,target=.,readonly \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    make build VERSION=$VERSION REVISION=$REVISION OUTPUT=/out/decolint

FROM scratch

COPY --from=build /out/decolint /decolint

WORKDIR /workspace
ENTRYPOINT ["/decolint"]
CMD ["."]
