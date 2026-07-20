"""Resolve exit-IP / host country with in-memory cache.

Preferred: for a proxy node name, temporarily select it on mihomo GLOBAL,
query a public IP service through the mixed-port proxy, then geo-lookup that IP.
Fallback: resolve node server hostname and geo-lookup the server IP.
"""

from __future__ import annotations

import asyncio
import ipaddress
import logging
import socket
import time
from typing import Any
from urllib.parse import quote

import httpx
from fastapi import APIRouter, Depends
from pydantic import BaseModel, Field
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.database import get_db
from app.services.security_settings_service import get_fetch_proxy_config, get_security_settings

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/geoip", tags=["geoip"])
settings = get_settings()

# key -> (expire_ts, payload)
_CACHE: dict[str, tuple[float, dict[str, Any]]] = {}
_CACHE_TTL = 6 * 3600
_NEGATIVE_TTL = 30 * 60
_EXIT_IP_SERVICES = (
    "https://api.ipify.org",
    "https://ifconfig.me/ip",
    "https://icanhazip.com",
)


class GeoIpRequest(BaseModel):
    hosts: list[str] = Field(default_factory=list, max_length=80)
    # Preferred: proxy node names already loaded in mihomo
    names: list[str] = Field(default_factory=list, max_length=40)
    # Optional mixed-port proxy used to fetch exit IP, e.g. http://host.docker.internal:7890
    exit_proxy_url: str | None = None


class GeoIpResult(BaseModel):
    host: str | None = None
    name: str | None = None
    ip: str | None = None
    country: str | None = None
    country_code: str | None = None
    error: str | None = None
    mode: str = "server"  # server | exit


def _extract_host(item: str) -> str:
    value = str(item or "").strip()
    if not value:
        return ""
    if value.startswith("["):
        end = value.find("]")
        if end > 0:
            return value[1:end]
    if value.count(":") == 1 and not value.replace(".", "").isdigit():
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
    unique = []
    seen = set()
    for ip in ips:
        if not ip or ip in seen:
            continue
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
        if _cache_get(ip) is not None:
            continue
        seen.add(ip)
        unique.append(ip)

    result: dict[str, dict[str, Any]] = {}
    if not unique:
        return result

    body = [
        {"query": ip, "fields": "status,message,country,countryCode,query"}
        for ip in unique[:80]
    ]
    try:
        resp = await client.post(
            "http://ip-api.com/batch?lang=zh-CN", json=body, timeout=12.0
        )
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


async def _select_proxy(client: httpx.AsyncClient, base_url: str, name: str) -> None:
    encoded = quote(name, safe="")
    url = f"{base_url.rstrip('/')}/proxies/GLOBAL"
    resp = await client.put(url, json={"name": name}, timeout=8.0)
    if resp.status_code >= 400:
        # Some builds use different selector names; still try.
        resp2 = await client.put(
            f"{base_url.rstrip('/')}/proxies/{encoded}",
            json={"name": name},
            timeout=8.0,
        )
        if resp2.status_code >= 400:
            raise RuntimeError(f"select failed HTTP {resp.status_code}/{resp2.status_code}")


async def _fetch_exit_ip(proxy_url: str) -> str:
    async with httpx.AsyncClient(
        proxy=proxy_url, timeout=12.0, trust_env=False, follow_redirects=True
    ) as client:
        last_err = "exit ip unavailable"
        for service in _EXIT_IP_SERVICES:
            try:
                resp = await client.get(service)
                text = (resp.text or "").strip()
                if resp.status_code < 400 and _is_ip(text):
                    return text
                last_err = f"{service} -> {resp.status_code}"
            except Exception as exc:
                last_err = str(exc)[:120]
        raise RuntimeError(last_err)


@router.post("/lookup", response_model=list[GeoIpResult])
async def lookup_geoip(
    payload: GeoIpRequest,
    db: AsyncSession = Depends(get_db),
) -> list[GeoIpResult]:
    names = [str(n).strip() for n in (payload.names or []) if str(n).strip()][:40]
    hosts_raw = [str(h).strip() for h in (payload.hosts or []) if str(h).strip()][:80]

    runtime_security = await get_security_settings(db)
    fetch_proxy_enabled, fetch_proxy_url = get_fetch_proxy_config(runtime_security)
    lookup_client_kwargs: dict[str, Any] = {"timeout": 12.0, "trust_env": False}
    if fetch_proxy_enabled and fetch_proxy_url:
        lookup_client_kwargs["proxy"] = fetch_proxy_url

    # Exit-IP path via mihomo selector + mixed port
    api_url = (settings.mihomo_api_url or "").strip()
    if names and api_url:
        secret = (settings.mihomo_api_secret or "").strip()
        headers = {"Authorization": f"Bearer {secret}"} if secret else {}
        exit_proxy = (
            (payload.exit_proxy_url or "").strip()
            or (fetch_proxy_url if fetch_proxy_enabled else "")
            or "http://host.docker.internal:7890"
        )
        output: list[GeoIpResult] = []
        async with httpx.AsyncClient(headers=headers, trust_env=False) as ctl:
            async with httpx.AsyncClient(**lookup_client_kwargs) as geo_client:
                for name in names:
                    cache_key = f"exit:{name}"
                    cached = _cache_get(cache_key)
                    if cached is not None:
                        output.append(
                            GeoIpResult(
                                name=name,
                                ip=cached.get("ip"),
                                country=cached.get("country"),
                                country_code=cached.get("country_code"),
                                error=cached.get("error"),
                                mode="exit",
                            )
                        )
                        continue
                    try:
                        await _select_proxy(ctl, api_url, name)
                        # brief settle for selector switch
                        await asyncio.sleep(0.15)
                        ip = await _fetch_exit_ip(exit_proxy)
                        await _lookup_ips([ip], geo_client)
                        geo = _cache_get(ip) or {
                            "ip": ip,
                            "country": None,
                            "country_code": None,
                            "error": None,
                        }
                        payload_out = {
                            "ip": geo.get("ip") or ip,
                            "country": geo.get("country"),
                            "country_code": geo.get("country_code"),
                            "error": geo.get("error"),
                        }
                        _cache_set(cache_key, payload_out, ttl=30 * 60)
                        output.append(
                            GeoIpResult(
                                name=name,
                                ip=payload_out["ip"],
                                country=payload_out["country"],
                                country_code=payload_out["country_code"],
                                error=payload_out["error"],
                                mode="exit",
                            )
                        )
                    except Exception as exc:
                        err = str(exc)[:120]
                        _cache_set(
                            cache_key,
                            {
                                "ip": None,
                                "country": None,
                                "country_code": None,
                                "error": err,
                            },
                            ttl=120,
                        )
                        output.append(
                            GeoIpResult(name=name, error=err, mode="exit")
                        )
        return output

    # Fallback: hostname/server IP geo
    if not hosts_raw:
        return []

    host_pairs: list[tuple[str, str]] = []
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

    async with httpx.AsyncClient(**lookup_client_kwargs) as client:
        await _lookup_ips(need_lookup, client)

    output = []
    for host, ip in host_pairs:
        if not ip:
            output.append(GeoIpResult(host=host, error="resolve failed", mode="server"))
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
                mode="server",
            )
        )
    return output
