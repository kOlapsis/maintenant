# ── Stage 1: Build SPA ────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS spa-builder
WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --ignore-scripts

COPY frontend/ ./
RUN npm run build-only

# ── Stage 2: Build Go binary ─────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=spa-builder /src/frontend/dist cmd/pulseboard/web/dist/

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG LICENSE_PUBLIC_KEY
ARG TARGETOS TARGETARCH

RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.buildDate=${BUILD_DATE} \
      -X main.publicKeyB64=${LICENSE_PUBLIC_KEY}" \
    -o /out/pulseboard \
    ./cmd/pulseboard

# ── Stage 3: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S pulseboard && adduser -S pulseboard -G pulseboard \
    && mkdir -p /data && chown pulseboard:pulseboard /data

COPY --from=builder /out/pulseboard /app/pulseboard

USER pulseboard
EXPOSE 8080
VOLUME /data

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

ENTRYPOINT ["/app/pulseboard"]
