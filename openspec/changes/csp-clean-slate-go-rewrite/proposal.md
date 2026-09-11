# Proposal: CSP Clean-Slate Go Rewrite 1.0 (High-Concurrency Node Prober & Single-Binary Control Plane)

## Why

Current `clash-sub-parser` (CSP) runs as a Python 3.10 FastAPI backend inside Docker (`clash-sub-parser-app`). While rich in features (subscriptions, 29 node groups, 470 routing rules, 14 rule categories, 5 export targets, and deep media/AI unlock observations), its node probing path creates an isolated `sing-box` child process with temporary on-disk JSON configs and loopback port allocation (`AsyncPortPool` on ports 21000..21100). This process-per-node execution model limits practical concurrency to 5–20 nodes, creates disk/socket thrashing under large inventories (7,236 nodes in production), and requires a heavy multi-gigabyte Python+Node container stack.

User Einck has authorized a **Clean-Slate Go Rewrite 1.0** targeting:
1. High-concurrency node probing (100–500 concurrency capability) benchmarking against `subs-check-pro` (`sinspired/subs-check-pro`);
2. Single static binary delivery containing the embedded modernized Vue 3 + Tailwind + shadcn frontend (`embed.FS`);
3. 100% lossless migration of all production assets (8 subscriptions, 7,236 nodes, 47,907 node-source links, 29 groups, 470 rules, 7,297 probe results, 50 snapshots);
4. Strict operational safety: zero-downtime rollback capability, immutable production database backups, and frozen Git safeguard checkpoints.

## What Changes

- **Runtime & Deployment**:
  - Replace Python FastAPI + Uvicorn + APScheduler runtime with a unified Go service.
  - Deliver a single statically-linked binary (`/app/clash-sub-parser`) embedding the compiled frontend bundle via Go standard library `embed.FS`.
  - Dramatically simplify Docker packaging: Alpine/scratch-based container replacing the 130MB Debian/Python image.
- **Probe Execution Engine**:
  - Transition from child-process spawn (`exec.Command("sing-box", "run")`) to an in-process, memory-direct dialing probe engine inspired by `subs-check-pro` and official `sagernet/sing-box` packages.
  - Implement a bounded goroutine worker pool with sliding window dispatch, supporting 100–500 concurrency without port exhaustion or temporary file churn.
  - Preserve strict physical egress isolation: every probe HTTP/TLS/UDP client MUST explicitly bind the node's isolated proxy transport with environment proxy variables (`HTTP_PROXY`, `ALL_PROXY`) disabled.
- **Database & Lossless Migration**:
  - Retain SQLite as the persistent storage engine using pure-Go `modernc.org/sqlite` (enabling CGO-free static compilation) or CGO `mattn/go-sqlite3`.
  - Establish a formal, read-only migration verification dry-run pipeline that audits all 7,297 probe results, 7,236 nodes, 8 subscriptions, 29 groups, and 470 rules row-for-row and byte-for-byte.
  - Adopt the unified target schema (`probe_profiles`, `probe_jobs`, `probe_observations`) while maintaining complete backward compatibility for legacy queries and exports.
- **Configuration Compilation & Multi-Client Distribution**:
  - Adopt `flosch/pongo2` (Jinja2 compatible Go template engine) to guarantee 100% seamless migration of existing configuration templates.
  - Retain deterministic export pipelines for all 5 client targets: Clash Meta/Mihomo, Sing-box, Surge, Loon, and Quantumult X (QX), referencing `tindy2013/subconverter` for canonical protocol normalization.
- **Prior Art & Reference Repositories (Mandatory Guidance)**:
  - In accordance with Einck's explicit engineering directive, implementation tasks must reference proven open-source implementations:
    1. `sinspired/subs-check-pro`: Reference for high-concurrency Go proxy probing, goroutine worker pool, and in-memory direct dialing.
    2. `sagernet/sing-box`: Reference for official Go outbound protocol structures, reality/vless options, and dialer integration.
    3. `tindy2013/subconverter`: Reference for multi-protocol parsing and standard client config serialization (Clash/Surge/Loon/QX/Singbox).
    4. `xflash-panda/sub-store`: Reference for subscription aggregation, deduplication, and node grouping logic.
    5. `flosch/pongo2`: Reference for Jinja2 template compatibility in Go.
    6. `modernc.org/sqlite`: Reference for pure-Go CGO-free SQLite database persistence.

## Capabilities

### New Capabilities

- `go-single-binary-control-plane`: Go application architecture, embedded SPA web assets, configuration loader, and graceful shutdown lifecycle.
- `in-memory-probe-engine`: High-concurrency goroutine worker pool, in-memory proxy dialer, multi-tier capability probes (transport, geo identity, media/AI unlocks, speedtest), and strict cancellation bounds.
- `lossless-sqlite-migration`: Offline read-only rehearsal, schema transformation from legacy `node_probe_results` to `probe_observations`, checksum verification, and rollback guarantee.
- `multi-client-configuration-compiler`: Canonical intermediate graph representation, regex dynamic expansion, `flosch/pongo2` template rendering, and 5-target export APIs.

### Modified Capabilities

- None (Clean-slate replacement of the backend control plane with 100% feature parity).

## Impact

- **Codebase**: A new `cmd/` and `internal/` Go project structure will be created. The existing `backend/` Python application remains active and unmodified during planning and implementation until formal cutover.
- **Data**: The live SQLite database (`/var/lib/docker/volumes/clash-sub-parser_backend-data/_data/clash_sub_parser.db`, 44.38 MB, 11,362 pages) will be cloned into an immutable offline backup before migration verification. No data in the live volume is modified during planning.
- **Toolchain**: The host environment lacks a local Go compiler. Compilation and verification will be driven in Docker containers with Go >= 1.25 (as required by `sing-box` 1.14.0).
- **Rollback Asset**: The current container (`clash-sub-parser-app`) and existing volumes are preserved intact as immediate fallback assets.
