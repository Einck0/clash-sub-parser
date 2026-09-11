# Design: CSP Clean-Slate Go Rewrite 1.0

## Context

See `proposal.md` for background and motivation. Clash Subscription Parser (CSP) manages 8 production subscriptions with 7,236 active proxy nodes, 29 dynamic node groups, 470 routing rules, and 7,297 capability probe records stored in a 44.38 MB SQLite database (`clash_sub_parser.db`). The existing Python/FastAPI implementation manages background probing by invoking `/usr/local/bin/sing-box` (version 1.14.0) via `asyncio.create_subprocess_exec` on dynamic loopback ports (`AsyncPortPool` ports 21000..21100).

User Einck authorized a clean-slate Go rewrite targeting the high-concurrency model demonstrated by `subs-check-pro` (`sinspired/subs-check-pro`), requesting single-binary delivery with embedded frontend assets, high concurrent probing (100–500 concurrency), and 100% lossless migration of production SQLite data.

## Goals / Non-Goals

**Goals:**
- **High-Concurrency Probing**: Replace subprocess fork/exec with goroutines and in-memory dialers capable of 100–500 concurrency.
- **Single-Binary Delivery**: Package HTTP server, background scheduler, probe engine, and compiled Vue 3 + Tailwind + shadcn-vue assets into a single static binary via `embed.FS`.
- **100% Lossless Migration**: Retain all 7,236 nodes, 8 subscriptions, 29 groups, 470 rules, and 7,297 historical probe results without schema truncation or secret leakage.
- **Physical Egress Isolation**: Guarantee probe connections exit exclusively through the designated proxy node without inheriting host environment proxies (`HTTP_PROXY`, `ALL_PROXY`).
- **Deterministic 5-Target Export**: Render exact, valid configurations for Clash Meta (Mihomo), Sing-box, Surge, Loon, and Quantumult X.
- **Operational Safety**: Enable 5-second rollback to the stopped Python container in the event of an unforeseen operational anomaly.

**Non-Goals:**
- No rewrite of the existing frontend: The modernized Vue 3 + Tailwind CSS + shadcn-vue frontend is preserved as-is and embedded directly.
- No remote cloud database migration: SQLite is retained as the authoritative single source of truth; no PostgreSQL or MySQL dependency is introduced.
- No deprecation of existing export URLs: All existing endpoint paths (`/clash`, `/mihomo`, `/stash`, `/shadowrocket`, `/sing-box`, `/api/*`) maintain exact signature parity.

---

## Prior Art & Reference Architecture Mapping

In accordance with Einck's explicit engineering directive, subagents and executors must NOT reinvent core mechanics. The implementation directly leverages proven open-source paradigms:

| Domain | Prior Art / Reference Repository | Architectural Role & Implementation Pattern |
|---|---|---|
| **High-Concurrency Probe Engine** | `sinspired/subs-check-pro` | Goroutine sliding-window worker pool (`sync.WaitGroup` + buffered semaphore channel), in-memory dialer context, non-blocking DNS cache, and timeout bucket pipeline. |
| **Proxy Protocol Dialing & sing-box Core** | `sagernet/sing-box` (v1.14.0) | Upstream Go library for outbound protocol definitions (`option.Outbound`), VLESS/Reality, VMess AEAD, Hysteria2, TUIC, and memory-direct `box.Box` / `adapter.Outbound` dialers. |
| **Multi-Protocol Parsing & Serialization** | `tindy2013/subconverter` | Industry gold standard for parsing Clash, V2Ray/VMess base64, SIP002, SSR, Trojan, and Hysteria URLs into normalized node schemas, and rendering output configs. |
| **Subscription Aggregation & Rule Pipeline** | `xflash-panda/sub-store` | Modern reference for multi-source subscription merging, canonical node deduplication (`name\|type\|server:port`), and tag-based dynamic grouping. |
| **Pure-Go SQLite Persistence** | `modernc.org/sqlite` | CGO-free, pure-Go SQLite driver enabling static cross-compilation for Linux amd64/arm64 without gcc/glibc toolchain friction. |
| **Lightweight Web Framework** | `go-chi/chi` | Idiomatic, 100% `net/http` compatible lightweight router with zero heap allocations during route matching, ideal for micro-footprint control planes. |

---

## Decisions

### Decision 1: Subcheck Architectural Benchmark & Memory/Concurrency Reality

#### Concurrency Quantified Analysis
- **Python Legacy Baseline**: Subprocess per node -> 1 fork/exec + 1 temporary config JSON file on disk + 1 local loopback TCP port + 1 `sing-box run` instance + 1 `httpx` client. Concurrency was capped at 10–20 workers by file descriptors, port exhaustion (100-port range 21000..21100), and Python GIL/event-loop latency.
- **Go Target Model (`subs-check-pro` benchmark)**: Goroutines per node with buffered task channels. 100–500 goroutines multiplexing over Go's `net.Dialer` and `crypto/tls` / `sing-box` outbound dialers without subprocess fork, without on-disk temp files, and without local TCP loopback ports.
- **Throughput**: 100 concurrent workers completing a 2-second timeout probe process ~3,000 nodes per minute, completing the entire 7,236 production node inventory in ~2.5 minutes (compared to ~45 minutes in Python).

#### Memory Target Quantified Reality & Feasibility
- **The 15–25MB Target Context**:
  - The historical reference to "15MB" in `openspec/changes/archive/2026-09-02-node-capability-probe/design.md:22` described the resident memory of a *single standalone ephemeral sing-box subprocess*, not the total memory of a running web application with an embedded prober.
  - The standalone compiled sing-box 1.14.0 binary is **91.84 MB** on disk.
  - Linking `github.com/sagernet/sing-box` in-process pulls in gVisor netstack, `quic-go` (v0.61), uTLS (v1.8), WireGuard, and comprehensive crypto tables. Baseline idle Go heap + runtime allocations for such a binary start at **25MB–40MB**.
  - During an active 100–500 concurrent probe run, TLS connection buffers, HTTP/2 frame buffers, and DNS resolver maps require an additional **40MB–100MB** of dynamic heap.
- **Architectural Resolution**:
  - **Memory Footprint Profile**:
    - Idle Control Plane: 20MB–35MB RSS.
    - Active Probing (100 concurrency): 60MB–90MB RSS.
    - Peak Batch Probing (200–500 concurrency): 90MB–150MB RSS.
  - **Memory Guard Pipeline**: Use `debug.SetMemoryLimit` (Go 1.19+ soft memory limit) configured to 200MB, coupled with worker pool throttling: if host or cgroup memory approaches limit, the worker pool dynamically throttles concurrency down to prevent OOM.

### Decision 2: sing-box In-Process Embedding vs Direct Outbound Dialer

To achieve high concurrency without process overhead while avoiding sing-box's complex global lifecycle, we adopt a layered dialing strategy:

1. **Direct Protocol Layer (Lightweight fast path for 70%+ nodes)**:
   - For standard protocols (Shadowsocks, Trojan, HTTP/SOCKS5, plain VMess), use Go native or lightweight dialers directly with `net/http.Transport`.
2. **sing-box Memory Outbound Layer (For complex protocols)**:
   - For VLESS/Reality (uTLS fingerprinting), Hysteria2 (QUIC/UDP obfs), and TUIC, instantiate memory-only sing-box outbound dialers using `github.com/sagernet/sing-box/adapter` and `box.New` with in-memory pipes.
   - Do NOT run a full `box.Box` daemon per node. Instead, use a shared single `box.Box` instance managing an internal routing table with dynamically registered outbounds, OR use `sing-box/common/dialer` directly.
3. **Egress Isolation Guarantee**:
   - Every outbound request MUST use `Proxy: nil` on the HTTP transport and route solely through the node dialer.
   - The Go runtime must explicitly unset or ignore `HTTP_PROXY`, `HTTPS_PROXY`, and `ALL_PROXY` for probe connections, upholding the core invariant of `proxy-node-capability-probing`.

### Decision 3: Programmatic Target Adapters vs Jinja2 Templates

- **Empirical Grounding**: Codebase inspection of `backend/app/services/compiler_target_adapters.py` confirms that CSP does NOT use external Jinja2 template files. The Python implementation programmatically constructs dictionaries and serializes them:
  - Clash / Mihomo / Stash -> serialized via customized YAML dumper.
  - Sing-box -> serialized via `json.dumps`.
  - Surge / Loon / Quantumult X -> generated via section-based text builders (`[Proxy]`, `[Proxy Group]`, `[Rule]`).
- **Decision**: Reject introducing `flosch/pongo2` as an unnecessary abstraction layer (YAGNI). Implement native Go strongly-typed struct models and serializers in `internal/compiler/adapters/`:
  - Clash/Mihomo/Stash: serialized via `gopkg.in/yaml.v3` (with proper quoting of reality short-ids).
  - Sing-box: serialized via standard library `encoding/json`.
  - Surge/Loon/QX: programmatic string builders formatting sections identically to Python's `compiler_target_adapters.py`.
- **Benefit**: Eliminates template parsing overhead, guarantees type safety, reduces memory allocations, and delivers 100% byte-for-byte output compatibility.

### Decision 4: Pure-Go `modernc.org/sqlite` for CGO-Free Static Compilation

- **Rationale**: Using pure-Go `modernc.org/sqlite` allows `CGO_ENABLED=0` static binary builds. This completely eliminates glibc/musl compatibility hurdles, simplifies Docker containerization to a minimal Alpine/scratch base, and avoids cross-compilation toolchain issues across architectures.
- **Performance**: Given CSP's volume (7,236 nodes, 470 rules, reads/writes take single-digit milliseconds under WAL mode), the pure-Go SQLite engine delivers exceptional throughput well within all performance budgets.

### Decision 5: Single-Binary Distribution with Go Standard `embed.FS`

```go
package main

import (
    "embed"
    "io/fs"
    "net/http"
    "github.com/go-chi/chi/v5"
)

//go:embed frontend/dist/*
var embeddedFrontend embed.FS

func RegisterRoutes(r chi.Router) {
    // API routes
    r.Mount("/api", apiRouter)
    // 5-target export routes
    r.Get("/clash", exportClashHandler)
    r.Get("/mihomo", exportMihomoHandler)
    r.Get("/stash", exportStashHandler)
    r.Get("/shadowrocket", exportShadowrocketHandler)
    r.Get("/sing-box", exportSingboxHandler)
    
    // Embedded SPA static file serving with fallback to index.html
    distFS, _ := fs.Sub(embeddedFrontend, "frontend/dist")
    r.Handle("/*", spaFileServer(http.FS(distFS)))
}
```

### Decision 6: Lossless SQLite Migration & Dual-Schema Reconciliation

Production SQLite holds:
- 7,236 nodes, 8 subscriptions, 29 groups, 470 rules, 14 rule categories, 50 snapshots
- 7,297 rows in `node_probe_results` (legacy active table)
- 0 rows in `probe_profiles`, `probe_jobs`, `probe_observations` (target schema introduced in Alembic migration `csp_target_domain_schema`)

**Migration Strategy**:
1. **Direct SQLite Compatibility**: The Go repository models will map directly to the existing production SQLite tables (`subscriptions`, `nodes`, `node_groups`, `rules`, `rule_categories`, `node_probe_results`), ensuring immediate zero-copy read capability.
2. **Dual-Read / Dual-Write Reconciliation**: Ensure all existing UI queries (which expect `node_key`, `latency_ms`, `media` JSON) continue to receive identical responses.
3. **Offline Dry-Run Verification**:
   - Step 1: Copy `/var/lib/docker/volumes/clash-sub-parser_backend-data/_data/clash_sub_parser.db` to an isolated rehearsal database.
   - Step 2: Run Go migration tool in `--dry-run --verify` mode.
   - Step 3: Assert row counts match exactly: 7,236 nodes, 8 subscriptions, 29 groups, 470 rules, 7,297 probe observations.
   - Step 4: Compare generated Clash and Sing-box configs before and after migration byte-for-byte.

---

## Risks / Trade-offs

| Risk | Severity | Mitigation Strategy |
|---|---|---|
| **Host Toolchain Lacks Go Compiler** | High | Build entirely via Docker multi-stage build using official `golang:1.26-alpine` image; no host compiler installation required. |
| **sing-box 1.14 Requires Go >= 1.25** | High | Explicitly pin Docker builder to Go 1.26 (`golang:1.26-alpine`); pass `-checklinkname=0` ldflag and `badlinkname,tfogo_checklinkname0` build tags. |
| **In-Process Memory Spikes on 500 Concurrency** | Medium | Bounded sliding-window worker pool (`concurrency: 100..500` user-configurable in `probe_config`); soft memory limit via `debug.SetMemoryLimit(200MB)`. |
| **Production SQLite Corruption during Cutover** | Critical | Cold snapshot of production volume to `/home/service/clash-sub-parser/backups/` before cutover; read-only dry-run rehearsal script must pass 100% before live switch. |
| **Existing Python Container Fallback** | Low | Old container `clash-sub-parser` and image `clash-sub-parser-app` are preserved stopped, enabling instant 5-second `docker compose up -d` rollback. |

---

## Migration Plan

1. **Phase 0 (Safety Freeze)**: Create Git tag/branch snapshot of the 11 commits and 44 uncommitted working tree files; cold backup live SQLite database to `backups/clash_sub_parser_pre_go_rewrite_20260911.db`.
2. **Phase 1 (Go Foundation & Data Layer)**: Initialize Go module, build `modernc.org/sqlite` repository layer, verify read-only queries against the rehearsal database.
3. **Phase 2 (In-Memory Probe Engine)**: Implement goroutine pool and memory dialers; benchmark 100–500 concurrency probe throughput against test nodes.
4. **Phase 3 (Compiler & Embed Packaging)**: Implement 5-target configuration adapters in Go; package `frontend/dist` via `embed.FS`; produce single binary.
5. **Phase 4 (Offline Rehearsal & Verification)**: Run `migration-verify --dry-run` and Golden Fixtures regression suite comparing output against Python backend.
6. **Phase 5 (Container Cutover & Canary)**: Deploy new Go container on port 18080; run health check and test export queries; keep Python container stopped for instant rollback.
