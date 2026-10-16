# Implementation Plan: Delivery Architecture & Container Governance

**Branch**: `008-delivery-architecture` | **Feature Spec**: `specs/008-delivery-architecture/spec.md`

## 1. Open-Source Benchmark Analysis (Prior Art First)

To address the user's requirement ("重新梳理github ，编译，容器，发布相关逻辑，看看docker容器怎么设计，本地直接编译还是远程拉分支，这些所有都对比开源规范"), we benchmarked mature Go + Web full-stack open-source projects:

| Benchmark Project | Dockerfile Design | CI/CD Build Flow | Image Naming & Visibility | Host Deployment Model |
| :--- | :--- | :--- | :--- | :--- |
| **Gitea** (Go + Vue/Templates) | Multi-stage `Dockerfile` (Node builder -> Go builder -> Alpine/Rootless runtime with dedicated UID/GID). | GitHub Actions compiles binaries, creates GitHub Releases, and builds multi-arch container images pushed to GHCR / Docker Hub. | Public `ghcr.io/go-gitea/gitea`, tags `:latest`, `:version`, `:dev`. No `-private` naming. | Servers pull pre-built images via Docker Compose. No compiling on production hosts. |
| **Mihomo / Sing-box** (Go Core + Web/Dashboard) | Multi-stage build for standalone containers; single-stage packaging of pre-built release binaries in CI to minimize runner load. | GitHub Actions triggers on `dev`/`main` pushes, runs full test matrices, publishes prereleases and tags. | Public `ghcr.io/metacubex/mihomo`, `ghcr.io/sagernet/sing-box`. | End-users and servers pull binaries or containers. Zero remote branch sprawl. |
| **Caddy** (Go + Plugins) | Multi-stage `builder` + minimal `alpine` runtime. Custom builds use `xcaddy` locally or official container image. | CI builds releases, publishes to GitHub Releases with SHA256 checksums, and pushes official images. | Public `caddy:latest`, `caddy:alpine`. | Production uses pre-built images or official binaries. |
| **CSP (Current vs Proposed)** | Currently has `Dockerfile` (multi-stage) and `Dockerfile.runtime`, but used `-private` image naming and had a broken `pages.yml`. | Pushes to `dev` run CI and publish prerelease. Proposed: fully remove `pages.yml`, standardize image name to `ghcr.io/einck0/clash-sub-parser`. | Transition from `ghcr.io/einck0/csp-runtime-private` to standard `ghcr.io/einck0/clash-sub-parser`. | Host server strictly pulls pre-built image (zero compilation locally). Developers push to `dev`, no temporary remote branches. |

---

## 2. Architectural Decisions (ADR)

### ADR-1: Standardized Public GHCR Image Naming
- **Decision**: Rename target image package from `ghcr.io/einck0/csp-runtime-private` to `ghcr.io/einck0/clash-sub-parser`.
- **Rationale**: CSP contains zero secrets in its binary or container (database and tokens are external mounts/env). Binary releases are already public. The `-private` suffix was an unnecessary historical artifact that caused complex, fragile private-gate checking scripts.
- **Affected Components**: `.github/workflows/ci.yml`, `.github/scripts/promote_prelease.py`, `scripts/cloud-delivery/preflight-pull.sh`, `tests/cloud-delivery/test_cloud_delivery.py`, `.github/scripts/test_ci_synthetic.py`, `docs/operations/csp-cloud-delivery.md`.

### ADR-2: Dual Dockerfile Architecture
- **Decision**: Maintain both `Dockerfile` (canonical multi-stage) and `Dockerfile.runtime` (CI artifact packaging).
- **Rationale**:
  - `Dockerfile`: Enables any developer or automated build system to build CSP from scratch (`node:22-alpine` -> `golang:1.27.1-alpine` -> `alpine:3.20`) with zero prerequisites.
  - `Dockerfile.runtime`: Enables GitHub Actions to avoid duplicate compilation. The binary is tested and verified once in the CI runner, then copied directly into the runtime container, guaranteeing byte-for-byte identity between the tested binary and the container binary.
  - Both share identical runtime characteristics: Alpine 3.20, `appuser` (UID 10001, GID 10001), port 18080, volume `/data`, and `/healthz` probe.

### ADR-3: Single-Track dev -> prerelease Pipeline & Elimination of `pages.yml`
- **Decision**: Delete `.github/workflows/pages.yml`. Retain single CI workflow `.github/workflows/ci.yml` triggering on `dev`.
- **Rationale**:
  - CSP serves its web UI directly through Go `//go:embed` on port 18080. A standalone static GitHub Pages deployment is obsolete and broken (was looking for deleted `frontend/`).
  - Pushing to `dev` builds the binary, tests it, builds the container, pushes to `ghcr.io/einck0/clash-sub-parser:prelease-<shortsha>`, and creates the GitHub Prerelease. `main` promotes via fast-forward only.

### ADR-4: Local Direct Compilation vs. Remote Branch Pull Strategy
- **Decision**: Strict division of labor:
  - **Local Development**: Developers with Go/Node toolchains run `npm run dev` in `web/` and `go run ./cmd/csp serve` for instant inner-loop feedback.
  - **Server/Host Deployment (This Machine)**: Zero heavy compilation locally (`go test`, `go build`, `npm build` strictly forbidden). The host pulls the pre-built container image from GHCR.
  - **Cloud CI**: Pushing directly to `dev` triggers the cloud build. Creating temporary branches on `origin` (such as `cloud-build/*` or `task/*`) is prohibited.

### ADR-5: Clean Compose File Ecosystem
- **Decision**: Organize compose files by clear operational intent:
  - `docker-compose.yml` (and `docker-compose.prod.yml`): Production deployment using image pull (`ghcr.io/einck0/clash-sub-parser:${CSP_VERSION:-latest}`).
  - `docker-compose.dev.yml`: Local container build testing (`build: .`).
  - `docker-compose.isolated.yml`: Automated isolated test runs with synthetic token and dedicated non-conflicting port.

---

## 3. Implementation Plan & File Modifications

### Phase 1: Workflow & Script Modernization
1. **Remove `.github/workflows/pages.yml`**: Delete file completely.
2. **Update `.github/workflows/ci.yml`**:
   - Update `TARGET_IMAGE_NAME` from `ghcr.io/einck0/csp-runtime-private` to `ghcr.io/einck0/clash-sub-parser`.
   - Update tags to publish `:prelease-<shortsha>`, `:dev`, `:sha-<fullsha>`.
3. **Update `.github/scripts/promote_prelease.py`**:
   - Update `IMAGE = "ghcr.io/einck0/clash-sub-parser"`.
4. **Update `scripts/cloud-delivery/preflight-pull.sh`**:
   - Update default `EXPECTED_PACKAGE` to `ghcr.io/einck0/clash-sub-parser`.

### Phase 2: Test Suite & Synthetic Verification Updates
1. **Update `.github/scripts/test_ci_synthetic.py`**:
   - Update expected image package name to `ghcr.io/einck0/clash-sub-parser`.
2. **Update `tests/cloud-delivery/test_cloud_delivery.py`**:
   - Update expected image package fixtures to `ghcr.io/einck0/clash-sub-parser`.
3. **Execute Python test suites**:
   - Run `python3 -B .github/scripts/test_ci_synthetic.py`
   - Run `python3 -B -m unittest discover -s tests/cloud-delivery -p 'test_cloud_delivery.py' -v`

### Phase 3: Documentation & Compose Alignment
1. **Update `docs/operations/csp-cloud-delivery.md`**:
   - Reflect `ghcr.io/einck0/clash-sub-parser` standard public image name.
   - Document open-source comparison, Dockerfile design rationale, and host zero-compile rule.
2. **Review Docker Compose files**:
   - Ensure `docker-compose.prod.yml`, `docker-compose.dev.yml`, and `docker-compose.isolated.yml` align with standard naming.

---

## 4. Verification & Gate Plan

- **Executor Self-Test**:
  - Verify `test_ci_synthetic.py` passes 100%.
  - Verify `test_cloud_delivery.py` passes 100%.
  - Verify `.github/workflows/pages.yml` does not exist.
  - Verify no occurrences of `csp-runtime-private` in active workflows/scripts.
- **Reviewer Gate**:
  - Independent review against `specs/008-delivery-architecture/spec.md`.
  - Verification that no production containers or databases are touched.
- **Git Commit & Cloud Dev Push**:
  - Commit verified changes using `git_commit`.
  - Push to `origin dev`.
  - Monitor GitHub Actions Run on `dev` to verify all 3 jobs succeed and publish to `ghcr.io/einck0/clash-sub-parser`.
