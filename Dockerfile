FROM node:20-alpine AS frontend-build

WORKDIR /frontend
ARG NPM_CONFIG_REGISTRY
COPY frontend/package*.json ./
RUN npm ci ${NPM_CONFIG_REGISTRY:+--registry=$NPM_CONFIG_REGISTRY}
COPY frontend/ ./
RUN npm run build

FROM python:3.10-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*

WORKDIR /app
ARG PIP_INDEX_URL
ARG PIP_TRUSTED_HOST
COPY backend/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir ${PIP_INDEX_URL:+-i $PIP_INDEX_URL} ${PIP_TRUSTED_HOST:+--trusted-host $PIP_TRUSTED_HOST} -r requirements.txt
COPY backend/ ./
COPY --from=frontend-build /frontend/dist ./frontend_dist
RUN adduser --system --group --home /app appuser \
    && mkdir -p /data \
    && chown -R appuser:appuser /app /data
USER appuser

EXPOSE 18080
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "18080"]
