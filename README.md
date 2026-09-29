# Clash Subscription Parser (CSP 1.0)

[简体中文](README.zh-CN.md) | English

Clash Subscription Parser (CSP 1.0) is a single-container, single-binary control plane and configuration compiler for managing proxy subscriptions, node ledgers, probe health evidence chains, policy trees, and modern client publications (Mihomo / Clash Meta, Sing-box, Quantumult X, and Surge).

Static assets are embedded directly into the Go executable, providing a self-contained, zero-dependency deployment with SQLite WAL storage.

---

## Architecture & Tech Stack

| Layer | Technology |
| --- | --- |
| Control Plane & Compiler | Go 1.27.1 / Chi v5 / Sing-box Core |
| Web UI | Vue 3 + TypeScript + TailwindCSS + DaisyUI (embedded in Go binary via `embed`) |
| Storage & Schema | SQLite with WAL mode & embedded schema migrations |
| Testing & Verification | Go testing framework, Vitest, Playwright E2E |
| Deployment | Single-container Docker Compose or standalone Go binary |

---

## Quick Start

### Docker Compose

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

Default endpoints:
- **Web UI & Management API**: `http://127.0.0.1:18080` (API under `/api/v1/*`)
- **Liveness probe**: `http://127.0.0.1:18080/healthz`
- **Readiness probe**: `http://127.0.0.1:18080/readyz`
- **Legacy endpoints**: `GET /yaml` and `GET /script` return `HTTP 410 Gone` (directing clients to versioned publications).

Verify running status:
```bash
curl -f http://127.0.0.1:18080/healthz
curl -f http://127.0.0.1:18080/readyz
```

---

## Port Allocation & Environment Invariant

- **Port 18080**: Standard default service port.
- **Port 17000**: **Production mapped host alias contract**. Port 17000 on the host machine is mapped to production instances. All automated tests, Playwright E2E suites, and development previews strictly use dynamic ephemeral ports (`0`) and isolated temporary databases, **never probing or mutating 17000 or 18080**.

---

## Authentication & Secure Deployment

CSP supports two operational authentication modes:

1. **Open Mode (`CSP_ADMIN_TOKEN` is unset/blank)**:
   - Zero-configuration mode for trusted local network or standalone development.
   - All management APIs and the Web UI are directly accessible.

2. **Protected Mode (`CSP_ADMIN_TOKEN` is set)**:
   - Required for any deployment reachable over a LAN or the public Internet.
   - Web UI presents an interactive `AuthGate` requiring the Admin Token.
   - API endpoints require `Authorization: Bearer <token>` or authenticated session cookie with `X-CSRF-Token` protection.
   - Published subscription URLs require their own per-publication export token (`?token=<export-token>`), not `CSP_ADMIN_TOKEN`.

### Configuring Credentials

Copy `.env.example` to `.env`:
```bash
cp .env.example .env
```

Generate a high-entropy secret token:
```bash
openssl rand -hex 32
```
Set `CSP_ADMIN_TOKEN=<your-token>` in `.env`.

Always keep the service bound to `127.0.0.1:18080` or place it behind a TLS reverse proxy (Caddy / Nginx).

---

## Configuration Reference

Variables supported in `.env` and `docker-compose.yml`:

| Variable | Default | Description |
| --- | --- | --- |
| `CSP_CONTAINER_NAME` | `clash-sub-parser` | Container name in Docker Compose |
| `CSP_PORT` | `127.0.0.1:18080` | Host port mapping for Docker Compose |
| `CSP_ADDR` | `0.0.0.0:18080` | Listen host and port for CSP server |
| `CSP_DB_PATH` | `/data/csp-v1.db` | SQLite database file path |
| `CSP_ADMIN_TOKEN` | *(empty)* | Admin authentication token (enables Protected Mode) |
| `CSP_FETCH_PROXY` | `http://host.docker.internal:7890` | Outbound proxy for subscription fetching |
| `CSP_VOLUME_NAME` | `csp-v1-data` | Dedicated persistent Docker volume name |
| `TZ` | `Asia/Shanghai` | Container timezone |

---

## Local Development & Testing

### Standalone Go Server
```bash
# Compile and run
go build -o csp ./cmd/csp
./csp serve -addr 127.0.0.1:18080 -db ./data/csp-v1.db
```

### Frontend Development
```bash
cd web
npm install
npm run dev
```

### Running Test Suites

- **Go unit and integration tests**:
  ```bash
  go test ./...
  ```

- **Frontend unit and component tests**:
  ```bash
  cd web && npm test
  ```

- **Frontend TypeScript type check**:
  ```bash
  cd web && npm run type-check
  ```

- **Playwright real-server end-to-end tests**:
  ```bash
  cd web && npx playwright test -c e2e/playwright.config.ts
  ```

- **Operations and smoke test harness**:
  ```bash
  bash scripts/smoke_test.sh
  ```
