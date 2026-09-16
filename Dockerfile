# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
# check=error=true

FROM --platform=$BUILDPLATFORM golang:1.27.1-trixie@sha256:9baa6b4187bbb98d240372a8a235ac0bb6b5ddd52bba1431dc2f7c0705862728 AS build

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
