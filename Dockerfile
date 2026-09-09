# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS web-builder

RUN corepack enable && corepack prepare pnpm@10.33.0 --activate

WORKDIR /build/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --store-dir /pnpm/store

COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS server-builder

ARG TARGETOS
ARG TARGETARCH

ENV CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
COPY --from=web-builder /build/web/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/miyabi ./cmd/miyabi

FROM debian:bookworm-slim AS runtime

ARG VERSION=dev
ARG REVISION=unknown

LABEL org.opencontainers.image.title="Miyabi" \
      org.opencontainers.image.description="JavDB media library with 115 cloud playback" \
      org.opencontainers.image.source="https://github.com/ppxb/miyabi" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 miyabi \
    && useradd --uid 10001 --gid miyabi --create-home --home-dir /app miyabi \
    && mkdir -p /app/data \
    && chown miyabi:miyabi /app/data

WORKDIR /app
COPY --from=server-builder --chown=miyabi:miyabi /out/miyabi /app/miyabi
COPY LICENSE /app/LICENSE

USER miyabi
ENV MIYABI_LISTEN=":8080" MIYABI_DATA_DIR="/app/data" MIYABI_LOG_LEVEL="info"

VOLUME ["/app/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl --fail --silent http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/app/miyabi"]
