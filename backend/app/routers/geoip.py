"""Resolve node host -> IP country code/name with in-memory cache."""

from __future__ import annotations

import asyncio
import ipaddress
import logging
import socket
import time
from typing import Any

import httpx
from fastapi import APIRouter
from pydantic import BaseModel, Field
from sqlalchemy.ext.asyncio import AsyncSession
from fastapi import Depends

from app.database import get_db
from app.services.security_settings_service import get_fetch_proxy_config, get_security_settings

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/geoip", tags=["geoip"])

# host/ip -> (expire_ts, payload)
_CACHE: dict[str, tuple[float, dict[str, Any]]] = {}
_CACHE_TTL = 6 * 3600
_NEGATIVE_TTL = 30 * 60


class GeoIpRequest(BaseModel):
    hosts: list[str] = Field(default_factory=list, max_length=80)
    # Accept "host", "host:port", or raw IP.


class GeoIpResult(BaseModel):
    host: str
    ip: str | None = None
    country: str | None = None
    country_code: str | None = None
    error: str | None = None


def _extract_host(item: str) -> str:
    value = str(item or "").strip()
    if not value:
        return ""
    # strip scheme leftovers and brackets for IPv6-ish inputs
    if value.startswith("["):
        end = value.find("]")
        if end > 0:
            return value[1:end]
    if value.count(":") == 1 and not value.replace(".", "").isdigit():
        # hostname:port or ipv4:port
        return value.rsplit(":", 1)[0]
    return value


def _is_ip(value: str) -> bool:
    try:
        ipaddress.ip_address(value)
        return True
    except ValueError:
        return False


def _cache_get(key: str) -> dict[str, Any] | None:
    item = _CACHE.get(key)
    if not item:
        return None
    expire_ts, payload = item
    if expire_ts < time.time():
        _CACHE.pop(key, None)
        return None
    return payload


def _cache_set(key: str, payload: dict[str, Any], ttl: int = _CACHE_TTL) -> None:
    _CACHE[key] = (time.time() + ttl, payload)


async def _resolve_ip(host: str) -> str | None:
    if not host:
        return None
    if _is_ip(host):
        return host
    try:
        infos = await asyncio.get_running_loop().getaddrinfo(
            host, None, type=socket.SOCK_STREAM
        )
        for info in infos:
            addr = info[4][0]
            if _is_ip(addr):
                return addr
    except OSError:
        return None
    return None


async def _lookup_ips(
    ips: list[str],
    client: httpx.AsyncClient,
) -> dict[str, dict[str, Any]]:
    """Batch lookup via ip-api.com (HTTP free tier)."""
    unique = []
    seen = set()
    for ip in ips:
        if not ip or ip in seen:
            continue
        # skip private / local
        try:
            obj = ipaddress.ip_address(ip)
            if obj.is_private or obj.is_loopback or obj.is_link_local or obj.is_reserved:
                _cache_set(
                    ip,
                    {
                        "ip": ip,
                        "country": "Private",
                        "country_code": "LAN",
                        "error": None,
                    },
                    ttl=_CACHE_TTL,
                )
                continue
        except ValueError:
            continue
        cached = _cache_get(ip)
        if cached is not None:
            continue
        seen.add(ip)
        unique.append(ip)

    result: dict[str, dict[str, Any]] = {}
    if not unique:
        return result

    # ip-api free batch endpoint, max 100
    body = [{"query": ip, "fields": "status,message,country,countryCode,query"} for ip in unique[:80]]
    try:
        resp = await client.post("http://ip-api.com/batch?lang=zh-CN", json=body, timeout=12.0)
        resp.raise_for_status()
        data = resp.json()
        if not isinstance(data, list):
            raise ValueError("unexpected geoip response")
        for item in data:
            if not isinstance(item, dict):
                continue
            ip = str(item.get("query") or "").strip()
            if not ip:
                continue
            if item.get("status") == "success":
                payload = {
                    "ip": ip,
                    "country": item.get("country") or None,
                    "country_code": item.get("countryCode") or None,
                    "error": None,
                }
                _cache_set(ip, payload)
                result[ip] = payload
            else:
                payload = {
                    "ip": ip,
                    "country": None,
                    "country_code": None,
                    "error": str(item.get("message") or "lookup failed"),
                }
                _cache_set(ip, payload, ttl=_NEGATIVE_TTL)
                result[ip] = payload
    except Exception as exc:
        logger.warning("geoip batch lookup failed: %s", exc)
        for ip in unique:
            payload = {
                "ip": ip,
                "country": None,
                "country_code": None,
                "error": "geoip unavailable",
            }
            _cache_set(ip, payload, ttl=120)
            result[ip] = payload
    return result


@router.post("/lookup", response_model=list[GeoIpResult])
async def lookup_geoip(
    payload: GeoIpRequest,
    db: AsyncSession = Depends(get_db),
) -> list[GeoIpResult]:
    hosts_raw = [str(h or "").strip() for h in (payload.hosts or []) if str(h or "").strip()]
    hosts_raw = hosts_raw[:80]
    if not hosts_raw:
        return []

    # Resolve hostnames concurrently.
    host_pairs: list[tuple[str, str]] = []  # (display_host, resolved_ip_or_empty)
    resolve_tasks = []
    display_hosts: list[str] = []
    for item in hosts_raw:
        host = _extract_host(item)
        display_hosts.append(host or item)
        resolve_tasks.append(_resolve_ip(host))

    resolved = await asyncio.gather(*resolve_tasks, return_exceptions=True)
    need_lookup: list[str] = []
    for host, ip_or_exc in zip(display_hosts, resolved):
        if isinstance(ip_or_exc, Exception) or not ip_or_exc:
            host_pairs.append((host, ""))
            continue
        ip = str(ip_or_exc)
        host_pairs.append((host, ip))
        if _cache_get(ip) is None:
            need_lookup.append(ip)

    runtime_security = await get_security_settings(db)
    fetch_proxy_enabled, fetch_proxy_url = get_fetch_proxy_config(runtime_security)
    client_kwargs: dict[str, Any] = {"timeout": 12.0, "trust_env": False}
    if fetch_proxy_enabled and fetch_proxy_url:
        client_kwargs["proxy"] = fetch_proxy_url

    async with httpx.AsyncClient(**client_kwargs) as client:
        await _lookup_ips(need_lookup, client)

    output: list[GeoIpResult] = []
    for host, ip in host_pairs:
        if not ip:
            output.append(GeoIpResult(host=host, error="resolve failed"))
            continue
        cached = _cache_get(ip) or {
            "ip": ip,
            "country": None,
            "country_code": None,
            "error": "unknown",
        }
        output.append(
            GeoIpResult(
                host=host,
                ip=cached.get("ip") or ip,
                country=cached.get("country"),
                country_code=cached.get("country_code"),
                error=cached.get("error"),
            )
        )
    return output
