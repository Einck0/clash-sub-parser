# Feature Specification: Delivery Architecture & Container Governance

**Feature Branch**: `008-delivery-architecture`
**Created**: 2026-10-16
**Status**: Ready
**Input**: Comprehensive review and restructuring of GitHub Actions CI/CD, Docker container design, build & pull strategy, image naming, and delivery workflow against open-source industry standards (Gitea, Caddy, Mihomo, Sing-box).

## Context & Objectives

Clash Sub Parser (CSP) has evolved into a Go + Vue embedded single-binary fullstack service. Previously, several transitional mechanisms were introduced:
1. Historical `-private` container naming (`ghcr.io/einck0/csp-runtime-private`) with excessive private gate checks, despite CSP code and releases being public and images containing zero secrets or sensitive data.
2. A broken legacy GitHub Pages workflow (`.github/workflows/pages.yml`) failing on push due to references to a deleted `frontend/` directory.
3. Confusion between local compilation, temporary branch building, and host deployment.
4. Divergent Docker Compose files without clear role separation.

This specification benchmarks against mature open-source projects (Mihomo, Sing-box, Caddy, Gitea) to establish a clean, single-track, reproducible delivery architecture.

---

## User Scenarios & Priority

### User Story 1 - Standardized Public Container Image & Multi-Stage Architecture (Priority: P1)

As an operator or developer, I want a standardized, cleanly named container image (`ghcr.io/einck0/clash-sub-parser`) built with standard OCI labels, non-root user execution, and proper build separation, so that deployment follows standard container practices without artificial private gates.

**Why this priority**: Core naming and container packaging foundation; eliminates misleading `-private` tags and ensures images can be pulled reliably.

**Independent Test**:
- Build canonical multi-stage `Dockerfile` and verify static binary embedding.
- Package `Dockerfile.runtime` with tested static binary, verify UID 10001, OCI revision labels, and healthcheck.
- Verify `ghcr.io/einck0/clash-sub-parser` image tags and pull manifests without any private gate blockers.

**Acceptance Scenarios**:
1. **Given** the repository root, **When** examining `Dockerfile`, **Then** it provides a reproducible multi-stage build (`node:22-alpine` -> `golang:1.27.1-alpine` -> `alpine:3.20` non-root runtime) that any developer can build locally with `docker build`.
2. **Given** pre-compiled binary artifacts from CI, **When** building `Dockerfile.runtime`, **Then** it packages the exact static binary into an identical Alpine 3.20 non-root runtime with matching UID/GID (10001:10001) and healthcheck probe.
3. **Given** container registry configuration, **When** published to GHCR, **Then** the image is named `ghcr.io/einck0/clash-sub-parser` with immutable tags (`:sha-<fullsha>`, `:prelease-<shortsha>`, `:dev`) and zero `-private` references.

---

### User Story 2 - Single-Track GitHub Actions CI/CD & Obsolete Workflow Cleanup (Priority: P1)

As a maintainer, I want a single-track, reliable GitHub Actions pipeline where pushing to `dev` automatically tests, builds, and publishes to GitHub Releases (`prelease-<shortsha>`) and GHCR, while obsolete workflows like `pages.yml` are eliminated.

**Why this priority**: Eliminates CI noise, fixes broken Pages runs, and aligns with the user's explicit rule: test on `dev` -> publish to `prerelease`, no temporary branch creation.

**Independent Test**:
- Verify deletion of `.github/workflows/pages.yml`.
- Verify `.github/workflows/ci.yml` triggers only on `dev` (push/PR) and handles publishing cleanly.
- Verify test scripts and delivery scripts validate `ghcr.io/einck0/clash-sub-parser`.

**Acceptance Scenarios**:
1. **Given** `.github/workflows/`, **When** inspecting workflow files, **Then** `.github/workflows/pages.yml` is deleted, preventing failed GitHub Pages runs.
2. **Given** a push to `dev`, **When** `.github/workflows/ci.yml` runs, **Then** it executes backend and frontend checks, publishes the runtime container to `ghcr.io/einck0/clash-sub-parser`, and creates/updates the immutable GitHub Prerelease (`prelease-<shortsha>`).
3. **Given** `.github/scripts/promote_prelease.py` and `scripts/cloud-delivery/preflight-pull.sh`, **When** executed, **Then** they reference `ghcr.io/einck0/clash-sub-parser` and pass all synthetic regression tests.

---

### User Story 3 - Clear Separation of Build, Pull, and Host Deployment (Priority: P2)

As a system operator on a resource-constrained server, I want a clearly documented and implemented deployment model where the host server never compiles code, never creates temporary remote branches, and strictly deploys by pulling verified release containers.

**Why this priority**: Protects server CPU/memory, eliminates branch sprawl, and adheres to GitOps/container best practices.

**Independent Test**:
- Inspect documentation in `docs/operations/csp-cloud-delivery.md` and verify clear boundaries between local dev, cloud CI, and server deployment.
- Verify that deployment scripts pull pre-built OCI images by exact SHA/digest rather than running local compiler toolchains.

**Acceptance Scenarios**:
1. **Given** deployment on the host server, **When** updating CSP, **Then** the operator pulls the pre-built image from GHCR without running `go build` or `npm build` locally.
2. **Given** developer feature work, **When** requesting cloud validation, **Then** changes are pushed directly to `dev` (or a PR to `dev`), never creating temporary `cloud-build/*` or `task/*` branches on origin.

---

### User Story 4 - Minimal, Decoupled Docker Compose Ecosystem (Priority: P2)

As an operator and developer, I want a clear, minimal set of Docker Compose configurations for production deployment, local container building, and isolated testing.

**Why this priority**: Prevents configuration drift and clarifies environment variable defaults.

**Independent Test**:
- Validate `docker-compose.yml` (production default with GHCR image pull).
- Validate `docker-compose.dev.yml` (local multi-stage build context).
- Validate `docker-compose.isolated.yml` (isolated automated testing with dedicated network and ports).

**Acceptance Scenarios**:
1. **Given** `docker-compose.yml` (or `docker-compose.prod.yml`), **Then** it defaults to `ghcr.io/einck0/clash-sub-parser:latest` (or `:prelease-latest`), with clear volume mappings for `/data`.
2. **Given** `docker-compose.dev.yml`, **Then** it builds from local `Dockerfile` for testing local container builds.
3. **Given** `docker-compose.isolated.yml`, **Then** it runs preview containers on isolated ports with `--disable-subscription-scheduler` and `--disable-probe-coordinator` for non-interfering critic testing.

---

## Functional Requirements

- **FR-ARC-01 (Image Naming)**: The container package name SHALL be `ghcr.io/einck0/clash-sub-parser`. All occurrences of `ghcr.io/einck0/csp-runtime-private` SHALL be replaced across workflows, scripts, and tests.
- **FR-ARC-02 (Pages Removal)**: The broken legacy workflow `.github/workflows/pages.yml` SHALL be deleted.
- **FR-ARC-03 (Dockerfile Alignment)**: Both `Dockerfile` (multi-stage) and `Dockerfile.runtime` (pre-built binary packaging) SHALL use Alpine 3.20, non-root user `appuser` (UID 10001, GID 10001), port 18080, volume `/data`, and identical health check endpoints (`/healthz`).
- **FR-ARC-04 (CI Single-Track Delivery)**: CI SHALL trigger on `push` to `dev`, build and verify static binary artifacts, publish runtime container to `ghcr.io/einck0/clash-sub-parser`, and create an immutable GitHub Prerelease (`prelease-<shortsha>`).
- **FR-ARC-05 (Host Zero-Compile Rule)**: The host environment SHALL strictly deploy via pre-built container pulls, never running heavy compilation (`go build`, `npm run build`, `go test`) on the production host.
- **FR-ARC-06 (Automated Verification)**: All synthetic test suites (`test_ci_synthetic.py` and `test_cloud_delivery.py`) SHALL be updated to validate the new image name and workflow structure, passing 100% with exit code 0.

---

## Non-Functional Requirements & Constraints

- **No Secret Baking**: Container images MUST NOT contain database files, API tokens, private keys, or credentials.
- **Zero Local Host Compilation**: Local commands are restricted to script execution, unit tests of scripts, and git operations. Heavy builds occur solely in GitHub Actions.
- **Backwards Compatibility & Safety**: Production container currently runs on port 18080 with volume `csp-v1-data`; configuration schema and database paths (`/data/csp-v1.db`) remain strictly unchanged.
