# Tasks: Delivery Architecture & Container Governance

**Input**: Design documents from `specs/008-delivery-architecture/`
**Prerequisites**: `spec.md`, `plan.md`

## Phase 1: Workflow Pruning & Setup

- [x] T001 Remove obsolete legacy workflow `.github/workflows/pages.yml`

## Phase 2: Standardized Image Naming & Scripts Alignment (US1 & US2)

- [x] T002 Update target image package name in `.github/workflows/ci.yml` from `ghcr.io/einck0/csp-runtime-private` to `ghcr.io/einck0/clash-sub-parser`
- [x] T003 Update `.github/scripts/promote_prelease.py` to reference `IMAGE = "ghcr.io/einck0/clash-sub-parser"`
- [x] T004 Update `scripts/cloud-delivery/preflight-pull.sh` default `EXPECTED_PACKAGE` to `ghcr.io/einck0/clash-sub-parser`

## Phase 3: Test Suites & Regression Contracts (US1 & US2)

- [x] T005 Update `.github/scripts/test_ci_synthetic.py` to test `ghcr.io/einck0/clash-sub-parser`
- [x] T006 Update `tests/cloud-delivery/test_cloud_delivery.py` fixtures and assertions to use `ghcr.io/einck0/clash-sub-parser`
- [x] T007 Run local lightweight synthetic test suites (`test_ci_synthetic.py` and `test_cloud_delivery.py`) to confirm zero regressions (zero Go/npm builds)

## Phase 4: Operations Documentation & Dockerfile/Compose Alignment (US3 & US4)

- [x] T008 Update `docs/operations/csp-cloud-delivery.md` documenting open-source comparison, Dockerfile design rationale, image naming, and host zero-compile boundary
- [x] T009 Verify `Dockerfile`, `Dockerfile.runtime`, and Docker Compose templates align with port 18080, non-root user 10001, and standard naming

## Phase 5: Review & Delivery Convergence

- [x] T010 Independent reviewer read-only verification against `specs/008-delivery-architecture/spec.md` (Verdict: PASS, Report: `rep_rev_whole_task_6431aa81-0415-47d2-af3f-fbcd02a2cc2b`)
- [ ] T011 Commit changes via `git_commit`, push to `origin dev`, and verify GitHub Actions cloud run succeeds with prelease published to `ghcr.io/einck0/clash-sub-parser`
