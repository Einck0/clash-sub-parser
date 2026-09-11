# syntax=docker/dockerfile:1

# Stage 1: Build the modernized Vue 3 + Tailwind CSS + shadcn-vue frontend SPA
FROM node:20-alpine AS frontend-builder

WORKDIR /frontend

ARG NPM_CONFIG_REGISTRY=https://registry.npmmirror.com

# Cache package dependencies
COPY frontend/package*.json ./
RUN npm ci ${NPM_CONFIG_REGISTRY:+--registry=$NPM_CONFIG_REGISTRY}

# Copy frontend source code and compile static bundle into /frontend/dist
COPY frontend/ ./
RUN npm run build

# Stage 2: Compile pure-static single Go executable with embedded frontend assets
FROM golang:1.27-alpine AS go-builder

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Cache Go module dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy Go source trees and embed package files
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY frontend/embed.go frontend/embed_test.go ./frontend/
COPY --from=frontend-builder /frontend/dist ./frontend/dist

# Build pure-static stripped Go binary
RUN go build -ldflags="-s -w" -trimpath -o /src/bin/clash-sub-parser ./cmd/server

# Stage 3: Production minimal Alpine 3.20 static runtime
FROM alpine:3.20 AS runtime

# Install basic CA certificates, timezone data, and curl for container health check
RUN apk add --no-cache ca-certificates tzdata curl && \
    rm -rf /var/cache/apk/*

# Create dedicated non-root application user matching standard container UID/GID (100:101)
RUN addgroup -g 101 -S appuser && \
    adduser -u 100 -S -G appuser -s /sbin/nologin -h /app appuser && \
    mkdir -p /app /data && \
    chown -R appuser:appuser /app /data && \
    chmod 750 /data

# Copy single binary from go-builder stage
COPY --from=go-builder --chown=appuser:appuser /src/bin/clash-sub-parser /app/clash-sub-parser
RUN chmod 755 /app/clash-sub-parser

# Environment configurations for containerized runtime
ENV CSP_PORT=18080 \
    CSP_BIND=0.0.0.0 \
    CSP_DB_PATH=/data/clash_sub_parser.db \
    TZ=Asia/Shanghai

WORKDIR /app
USER appuser

EXPOSE 18080
VOLUME ["/data"]

# Container-level health check probe
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:18080/health || exit 1

ENTRYPOINT ["/app/clash-sub-parser"]
