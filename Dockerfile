FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS spa-builder
WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --ignore-scripts

COPY frontend/ ./
RUN npm run build-only

FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.6.1 AS xx

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

COPY --from=xx / /
RUN apk add --no-cache clang lld

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=spa-builder /src/frontend/dist cmd/maintenant/web/dist/

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG LICENSE_PUBLIC_KEY
ARG TARGETPLATFORM

RUN xx-apk add --no-cache gcc musl-dev
RUN xx-go --wrap
RUN CGO_ENABLED=1 go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.buildDate=${BUILD_DATE} \
      -X main.publicKeyB64=${LICENSE_PUBLIC_KEY}" \
    -o /out/maintenant \
    ./cmd/maintenant
RUN xx-verify /out/maintenant

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /data \
    && chown 65534:65534 /data

COPY --from=builder --chmod=555 /out/maintenant /app/maintenant

EXPOSE 8080
VOLUME /data

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

USER 65534:65534
ENTRYPOINT ["/app/maintenant"]
