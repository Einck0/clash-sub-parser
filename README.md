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

Standard deployment uses the prebuilt multi-platform container image from GitHub Container Registry (GHCR):

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose pull app && docker compose up -d --no-build
```

> **Pull & Launch Invariants**:
> - Always use `docker compose pull app && docker compose up -d --no-build` (chained with `&&`) so that if remote image pulling fails, execution terminates immediately instead of silently falling back to a stale or missing local cache.
> - The `--no-build` flag guarantees that the host will never attempt local compilation on deployment servers.
> - **Image Channels vs. Pinned Digests**: By default, `docker-compose.example.yml` tracks the rolling pre-release channel `ghcr.io/einck0/clash-sub-parser:prelease`. Because channel tags are mutable pointers updated on every verified prelease delivery, production deployments are strongly recommended to pin an exact immutable digest (e.g. `CSP_IMAGE=ghcr.io/einck0/clash-sub-parser@sha256:<64hex-digest>`).
>
> **Controlled Ops Deployment**:
> For production environments requiring strict cryptographic verification prior to container execution:
> 1. Run `scripts/cloud-delivery/preflight-pull.sh --image <package@sha256:...> --expected-revision <git-sha>` to pull the image and assert exact OCI revision and RepoDigests.
> 2. Start the service with the pinned digest and `--pull never`:
>    ```bash
>    CSP_IMAGE="ghcr.io/einck0/clash-sub-parser@sha256:<digest>" docker compose -f docker-compose.prod.yml up -d --no-build --pull never
>    ```
>    Here `--pull never` explicitly operates on the cryptographically verified local image from step 1, preventing runtime drift or external pull failures.
>
> **Local Source Compilation**:
> If developing locally with source builds, use `docker-compose.dev.yml` (`docker compose -f docker-compose.dev.yml up -d --build`). In accordance with CSP cloud delivery rules, local compilation is strictly prohibited on production and CI server hosts.

Default endpoints:
- **Web UI & Management API**: `http://127.0.0.1:18080` (API under `/api/v1/*`)
- **Liveness probe**: `http://127.0.0.1:18080/healthz`
- **Readiness probe**: `http://127.0.0.1:18080/readyz`
- **Legacy endpoints**: `GET /yaml` and `GET /script` return `HTTP 410 Gone` (directing clients to client subscription links).

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
   - Client subscription URLs require their own independent export token (`?token=<export-token>`), not `CSP_ADMIN_TOKEN`.

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

## Subscription update routing

Settings → Subscription Update Route controls the **next** fetch without restarting. `environment` (migration default) inherits `--fetch-proxy` / `CSP_FETCH_PROXY`; `direct` ignores that fallback; `proxy` uses the persisted HTTP/HTTPS endpoint. An explicit per-subscription override takes precedence, but private overrides must match the currently trusted global relay. Subscription targets and every redirect still require local public-IP validation. Direct fetches pin the validated IP; HTTP/HTTPS proxy fetches preserve the original target domain (including CONNECT authority), so the trusted proxy resolves it remotely. Local validation cannot guarantee which IP the remote resolver will use.

Management-authenticated `GET/PUT /api/v1/settings/subscription-fetch` uses the existing cookie CSRF protection. Example PUT: `{"mode":"proxy","proxy_url":"http://user:password@proxy.example:7890"}`. HTTP/HTTPS and an explicit port are required. Omitting `proxy_url` retains the saved URL **and credentials**; `{"mode":"direct","clear_proxy_url":true}` clears them. `{"mode":"environment"}` restores inheritance without erasing the saved proxy. Invalid requests change nothing, including auth settings. GET returns only credential-free configured/effective endpoints, credential-presence flags, and effective source/mode; never resend a masked URL. Proxy credentials are stored in the existing SQLite settings record (not a vault), so protect database and backup access.

Fetch history retains safe stage/reason codes for DNS, connection, TLS, HTTP, body and parse failures. Failure/partial refresh preserves old source associations; complete success prunes only that subscription’s associations and retains node assets/history. A configured proxy cannot repair an unavailable upstream.

Subscription `prefix` is a top-level field, not part of advanced `config`. Saving a prefix atomically updates existing node labels. Shared nodes combine unique nonempty prefixes in stable source-ID order using the first source's latest raw name; clearing removes only that source label. Explicit user display names take precedence. Prefixes do not change raw entries, logical IDs, connection versions, or existing publications. Manual refresh is allowed even for disabled subscriptions (only scheduled refresh is disabled).

Current inventory includes active manual nodes without subscription-origin evidence, or explicitly manual-confirmed recovery. A refresh-pruned subscription asset remains in the ledger, but does not become a manual node merely because its source edge was removed.

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

---

## Core Workbenches & Features

- **Unified Nodes Workbench (`/nodes`)**: Centralized inventory and probe operations. Replaces the legacy separate probe engine view (`/probes` permanently redirects to `/nodes`). Supports desktop compact table and mobile card views, multi-page explicit selection, selective manual probing, full fleet probe confirmations, and non-blocking drawer run inspection and cancellation.
- **Global Probe Settings (`/settings`)**: Unified management for automated probe scheduling toggle, default interval (60–604800s), active probe kinds (`baseline`, `ai`, `streaming`), and per-node probe timeout (1–30s). Disabling automated scheduling leaves manual probing fully functional.
- **Per-Subscription Probe Policy (`/subscriptions`)**: Each subscription source independently configures its probe policy (`inherit`, `custom` interval, or `disabled`). Shared nodes across multiple sources dynamically evaluate the shortest valid interval under global coordination.
- **Structured Policy Filter Pipeline (`/policy`)**: Deterministic policy group member composition organized as Candidate Scope (global filter inheritance toggle) &rarr; Ordered Add Sources &rarr; Ordered Remove Filters &rarr; Live Verification. Live preview accurately reports evaluated candidates, active node count, and structured exclusion reasons (`regex removed`, `filter removed`).

---

## Agent Operations & Automated Skills (skills/csp-ops)

To enable external autonomous agents (e.g. Hermes Agent, Pi multi-agent clusters) and automation scripts to interact with, manage, and configure CSP 1.0 **without modifying source code or rebuilding containers**, CSP provides a dedicated agent skill package in [`skills/csp-ops/`](skills/csp-ops/SKILL.md).

### Built-in Agent CLI (`csp_ctl.py`)
Located at `skills/csp-ops/scripts/csp_ctl.py`, written in pure Python 3 standard library (zero external dependencies), ready for any container or host environment:

```bash
# 1. Cluster Status & Diagnostics
python3 skills/csp-ops/scripts/csp_ctl.py status

# 2. Routing Rules Management (fuzzy category & group resolution)
python3 skills/csp-ops/scripts/csp_ctl.py rule list --limit 100
python3 skills/csp-ops/scripts/csp_ctl.py rule list --search "openai"
python3 skills/csp-ops/scripts/csp_ctl.py rule add --expression "DOMAIN-SUFFIX,anthropic.com" --target "AI" --category "AI"
python3 skills/csp-ops/scripts/csp_ctl.py rule delete --id "<rule-uuid>"

# 3. Policy Group Pipeline & Empty Node Fallback Controls
python3 skills/csp-ops/scripts/csp_ctl.py group list
python3 skills/csp-ops/scripts/csp_ctl.py group get "选择节点"
python3 skills/csp-ops/scripts/csp_ctl.py group add-source "选择节点" --kind filter --filter-display-not-regex "自动|故障|官网|套餐|机场|订阅"
python3 skills/csp-ops/scripts/csp_ctl.py group set-empty-pass "加拿大" --enable

# 4. Subscriptions Management & Synchronous Refresh
python3 skills/csp-ops/scripts/csp_ctl.py sub list
python3 skills/csp-ops/scripts/csp_ctl.py sub refresh --all
```

For complete SDK usage, optimistic concurrency locking details, and REST API contracts, see [skills/csp-ops/SKILL.md](skills/csp-ops/SKILL.md).
