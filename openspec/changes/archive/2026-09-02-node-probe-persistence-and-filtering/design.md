# Technical Design: Node Probe Persistence and Filtering

## 1. Database Architecture
```
ProbeConfig (id=1, probe_interval_minutes, speedtest_min_speed_mbps, media_platforms...)
NodeProbeResult (node_key, name, server, port, type, status, latency_ms, speed_mbps, ip, country, media, checked_at)
Subscription (+ filter_min_speed_mbps, + filter_media_unlock)
NodeGroup (+ filter_min_speed_mbps, + filter_media_unlock)
```

## 2. Capability Filter Pipeline
```
[ All Candidate Nodes ]
          │
          ▼
[ Regex / Name Exclusions ]
          │
          ▼
[ Query Persistent Probe Results for Candidate Nodes ]
          │
          ▼
[ Check speed_mbps >= filter_min_speed_mbps ]
          │
          ▼
[ Check ∀ platform in filter_media_unlock: media[platform] is unlocked ]
          │
          ▼
[ Filtered Nodes for Output Subscription / Proxy Group ]
```

## 3. Periodic Background Runner
- In `app/services/scheduler.py`:
  - `_poll_node_probes()` job scheduled every minute.
  - Queries `ProbeConfig.probe_interval_minutes`. If `> 0`, checks timestamp of last execution.
  - Pulls all raw nodes from enabled subscriptions and executes `probe_batch_nodes(..., persist_db=True)`.
