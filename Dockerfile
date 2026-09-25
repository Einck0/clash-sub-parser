# syntax=docker/dockerfile:1

# Stage 1: Build the Vue 3 + Tailwind CSS + DaisyUI web control plane
FROM node:20-alpine AS frontend-builder

WORKDIR /web

ARG NPM_CONFIG_REGISTRY=https://registry.npmmirror.com

# Cache package dependencies
COPY web/package*.json ./
RUN npm ci ${NPM_CONFIG_REGISTRY:+--registry=$NPM_CONFIG_REGISTRY}

# Copy web source and compile production static bundle into dist
COPY web/ ./
RUN npm run build

# Stage 2: Compile pure-static single Go executable with embedded assets
FROM golang:alpine AS go-builder

ENV COMPILER_BUILD_EPOCH=20260925_rules_v1

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Cache Go module dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy Go source trees and embed directories (v1.0.1 rule-capabilities-fix)
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/
COPY --from=frontend-builder /web/dist/ ./internal/webassets/dist/

# Build pure-static stripped Go binary
RUN go build -ldflags="-s -w" -trimpath -o /src/bin/csp ./cmd/csp

# Stage 3: Minimal Alpine 3.20 non-root runtime
FROM alpine:3.20 AS runtime

# Install basic CA certificates, timezone data, curl, and sqlite for operations and container health check
RUN apk add --no-cache ca-certificates tzdata curl sqlite && \
    rm -rf /var/cache/apk/*

# Create dedicated non-root application user matching standard container UID/GID (10001:10001)
RUN addgroup -g 10001 -S appuser && \
    adduser -u 10001 -S -G appuser -s /sbin/nologin -h /app appuser && \
    mkdir -p /app /data && \
    chown -R appuser:appuser /app /data && \
    chmod 750 /data

# Copy single binary from go-builder stage
COPY --from=go-builder --chown=appuser:appuser /src/bin/csp /app/csp
RUN chmod 755 /app/csp

# Environment configurations for containerized runtime
ENV CSP_ADDR=0.0.0.0:18080 \
    CSP_DB_PATH=/data/csp-v1.db \
    TZ=Asia/Shanghai

WORKDIR /app
USER appuser

EXPOSE 18080
VOLUME ["/data"]

# Container-level health check probe
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://127.0.0.1:18080/healthz || exit 1

ENTRYPOINT ["/app/csp"]
CMD ["serve"]
