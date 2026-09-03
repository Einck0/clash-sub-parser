"""节点能力探测中枢服务：调度并发、持久化存储、整合各级探测项"""
from __future__ import annotations

import asyncio
import logging
import time
from typing import Any

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.node_probe_result import NodeProbeResult
from app.services.probe.providers import (
    check_download_speed,
    check_geo_identity,
    check_media_unlock,
    check_transport,
)
from app.services.probe.runner import spawn_node_runner

logger = logging.getLogger(__name__)

# 内存快速缓存
_PROBE_CACHE: dict[str, dict[str, Any]] = {}
_CACHE_TTL_S = 900  # 15分钟缓存


def _get_node_key(node: dict[str, Any]) -> str:
    server = str(node.get("server") or "").strip()
    port = str(node.get("port") or "")
    ntype = str(node.get("type") or "").strip()
    name = str(node.get("name") or "").strip()
    return f"{name}|{ntype}|{server}:{port}"


def get_cached_result(node: dict[str, Any]) -> dict[str, Any] | None:
    key = _get_node_key(node)
    item = _PROBE_CACHE.get(key)
    if item and time.monotonic() < item.get("expires_at", 0):
        return item.get("data")
    return None


def set_cached_result(node: dict[str, Any], result: dict[str, Any], ttl_s: int = _CACHE_TTL_S) -> None:
    key = _get_node_key(node)
    _PROBE_CACHE[key] = {
        "data": result,
        "expires_at": time.monotonic() + ttl_s,
    }


def get_all_cached_results() -> dict[str, dict[str, Any]]:
    now = time.monotonic()
    valid_cache = {}
    for k, v in list(_PROBE_CACHE.items()):
        if now < v.get("expires_at", 0):
            valid_cache[k] = v.get("data", {})
        else:
            _PROBE_CACHE.pop(k, None)
    return valid_cache


def clear_probe_cache() -> None:
    _PROBE_CACHE.clear()


async def save_probe_result_to_db(db: AsyncSession, result: dict[str, Any]) -> None:
    """持久化单个节点的探测结果到数据库"""
    await save_probe_results_batch_to_db(db, [result])


async def save_probe_results_batch_to_db(db: AsyncSession, results: list[dict[str, Any]]) -> None:
    """批量持久化节点探测结果到数据库"""
    if not results:
        return

    node_keys = [r.get("node_key") or _get_node_key(r) for r in results]
    stmt = select(NodeProbeResult).where(NodeProbeResult.node_key.in_(node_keys))
    res = await db.execute(stmt)
    existing_map = {item.node_key: item for item in res.scalars().all()}

    for result in results:
        node_key = result.get("node_key") or _get_node_key(result)
        name = result.get("name") or ""
        server = result.get("server") or ""
        port = result.get("port")
        ntype = result.get("type") or ""
        status = result.get("status") or "unknown"
        latency_ms = result.get("latency_ms")
        speed_mbps = result.get("speed_mbps")
        ip = result.get("ip")
        country = result.get("country")
        asn = result.get("asn")
        organization = result.get("organization")
        media = result.get("media") or {}
        error = result.get("error")
        checked_at = result.get("checked_at") or int(time.time())

        existing = existing_map.get(node_key)
        if existing:
            existing.status = status
            existing.latency_ms = latency_ms
            if speed_mbps is not None:
                existing.speed_mbps = speed_mbps
            existing.ip = ip
            existing.country = country
            existing.asn = asn
            existing.organization = organization
            existing.media = media
            existing.error = error
            existing.checked_at = checked_at
            db.add(existing)
        else:
            new_row = NodeProbeResult(
                node_key=node_key,
                name=name,
                server=server,
                port=port,
                type=ntype,
                status=status,
                latency_ms=latency_ms,
                speed_mbps=speed_mbps,
                ip=ip,
                country=country,
                asn=asn,
                organization=organization,
                media=media,
                error=error,
                checked_at=checked_at,
            )
            db.add(new_row)
            existing_map[node_key] = new_row

    await db.commit()


async def delete_all_db_probe_results(db: AsyncSession) -> None:
    """清空数据库中所有节点的探测记录"""
    from sqlalchemy import delete
    await db.execute(delete(NodeProbeResult))
    await db.commit()
    clear_probe_cache()


async def get_all_db_probe_results(db: AsyncSession) -> dict[str, dict[str, Any]]:
    """从数据库加载所有节点的最新探测记录字典"""
    res = await db.execute(select(NodeProbeResult))
    items = res.scalars().all()
    results_map: dict[str, dict[str, Any]] = {}
    for item in items:
        data = {
            "node_key": item.node_key,
            "name": item.name,
            "server": item.server,
            "port": item.port,
            "type": item.type,
            "status": item.status,
            "latency_ms": item.latency_ms,
            "speed_mbps": item.speed_mbps,
            "ip": item.ip,
            "country": item.country,
            "asn": item.asn,
            "organization": item.organization,
            "media": item.media or {},
            "error": item.error,
            "checked_at": item.checked_at,
        }
        results_map[item.node_key] = data
        if item.name:
            results_map[item.name] = data
    return results_map


async def probe_single_node(
    node: dict[str, Any],
    *,
    probe_enabled: bool = True,
    speedtest_enabled: bool = False,
    media_check_enabled: bool = True,
    media_platforms: list[str] | None = None,
    speedtest_url: str = "https://speed.cloudflare.com/__down?bytes=5000000",
    speedtest_max_bytes: int = 5242880,
    speedtest_timeout_s: float = 5.0,
    probe_timeout_ms: int = 3000,
    use_cache: bool = True,
    db: AsyncSession | None = None,
) -> dict[str, Any]:
    """对单个节点执行完整能力探测"""
    if use_cache:
        cached = get_cached_result(node)
        if cached:
            # 如果请求要求测速但缓存没有测速数据，则继续执行
            if not (speedtest_enabled and cached.get("speed_mbps") is None):
                return cached

    name = str(node.get("name") or "").strip()
    server = str(node.get("server") or "").strip()
    port = node.get("port")
    node_type = str(node.get("type") or "").strip()

    base_result: dict[str, Any] = {
        "node_key": _get_node_key(node),
        "name": name,
        "server": server,
        "port": port,
        "type": node_type,
        "status": "unknown",
        "latency_ms": None,
        "speed_mbps": None,
        "ip": None,
        "country": None,
        "asn": None,
        "organization": None,
        "media": {},
        "error": None,
        "checked_at": int(time.time()),
    }

    if not probe_enabled:
        base_result["status"] = "skipped"
        return base_result

    timeout_s = max(probe_timeout_ms / 1000.0, 1.0)
    platforms = media_platforms if media_platforms is not None else ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"]

    try:
        async with spawn_node_runner(node) as runner_ctx:
            proxy_url = runner_ctx["proxy_url"]

            # 1. 基础握手与 204 往返延迟
            transport = await check_transport(proxy_url, timeout_s=timeout_s)
            base_result["latency_ms"] = transport.get("latency_ms")
            if transport.get("status") != "ok":
                base_result["status"] = "fail"
                base_result["error"] = transport.get("error") or "Transport 204 failed"
                set_cached_result(node, base_result)
                if db:
                    await save_probe_result_to_db(db, base_result)
                return base_result

            base_result["status"] = "ok"

            # 2. 真实出口 IP 与地理位置共识
            geo = await check_geo_identity(proxy_url, timeout_s=timeout_s)
            base_result["ip"] = geo.get("ip")
            base_result["country"] = geo.get("country")
            base_result["asn"] = geo.get("asn")
            base_result["organization"] = geo.get("organization")

            # 3. 流媒体与 AI 解锁探测
            if media_check_enabled and platforms:
                media_res = await check_media_unlock(proxy_url, platforms=platforms, timeout_s=timeout_s)
                base_result["media"] = media_res

            # 4. 可选带宽测速
            if speedtest_enabled:
                speed_res = await check_download_speed(
                    proxy_url,
                    speedtest_url=speedtest_url,
                    max_bytes=speedtest_max_bytes,
                    timeout_s=speedtest_timeout_s,
                )
                base_result["speed_mbps"] = speed_res.get("speed_mbps")

            set_cached_result(node, base_result)
            if db:
                await save_probe_result_to_db(db, base_result)

    except Exception as exc:
        base_result["status"] = "fail"
        base_result["error"] = str(exc)
        set_cached_result(node, base_result)
        if db:
            await save_probe_result_to_db(db, base_result)

    return base_result


async def probe_batch_nodes(
    nodes: list[dict[str, Any]],
    *,
    probe_enabled: bool = True,
    speedtest_enabled: bool = False,
    media_check_enabled: bool = True,
    media_platforms: list[str] | None = None,
    speedtest_url: str = "https://speed.cloudflare.com/__down?bytes=5000000",
    speedtest_max_bytes: int = 5242880,
    speedtest_timeout_s: float = 5.0,
    probe_timeout_ms: int = 4000,
    concurrency: int = 5,
    use_cache: bool = True,
    db: AsyncSession | None = None,
) -> dict[str, Any]:
    """批量并发探测节点能力"""
    sem = asyncio.Semaphore(max(1, min(concurrency, 20)))

    async def _worker(n: dict[str, Any]) -> dict[str, Any]:
        async with sem:
            return await probe_single_node(
                n,
                probe_enabled=probe_enabled,
                speedtest_enabled=speedtest_enabled,
                media_check_enabled=media_check_enabled,
                media_platforms=media_platforms,
                speedtest_url=speedtest_url,
                speedtest_max_bytes=speedtest_max_bytes,
                speedtest_timeout_s=speedtest_timeout_s,
                probe_timeout_ms=probe_timeout_ms,
                use_cache=use_cache,
                db=None,
            )

    results = await asyncio.gather(*[_worker(n) for n in nodes], return_exceptions=False)

    if db:
        try:
            await save_probe_results_batch_to_db(db, results)
        except Exception as exc:
            logger.warning("Failed to batch save probe results: %s", exc)
            try:
                await db.rollback()
            except Exception:
                pass

    summary = {
        "total": len(results),
        "ok": sum(1 for r in results if r.get("status") == "ok"),
        "fail": sum(1 for r in results if r.get("status") == "fail"),
        "timeout": sum(1 for r in results if r.get("status") == "timeout"),
        "skipped": sum(1 for r in results if r.get("status") == "skipped"),
    }

    return {
        "results": results,
        "summary": summary,
    }
