# syntax=docker/dockerfile:1.7
FROM node:22-bookworm-slim AS spa-builder
WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --ignore-scripts

COPY frontend/ ./
RUN npm run build-only

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=spa-builder /src/frontend/dist cmd/maintenant/web/dist/

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG LICENSE_PUBLIC_KEY

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build \
        -ldflags="-s -w \
          -X main.version=${VERSION} \
          -X main.commit=${COMMIT} \
          -X main.buildDate=${BUILD_DATE} \
          -X main.publicKeyB64=${LICENSE_PUBLIC_KEY}" \
        -o /out/maintenant \
        ./cmd/maintenant

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata setpriv \
    && mkdir -p /data \
    && chown 65534:65534 /data

# Keep SQLite's temp files (statement journals, temp B-trees used by the one-time
# UUID conversion of large tables) on the data volume. The hardened runtime mounts
# /tmp as a tiny tmpfs, which SQLITE_FULL-fails the conversion; /data has real space.
ENV SQLITE_TMPDIR=/data

COPY --from=builder --chmod=555 /out/maintenant /app/maintenant
COPY --chmod=555 docker-entrypoint.sh /docker-entrypoint.sh

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="maintenant" \
      org.opencontainers.image.description="Monitor everything. Manage nothing." \
      org.opencontainers.image.url="https://github.com/kOlapsis/maintenant" \
      org.opencontainers.image.source="https://github.com/kOlapsis/maintenant" \
      org.opencontainers.image.vendor="kOlapsis" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

EXPOSE 8080
VOLUME /data

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

# Entrypoint runs as root only to chown root-owned volume mounts, then drops to
# uid 65534 via setpriv (see docker-entrypoint.sh); a build-time USER would break that chown.
# nosemgrep: dockerfile.security.missing-user-entrypoint.missing-user-entrypoint
ENTRYPOINT ["/docker-entrypoint.sh"]
# nosemgrep: dockerfile.security.missing-user.missing-user
CMD ["/app/maintenant"]
