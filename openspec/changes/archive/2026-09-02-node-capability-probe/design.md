## Context

See `proposal.md` for motivation. Currently, `clash-sub-parser` runs as a lightweight FastAPI backend inside Docker. Adding full-protocol proxy checking, geo consensus, streaming unlock checks, and bandwidth testing requires executing real proxy outbounds across diverse protocols (Shadowsocks, VMess, VLESS/Reality, Trojan, Hysteria2). This requires a dedicated outbound engine and an isolated probe runner architecture.

## Goals / Non-Goals

**Goals:**
- **Protocol Fidelity**: Support real handshake verification for all mainstream protocols using `sing-box` as the execution engine.
- **Port & Process Isolation**: Allocate dynamic loopback ports (e.g., 21000–21100) per running node, with guaranteed cleanup and timeout bounds.
- **Clean Separation of Planes**: Control plane (FastAPI / WebUI) manages subscriptions and tasks; Data plane (sing-box + probe workers) performs isolated checks without inheriting host proxy settings (`trust_env=False`).
- **Granular Capability Matrix**: Output structured capability results (IP, Country, YouTube, Netflix, Disney+, ChatGPT, Latency, Download Speed Mbps).
- **Graceful Resource Footprint**: Bounded concurrency (default max 3–5 concurrent node probes) to protect CPU, RAM, and server bandwidth from starvation.

**Non-Goals:**
- **Massive DDoS-style Speedtest**: Not performing multi-gigabit multi-stream flood tests; tests are bounded to 5MB–10MB sample payloads.
- **Replacing Sub-Store / Subs-Check PRO Architecture**: Retain `clash-sub-parser`'s primary focus on flexible rule filtering and subscription management, while offering native self-hosted capability detection.

## Decisions

### Decision 1: Embedded sing-box Runner vs Native Python Protocol Parsers
- **Choice**: Bundle static `sing-box` binary in the Docker container and invoke it with dynamic temporary configs.
- **Rationale**: Python cannot natively handle TLS Reality, uTLS fingerprints, VMess AEAD, Hysteria2 UDP, and TUIC without massive, error-prone C/Rust dependencies. `sing-box` is single-binary, memory-efficient (~15MB per instance), and standard.
- **Alternatives Considered**:
  - *Mihomo*: Larger binary and memory footprint.
  - *Pure Python SOCKS/HTTP only*: Fails on 90% of real-world subscription nodes.

### Decision 2: Outbound Egress Isolation and Port Pool
- **Choice**: Use an in-memory `AsyncPortPool` managing ports `21000–21050`. For each node:
  1. Acquire free port from pool.
  2. Write temporary `config.json` (Mixed inbound on 127.0.0.1:port, target outbound).
  3. Validate config (`sing-box check`).
  4. Spawn `sing-box run` subprocess with async PID tracking.
  5. Wait for TCP ready, run probes via `httpx.AsyncClient(proxy=f"http://127.0.0.1:{port}", trust_env=False)`.
  6. Terminate subprocess, remove temporary file, return port to pool.
- **Rationale**: Prevents any cross-talk between node checks and ensures the host machine's proxy (mihomo 7890) never pollutes the probe.

### Decision 3: Multi-tier Probe Providers
- **Tier 1 (Transport & Ping)**: `http://www.gstatic.com/generate_204` / `http://cp.cloudflare.com/generate_204`.
- **Tier 2 (Geo Consensus)**: `https://ipinfo.io/json` + `https://api.ip.sb/geoip`.
- **Tier 3 (Media & AI Capabilities)**:
  - YouTube: `https://www.youtube.com/premium` (check country code in response).
  - Netflix: `https://www.netflix.com/title/80018499` (check 200/404/redirect).
  - ChatGPT: `https://chatgpt.com` / `https://ios.chat.openai.com` (check Cloudflare 403 / 1020).
- **Tier 4 (Optional Speedtest)**: Download 5MB test file from Cloudflare Speed CDN with timer.

## Risks / Trade-offs

- **[Risk] High CPU / Port Leak on unexpected crash** → **Mitigation**: Wrap runner execution in `try...finally` with async timeout, signal traps, and on-startup cleanup of orphan sing-box processes.
- **[Risk] Target platform IP bans / rate limits** → **Mitigation**: Implement exponential backoff, randomized jitter, and concurrency throttling (max 3 concurrent requests per platform).
- **[Risk] Reality schema / public key format errors** → **Mitigation**: Validate base64 public keys before generating sing-box config, and run `sing-box check` before spawning.
