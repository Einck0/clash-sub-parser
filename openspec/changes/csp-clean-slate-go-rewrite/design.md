# Design: CSP Clean-Slate Go Rewrite 1.0

## Context & Background

Clash Subscription Parser (CSP) manages 8 production subscriptions with 7,236 active proxy nodes, 29 dynamic node groups, 470 routing rules, and 7,297 capability probe records stored in a 44.38 MB SQLite database (`clash_sub_parser.db`). The existing Python/FastAPI implementation manages background probing by invoking `/usr/local/bin/sing-box` (version 1.14.0) via `asyncio.create_subprocess_exec` on dynamic loopback ports (`AsyncPortPool` ports 21000..21100).

User Einck authorized a clean-slate Go rewrite targeting the high-concurrency model demonstrated by `subs-check-pro` (`sinspired/subs-check-pro`), requesting single-binary delivery with embedded frontend assets, high concurrent probing (100–500 concurrency), and 100% lossless migration of production SQLite data.

## Prior Art & Reference Architecture Mapping

In accordance with Einck's explicit engineering directive, subagents and executors must NOT reinvent core mechanics. The implementation directly leverages proven open-source paradigms:

| Domain | Prior Art / Reference Repository | Architectural Role & Implementation Pattern |
|---|---|---|
| **High-Concurrency Probe Engine** | `sinspired/subs-check-pro` | Goroutine sliding-window worker pool (`sync.WaitGroup` + buffered semaphore channel), in-memory dialer context, non-blocking DNS cache, and timeout bucket pipeline. |
| **Proxy Protocol Dialing & sing-box Core** | `sagernet/sing-box` (v1.14.0) | Upstream Go library for outbound protocol definitions (`option.Outbound`), VLESS/Reality, VMess AEAD, Hysteria2, TUIC, and memory-direct `box.Box` / `adapter.Outbound` dialers. |
| **Multi-Protocol Parsing & Serialization** | `tindy2013/subconverter` | Industry gold standard for parsing Clash, V2Ray/VMess base64, SIP002, SSR, Trojan, and Hysteria URLs into normalized node schemas, and rendering output configs. |
| **Subscription Aggregation & Rule Pipeline** | `xflash-panda/sub-store` | Modern reference for multi-source subscription merging, canonical node deduplication (`name|type|server:port`), and tag-based dynamic grouping. |
| **Template Engine (Jinja2 Compatible)** | `flosch/pongo2` | High-performance Go implementation of Jinja2 syntax, allowing 100% seamless reuse of existing Clash/Mihomo/Surge/Loon/QX configuration templates without rewriting. |
| **Pure-Go SQLite Persistence** | `modernc.org/sqlite` | CGO-free, pure-Go SQLite driver enabling static cross-compilation for Linux amd64/arm64 without gcc/glibc toolchain friction. |
| **Lightweight Web Framework** | `go-chi/chi` | Idiomatic, 100% `net/http` compatible lightweight router with zero heap allocations during route matching, ideal for micro-footprint control planes. |

---

## Architectural Decisions

### Decision 1: Subcheck Architectural Benchmark & Memory/Concurrency Reality

#### Concurrency Quantified Analysis
- **Python Legacy Baseline**: Subprocess per node -> 1 fork/exec + 1 temporary config JSON file on disk + 1 local loopback TCP port + 1 `sing-box run` instance + 1 `httpx` client. Concurrency was capped at 10–20 workers by file descriptors, port exhaustion (100-port range 21000..21100), and Python GIL/event-loop latency.
- **Go Target Model (`subs-check-pro` benchmark)**: Goroutines per node with buffered task channels. 100–500 goroutines multiplexing over Go's `net.Dialer` and `crypto/tls` / `sing-box` outbound dialers without subprocess fork, without on-disk temp files, and without local TCP loopback ports.
- **Throughput**: 100 concurrent workers completing a 2-second timeout probe process ~3,000 nodes per minute, completing the entire 7,236 production node inventory in ~2.5 minutes (compared to ~45 minutes in Python).

#### Memory Target Quantified Reality & Feasibility
- **The 15–25MB Target Context**:
  - The historical reference to "15MB" in `openspec/changes/archive/2026-09-02-node-capability-probe/design.md:22` described the resident memory of a *single standalone ephemeral sing-box subprocess*, not the total memory of a running web application with an embedded prober.
  - The standalone compiled sing-box 1.14.0 binary is **91.84 MB** on disk.
  - Linking `github.com/sagernet/sing-box` in-process pulls in gVisor netstack, `quic-go` (v0.61), uTLS (v1.8), WireGuard, and comprehensive crypto tables. Baseline idle Go heap + runtime allocations for such a binary start at **35MB–60MB**.
  - During an active 100–500 concurrent probe run, TLS connection buffers, HTTP/2 frame buffers, and DNS resolver maps require an additional **40MB–100MB** of dynamic heap.
- **Architectural Resolution**:
  - **Single-Binary In-Process Mode**: A realistic memory ceiling of **80MB–150MB** during full 500-concurrency batch probing (idle 30MB–45MB).
  - **Memory Guard Pipeline**: Use `debug.SetMemoryLimit` (Go 1.19+ soft memory limit) configured to 150MB, coupled with worker pool throttling: if host or cgroup memory approaches limit, the worker pool dynamically decreases concurrency from 500 down to 50.

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

### Decision 3: Lossless SQLite Migration & Dual-Schema Reconciliation

Production SQLite holds:
- 7,236 nodes, 8 subscriptions, 29 groups, 470 rules, 14 rule categories, 50 snapshots
- 7,297 rows in `node_probe_results` (legacy active table)
- 0 rows in `probe_profiles`, `probe_jobs`, `probe_observations` (target schema introduced in Alembic migration `csp_target_domain_schema`)

**Migration Strategy**:
1. **Zero Data Loss**: Maintain complete compatibility with `node_probe_results` while populating the new domain tables (`probe_observations`, `probe_jobs`, `probe_profiles`).
2. **Unified Data Layer**: The Go backend will query and update both models, ensuring existing UI queries (which expect `node_key`, `latency_ms`, `media` JSON) receive identical responses.
3. **Offline Dry-Run Verification**:
   - Step 1: Copy `/var/lib/docker/volumes/clash-sub-parser_backend-data/_data/clash_sub_parser.db` to an isolated rehearsal database.
   - Step 2: Run Go migration tool in `--dry-run --verify` mode.
   - Step 3: Assert row counts match exactly: 7,236 nodes, 8 subscriptions, 29 groups, 470 rules, 7,297 probe observations.
   - Step 4: Compare generated Clash and Sing-box configs before and after migration byte-for-byte.

### Decision 4: Single-Binary Distribution with Go Standard `embed.FS`

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
    
    // Embedded SPA static file serving
    distFS, _ := fs.Sub(embeddedFrontend, "frontend/dist")
    r.Handle("/*", http.FileServer(http.FS(distFS)))
}
```

### Decision 5: Template Compatibility with `flosch/pongo2`

Existing configuration templates rely on Jinja2-style filters and control flow (`{% for group in groups %}`, `{{ node.name }}`, `{% if ... %}`).
- Go standard `text/template` has different syntax and lacks Jinja2 filters, which would require manual rewriting of complex user templates.
- `flosch/pongo2` supports Django/Jinja2 template syntax directly in Go, enabling **100% zero-modification migration** of existing Clash, Mihomo, Stash, and Shadowrocket template files.

---

## Risk Analysis & Mitigation Matrix

| Risk | Severity | Mitigation Strategy |
|---|---|---|
| **Host Toolchain Lacks Go Compiler** | High | Build entirely via Docker multi-stage build using official `golang:1.26-alpine` image; no host compiler installation required. |
| **sing-box 1.14 Requires Go >= 1.25** | High | Explicitly pin Docker builder to Go 1.26 (`golang:1.26-alpine`); pass `-checklinkname=0` ldflag and `badlinkname,tfogo_checklinkname0` build tags. |
| **In-Process Memory Spikes on 500 Concurrency** | Medium | Bounded sliding-window worker pool (`concurrency: 100..500` user-configurable in `probe_config`); soft memory limit via `debug.SetMemoryLimit(150MB)`. |
| **Production SQLite Corruption during Cutover** | Critical | Cold snapshot of production volume to `/home/service/clash-sub-parser/backups/` before cutover; read-only dry-run rehearsal script must pass 100% before live switch. |
| **Existing Python Container Fallback** | Low | Old container `clash-sub-parser` and image `clash-sub-parser-app` are preserved stopped, enabling instant 5-second `docker compose up -d` rollback. |
