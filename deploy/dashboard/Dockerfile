# syntax=docker/dockerfile:1.7

# Pins resolved from the official multi-platform image indexes on 2026-10-26.
ARG GO_IMAGE=golang:1.26-alpine3.23@sha256:0525c952defebc693cdaf62cdf43afa32be5a7f377e99260642ea19c240c708e
ARG RUNTIME_IMAGE=alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63

FROM ${GO_IMAGE} AS build
RUN apk add --no-cache build-base
WORKDIR /src
COPY . .
ARG SOURCE_REVISION
RUN test -n "${SOURCE_REVISION}" && \
    test "${#SOURCE_REVISION}" -eq 40 && \
    case "${SOURCE_REVISION}" in *[!0-9a-f]*) exit 1 ;; esac
ENV GOTOOLCHAIN=local CGO_ENABLED=1
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -mod=readonly -trimpath -buildvcs=false \
    -ldflags="-s -w -linkmode external -extldflags -static -X encr.dev/internal/version.Version=v0.0.0-develop+${SOURCE_REVISION}" \
    -o /out/encore ./cli/cmd/encore

FROM ${RUNTIME_IMAGE} AS runtime
ARG SOURCE_REVISION
RUN test -n "${SOURCE_REVISION}" && \
    test "${#SOURCE_REVISION}" -eq 40 && \
    case "${SOURCE_REVISION}" in *[!0-9a-f]*) exit 1 ;; esac
LABEL org.opencontainers.image.title="Encore read-only dashboard" \
      org.opencontainers.image.description="Development dashboard foundation; external SSO and frontend asset service required" \
      org.opencontainers.image.source="https://github.com/Seed-Leaf/encore" \
      org.opencontainers.image.revision="${SOURCE_REVISION}"
# This pinned Alpine image already includes Mozilla's CA certificate bundle.
RUN test -s /etc/ssl/certs/ca-certificates.crt && \
    addgroup -S -g 65532 encore && \
    adduser -S -D -H -u 65532 -G encore encore && \
    mkdir -p /state/config/encore /state/cache/encore /state/data /state/home && \
    chown -R 65532:65532 /state
COPY --chmod=0555 deploy/dashboard/entrypoint.sh /usr/local/bin/dashboard-entrypoint
COPY --chmod=0555 deploy/dashboard/healthcheck.sh /usr/local/bin/dashboard-healthcheck
ENV HOME=/state/home \
    XDG_CONFIG_HOME=/state/config \
    XDG_CACHE_HOME=/state/cache \
    ENCORE_CONFIG_DIR=/state/config/encore \
    ENCORE_CACHE_DIR=/state/cache/encore/cache \
    ENCORE_DATA_DIR=/state/data \
    ENCORE_DAEMON_LOG_PATH=/dev/null \
    ENCORE_DEVDASH_LISTEN_ADDR=0.0.0.0:9400 \
    ENCORE_DEVDASH_URL=https://devdash.encore.dev \
    DISABLE_ENCORE_TELEMETRY=1
WORKDIR /state
USER 65532:65532
EXPOSE 9400
# The current foreground daemon handles SIGINT, not SIGTERM.
STOPSIGNAL SIGINT
HEALTHCHECK --interval=30s --timeout=12s --start-period=60s --retries=3 \
    CMD ["/usr/local/bin/dashboard-healthcheck"]
ENTRYPOINT ["/usr/local/bin/dashboard-entrypoint"]

# Fast path for a separately verified, static Linux binary. Supply a named build
# context containing a single file called encore; never copy host credentials.
FROM runtime AS prebuilt
COPY --from=dashboard_binary --chmod=0555 /encore /usr/local/bin/encore

# Default target compiles the CLI. It intentionally does not ship application
# runtimes, patched Go, Node, a Docker client or a Docker socket.
FROM runtime AS source
COPY --from=build --chmod=0555 /out/encore /usr/local/bin/encore
