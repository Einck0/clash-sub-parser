# Clash Subscription Parser

[简体中文](README.zh-CN.md) | English

Clash Subscription Parser is a single-container web application for managing Clash-compatible configuration. It provides a visual interface for subscriptions, proxy groups, rules, DNS, and generated YAML / Script.js outputs.

Runtime data is stored in the database. Private migration material, local backups, real subscription URLs, tokens, and generated full configurations should stay outside the public repository.

## Demo

This repository includes a GitHub Pages demo that builds the real Vue frontend in mock-data mode. After GitHub Pages is enabled with the "GitHub Actions" source, the demo will be available at:

```text
https://<your-github-username>.github.io/clash-sub-parser/
```

The demo uses sample data only. It does not run the FastAPI backend and does not include real subscription URLs, tokens, nodes, or generated configs.

## Features

- Subscription management with scheduled or manual fetching.
- YAML and Base64 subscription parsing.
- Node filtering with regex selection, manual include, and manual exclude.
- Proxy group management with static nodes, built-in targets, group references, expanded group nodes, and dynamic regex matching.
- Rule category and rule management with draft editing, sorting, searching, and mobile-friendly editing.
- DNS management with visual fields and raw YAML editing.
- Generated Clash YAML and Script.js outputs.
- Short subscription endpoints: `/yaml` and `/script`.
- Runtime security settings with optional token protection for the Web UI, management API, and export endpoints.
- Single-container Docker Compose deployment.

## Tech Stack

| Layer | Technology |
| --- | --- |
| Backend | Python 3.10 / FastAPI |
| ORM | SQLAlchemy Async |
| Schema | Pydantic |
| Frontend | Vue 3 + Vite |
| Database | SQLite by default, PostgreSQL optional |
| Scheduler | APScheduler |
| Deployment | Docker Compose |
| Tests | pytest |

## Quick Start

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

Default endpoints:

- Web UI and API: `http://127.0.0.1:18080`
- Health check: `http://127.0.0.1:18080/health`
- YAML output: `http://127.0.0.1:18080/yaml`
- Script output: `http://127.0.0.1:18080/script`

The default Compose ports bind to `127.0.0.1` only. Set `CLASH_PORT` / `CLASH_COMPAT_PORT` explicitly if you intentionally want to expose the service on another interface.

Common checks:

```bash
docker compose ps
curl --noproxy '*' http://127.0.0.1:18080/health
```

## Secure Deployment

If the service is reachable from a LAN or the Internet, do not expose the default unauthenticated setup directly.

Create a local `.env`:

```bash
cp .env.example .env
```

Recommended minimum settings:

```text
CLASH_AUTH_ENABLED=true
CLASH_AUTH_TOKEN=<a-long-random-token>
CLASH_PORT=127.0.0.1:18080
CLASH_COMPAT_PORT=127.0.0.1:20000
```

Generate a random token, for example:

```bash
openssl rand -base64 32
```

Then place the service behind an HTTPS reverse proxy, or keep it bound to a trusted local interface.
When serving the Web UI over HTTPS, set `CLASH_AUTH_COOKIE_SECURE=true`.

Security-related defaults:

- Subscription fetching only allows `http` and `https`.
- Subscription fetching rejects localhost, private, link-local, multicast, reserved, and unspecified networks by default.
- Redirect targets are validated before each request.
- Subscription response size is limited by `CLASH_REQUEST_MAX_BYTES`.
- Fetching private/internal subscription URLs requires explicit `CLASH_ALLOW_PRIVATE_FETCH_URLS=true`.
- Protected export URLs must carry `?token=...`, because Clash clients usually cannot send custom headers.
- Web UI login uses an HttpOnly cookie. The frontend does not persist the token in local storage.
- Cookie-authenticated management requests require an `X-Clash-CSRF: 1` header, which the bundled frontend sends automatically.

## Configuration

Important environment variables:

```text
CLASH_DATABASE_URL=sqlite+aiosqlite:////data/clash_sub_parser.db
CLASH_REQUEST_TIMEOUT_SECONDS=30
CLASH_REQUEST_MAX_BYTES=5242880
CLASH_REQUEST_USER_AGENT=ClashforWindows/0.20
CLASH_REQUEST_TRUST_ENV=false
CLASH_ALLOW_PRIVATE_FETCH_URLS=false
CLASH_DEFAULT_PROXY_TEST_URL=http://www.gstatic.com/generate_204
CLASH_SCHEDULER_ENABLED=true
CLASH_AUTH_ENABLED=false
CLASH_AUTH_TOKEN=
CLASH_AUTH_COOKIE_SECURE=false
```

For users who need China mainland build mirrors, uncomment the optional `build.args` block in `docker-compose.example.yml` before copying it to `docker-compose.yml`.

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

Proxy variables for upstream subscription fetching:

```text
CLASH_HTTP_PROXY=
CLASH_HTTPS_PROXY=
CLASH_ALL_PROXY=
CLASH_NO_PROXY=localhost,127.0.0.1
```

The compose file maps these `CLASH_*` variables to runtime proxy variables intentionally, so a host-level `HTTP_PROXY` is not accidentally baked into the deployment.

## Local Development

Backend:

```bash
cd backend
python3.10 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python -m app.main
```

Frontend:

```bash
cd frontend
npm install
npm run dev
```

The frontend dev server proxies `/api` to `http://localhost:18080` by default. Override it when needed:

```bash
VITE_API_PROXY_TARGET=http://127.0.0.1:18080 npm run dev
```

## Documentation

- [Project plan](project.md)
- [Design notes](docs/design.zh-CN.md)
- [Roadmap and release checklist](docs/roadmap.zh-CN.md)
- [Contributing guide](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## Tests and Build

Backend tests:

```bash
bash scripts/test_py310.sh
```

Frontend build:

```bash
cd frontend
npm run build
```

Docker build:

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

## API Overview

```text
GET    /api/subscriptions
POST   /api/subscriptions
PATCH  /api/subscriptions/{id}
DELETE /api/subscriptions/{id}
POST   /api/subscriptions/{id}/fetch

GET    /api/node-groups
POST   /api/node-groups
PATCH  /api/node-groups/{id}
DELETE /api/node-groups/{id}
GET    /api/node-groups/_preview
POST   /api/node-groups/validate

GET    /api/rule-categories
POST   /api/rule-categories
PATCH  /api/rule-categories/{id}
DELETE /api/rule-categories/{id}

GET    /api/rules
POST   /api/rules
PATCH  /api/rules/{id}
DELETE /api/rules/{id}

GET    /api/dns
PATCH  /api/dns

GET    /api/generate/settings
PATCH  /api/generate/settings
POST   /api/generate/yaml
POST   /api/generate/script
GET    /yaml
GET    /script
```

## Repository Hygiene

Do not commit:

- `.env` or real deployment configuration.
- SQLite/PostgreSQL dumps, backups, caches, or runtime logs.
- Real subscription URLs, access tokens, proxy credentials, or generated full Clash configurations.
- Private migration scripts, private rule material, or local notes.
- `frontend/node_modules/`, `frontend/dist/`, virtual environments, and test/build caches.

Use `.env.example` as the public configuration template.

## Acknowledgements

This project is built for the Clash-compatible ecosystem and is related to or inspired by:

- [Clash Meta / Mihomo](https://github.com/MetaCubeX/mihomo)
- [Clash Verge Rev](https://github.com/clash-verge-rev/clash-verge-rev)
- [OpenClaw](https://github.com/openclaw/openclaw)
- [OpenAI Codex](https://github.com/openai/codex)

Mentioning these projects does not imply endorsement or official affiliation.

## License

MIT
