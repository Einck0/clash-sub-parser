"""Latency checks.

Primary path: ask mihomo external-controller to dial through the named proxy
to a real URL (default google generate_204). Fallback: TCP connect to host:port.
"""

from __future__ import annotations

import asyncio
import logging
import socket
import time
from urllib.parse import quote

import httpx
from fastapi import APIRouter
from pydantic import BaseModel, Field

from app.config import get_settings

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/latency", tags=["latency"])
settings = get_settings()


class LatencyRequest(BaseModel):
    # Backward compatible: host:port TCP checks
    hosts: list[str] = Field(default_factory=list)
    timeout_ms: int = 5000
    # Preferred: proxy node names already present in mihomo
    names: list[str] = Field(default_factory=list)
    test_url: str | None = None


class LatencyResult(BaseModel):
    # For TCP path
    host: str | None = None
    port: int | None = None
    # For proxy path
    name: str | None = None
    latency_ms: float | None = None
    error: str | None = None
    mode: str = "tcp"  # tcp | proxy


async def _check_tcp(host: str, port: int, timeout: float) -> LatencyResult:
    try:
        start = time.monotonic()
        _, writer = await asyncio.wait_for(
            asyncio.open_connection(host, port),
            timeout=timeout,
        )
        elapsed = (time.monotonic() - start) * 1000
        writer.close()
        await writer.wait_closed()
        return LatencyResult(host=host, port=port, latency_ms=round(elapsed, 1), mode="tcp")
    except asyncio.TimeoutError:
        return LatencyResult(host=host, port=port, error="timeout", mode="tcp")
    except (OSError, socket.gaierror) as exc:
        return LatencyResult(host=host, port=port, error=str(exc)[:100], mode="tcp")


async def _check_proxy_delay(
    client: httpx.AsyncClient,
    base_url: str,
    name: str,
    test_url: str,
    timeout_ms: int,
) -> LatencyResult:
    encoded = quote(name, safe="")
    url = f"{base_url.rstrip('/')}/proxies/{encoded}/delay"
    try:
        resp = await client.get(
            url,
            params={"url": test_url, "timeout": str(timeout_ms)},
            timeout=(timeout_ms / 1000) + 2,
        )
        if resp.status_code >= 400:
            detail = ""
            try:
                detail = str(resp.json().get("message") or resp.text)[:120]
            except Exception:
                detail = resp.text[:120]
            return LatencyResult(
                name=name,
                error=detail or f"HTTP {resp.status_code}",
                mode="proxy",
            )
        data = resp.json()
        delay = data.get("delay")
        if delay is None:
            return LatencyResult(name=name, error="no delay", mode="proxy")
        return LatencyResult(name=name, latency_ms=float(delay), mode="proxy")
    except Exception as exc:
        return LatencyResult(name=name, error=str(exc)[:120], mode="proxy")


@router.post("/check", response_model=list[LatencyResult])
async def check_latency(payload: LatencyRequest):
    timeout_ms = max(500, min(int(payload.timeout_ms or 5000), 30000))
    names = [str(n).strip() for n in (payload.names or []) if str(n).strip()][:50]
    hosts = [str(h).strip() for h in (payload.hosts or []) if str(h).strip()][:50]

    # Prefer real proxy latency when mihomo is configured and names provided.
    api_url = (settings.mihomo_api_url or "").strip()
    if names and api_url:
        headers = {}
        secret = (settings.mihomo_api_secret or "").strip()
        if secret:
            headers["Authorization"] = f"Bearer {secret}"
        test_url = (payload.test_url or settings.mihomo_test_url or "").strip()
        if not test_url:
            test_url = "https://www.google.com/generate_204"
        async with httpx.AsyncClient(headers=headers, trust_env=False) as client:
            tasks = [
                _check_proxy_delay(client, api_url, name, test_url, timeout_ms)
                for name in names
            ]
            results = await asyncio.gather(*tasks, return_exceptions=True)
        output: list[LatencyResult] = []
        for name, item in zip(names, results):
            if isinstance(item, LatencyResult):
                output.append(item)
            elif isinstance(item, Exception):
                output.append(LatencyResult(name=name, error=str(item)[:120], mode="proxy"))
            else:
                output.append(LatencyResult(name=name, error="unknown", mode="proxy"))
        return output

    # Fallback: TCP connect latency to host:port
    timeout = timeout_ms / 1000
    tasks = []
    for item in hosts:
        parts = item.rsplit(":", 1)
        if len(parts) != 2:
            tasks.append(
                asyncio.sleep(0, result=LatencyResult(host=item, port=0, error="invalid format", mode="tcp"))
            )
            continue
        host, port_str = parts
        try:
            port = int(port_str)
        except ValueError:
            tasks.append(
                asyncio.sleep(0, result=LatencyResult(host=host, port=0, error="invalid port", mode="tcp"))
            )
            continue
        tasks.append(_check_tcp(host, port, timeout))

    results = await asyncio.gather(*tasks, return_exceptions=True)
    output = []
    for item in results:
        if isinstance(item, LatencyResult):
            output.append(item)
        elif isinstance(item, Exception):
            output.append(LatencyResult(host="unknown", port=0, error=str(item)[:100], mode="tcp"))
    return output
