FROM node:20-alpine@sha256:fb4cd12c85ee03686f6af5362a0b0d56d50c58a04632e6c0fb8363f609372293 AS frontend-build

WORKDIR /frontend
ARG NPM_CONFIG_REGISTRY
COPY frontend/package*.json ./
RUN HTTP_PROXY= HTTPS_PROXY= ALL_PROXY= http_proxy= https_proxy= all_proxy= npm ci ${NPM_CONFIG_REGISTRY:+--registry=$NPM_CONFIG_REGISTRY}
COPY frontend/ ./
RUN npm run build

FROM ghcr.io/sagernet/sing-box:latest AS singbox-bin

FROM python:3.10-slim@sha256:c1e4e6c01eb489c422288b2de34b0761ca316f7a2d98e2c33f47659a73ed108a AS runtime

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# 复制 sing-box 静态二进制用于节点握手与测速
COPY --from=singbox-bin /usr/local/bin/sing-box /usr/local/bin/sing-box
RUN chmod +x /usr/local/bin/sing-box

WORKDIR /app
ARG PIP_INDEX_URL
ARG PIP_TRUSTED_HOST
COPY backend/requirements-runtime.txt ./requirements-runtime.txt
COPY backend/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir ${PIP_INDEX_URL:+-i $PIP_INDEX_URL} ${PIP_TRUSTED_HOST:+--trusted-host $PIP_TRUSTED_HOST} -r requirements.txt
COPY backend/ ./
COPY --from=frontend-build /frontend/dist ./frontend_dist
RUN adduser --system --group --home /app appuser \
    && mkdir -p /data \
    && chown -R appuser:appuser /app /data
USER appuser

EXPOSE 18080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl --fail --silent http://127.0.0.1:18080/ready || exit 1
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "18080"]
