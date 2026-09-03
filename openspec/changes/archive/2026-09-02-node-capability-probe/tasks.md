## 1. Environment and Infrastructure Preparation

- [x] 1.1 Download static sing-box binary, verify executable integrity via `sing-box version`, and update `Dockerfile` to bundle it in runtime image.
- [x] 1.2 Add `httpx` (with SOCKS/HTTP proxy support) to `requirements.txt` and verify dependencies install cleanly.

## 2. Core Outbound Runner and Port Management

- [x] 2.1 Implement `AsyncPortPool` and `SingBoxRunner` in `app/services/probe/runner.py` with port lifecycle management and verify unit test passes.
- [x] 2.2 Implement Clash-to-SingBox outbound converter supporting SS, VMess, VLESS/Reality, Trojan, Hysteria2 and verify schema validation test suite passes.

## 3. Modular Probe Providers Implementation

- [x] 3.1 Implement Transport & Latency probe (gstatic/cloudflare 204) and verify latency measurement with simulated mock proxies.
- [x] 3.2 Implement Geo Identity consensus provider (ipinfo, ip.sb) and verify country code matching logic.
- [x] 3.3 Implement Media and AI unlock probes (YouTube, Netflix, ChatGPT, Gemini, Meta AI) and verify classification logic on mock responses.
- [x] 3.4 Implement Bounded Speedtest downloader and verify throughput calculation accuracy.

## 4. Database Schema, Service Orchestration, and API Routes

- [x] 4.1 Update database models and Alembic migrations for node probe results, latency, speed, and media capability tags.
- [x] 4.2 Create `/api/probe/batch` and `/api/probe/status` endpoints with concurrency control and background task dispatch.
- [x] 4.3 Update subscription generation pipeline to support filtering by capability tags (e.g., `filter_media=netflix`).

## 5. Frontend UI Integration and End-to-End Verification

- [x] 5.1 Add capability badges (Latency, Speed, Netflix, YouTube, ChatGPT, Gemini, Meta AI) to `NodeLedger.vue` and `NodePreviewList.vue`.
- [x] 5.2 Add "Test Capabilities" / "Batch Speedtest" action button with progress modal.
- [x] 5.3 Run end-to-end integration test against live test subscription and verify database persistence and WebUI display.
