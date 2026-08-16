"""Lightweight TCP reachability probe for subscription nodes.

This is NOT proxy latency and NOT protocol handshake.
It only answers: can we open a TCP connection to server:port in time?
"""

from __future__ import annotations

import asyncio
import time
from typing import Any


DEFAULT_TIMEOUT_MS = 2000
DEFAULT_CONCURRENCY = 20
MAX_NODES = 300


def _normalize_target(node: dict[str, Any]) -> dict[str, Any] | None:
    if not isinstance(node, dict):
        return None
    name = str(node.get("name") or "").strip()
    server = str(node.get("server") or "").strip()
    if not server:
        return None
    try:
        port = int(node.get("port"))
    except Exception:
        return None
    if port <= 0 or port > 65535:
        return None
    return {
        "name": name or f"{server}:{port}",
        "server": server,
        "port": port,
        "type": str(node.get("type") or "").strip(),
    }


async def _probe_one(
    server: str,
    port: int,
    timeout_s: float,
) -> dict[str, Any]:
    started = time.monotonic()
    try:
        # asyncio.open_connection covers DNS + TCP connect.
        reader, writer = await asyncio.wait_for(
            asyncio.open_connection(server, port),
            timeout=timeout_s,
        )
        writer.close()
        try:
            await writer.wait_closed()
        except Exception:
            pass
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return {
            "status": "ok",
            "connect_ms": elapsed_ms,
            "error": None,
        }
    except asyncio.TimeoutError:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return {
            "status": "timeout",
            "connect_ms": elapsed_ms,
            "error": "timeout",
        }
    except OSError as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return {
            "status": "fail",
            "connect_ms": elapsed_ms,
            "error": str(exc) or type(exc).__name__,
        }
    except Exception as exc:  # pragma: no cover - defensive
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return {
            "status": "fail",
            "connect_ms": elapsed_ms,
            "error": str(exc) or type(exc).__name__,
        }


async def probe_nodes(
    nodes: list[dict[str, Any]],
    *,
    timeout_ms: int = DEFAULT_TIMEOUT_MS,
    concurrency: int = DEFAULT_CONCURRENCY,
) -> dict[str, Any]:
    """Probe a batch of nodes with bounded concurrency.

    Returns:
      {
        "results": [{name, server, port, type, status, connect_ms, error}, ...],
        "summary": {total, ok, fail, timeout, skip},
        "timeout_ms": int,
      }
    """
    timeout_ms = max(200, min(int(timeout_ms or DEFAULT_TIMEOUT_MS), 10000))
    concurrency = max(1, min(int(concurrency or DEFAULT_CONCURRENCY), 50))
    timeout_s = timeout_ms / 1000.0

    targets: list[dict[str, Any]] = []
    skipped: list[dict[str, Any]] = []
    for raw in (nodes or [])[:MAX_NODES]:
        target = _normalize_target(raw)
        if target is None:
            skipped.append(
                {
                    "name": str((raw or {}).get("name") or "").strip() if isinstance(raw, dict) else "",
                    "server": "",
                    "port": None,
                    "type": str((raw or {}).get("type") or "").strip() if isinstance(raw, dict) else "",
                    "status": "skip",
                    "connect_ms": None,
                    "error": "missing server/port",
                }
            )
            continue
        targets.append(target)

    sem = asyncio.Semaphore(concurrency)
    results: list[dict[str, Any]] = []

    async def run_one(target: dict[str, Any]) -> dict[str, Any]:
        async with sem:
            probe = await _probe_one(target["server"], target["port"], timeout_s)
            return {**target, **probe}

    if targets:
        results = list(await asyncio.gather(*(run_one(t) for t in targets)))

    all_results = results + skipped
    summary = {
        "total": len(all_results),
        "ok": sum(1 for r in all_results if r["status"] == "ok"),
        "fail": sum(1 for r in all_results if r["status"] == "fail"),
        "timeout": sum(1 for r in all_results if r["status"] == "timeout"),
        "skip": sum(1 for r in all_results if r["status"] == "skip"),
    }
    return {
        "results": all_results,
        "summary": summary,
        "timeout_ms": timeout_ms,
    }


def is_probably_udp_only(node_type: str) -> bool:
    """Hint only; still try TCP unless caller decides to skip."""
    t = (node_type or "").lower()
    return t in {"hysteria", "hysteria2", "tuic", "wireguard"}
