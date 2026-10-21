# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# check=error=true

FROM --platform=$BUILDPLATFORM golang:1.27.1-trixie@sha256:8f58fd67ea075142d947a60e0caa4317746a55118d312f027793d382c7741734 AS build

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
