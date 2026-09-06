"""节点能力探测中枢服务：调度并发、持久化存储、整合各级探测项"""
from __future__ import annotations

import asyncio
import copy
import json
import logging
import re
import time
from typing import Any

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.node_probe_result import NodeProbeResult
from app.schemas.probe import ResolvedProbeConfig
from app.services.probe.models import ALLOWED_EVIDENCE_KEYS
from app.services.probe.providers import (
    check_download_speed,
    check_geo_identity,
    check_media_unlock,
    check_transport,
)
from app.services.probe.runner import spawn_node_runner
from app.utils.capability_filter import is_media_full_unlocked

logger = logging.getLogger(__name__)

FORBIDDEN_SECRET_KEYS = {
    "cookie",
    "cookies",
    "authorization",
    "proxy-authorization",
    "proxy_authorization",
    "token",
    "access_token",
    "refresh_token",
    "secret",
    "password",
    "proxy_password",
    "raw_body",
    "body",
    "response_body",
    "request_body",
}

_URL_CRED_REGEX = re.compile(r"https?://([^:]+):([^@]+)@", re.IGNORECASE)
_BEARER_REGEX = re.compile(r"Bearer\s+[a-zA-Z0-9_\-\.]{10,}", re.IGNORECASE)


def _sanitize_string(s: str) -> str:
    s = _URL_CRED_REGEX.sub(r"http://[REDACTED]@", s)
    s = _BEARER_REGEX.sub(r"Bearer [REDACTED]", s)
    return s


def sanitize_probe_evidence(data: Any) -> Any:
    """Recursively scrub forbidden secret keys, credentials, and non-allowlisted evidence keys."""
    if isinstance(data, dict):
        cleaned: dict[str, Any] = {}
        for k, v in data.items():
            k_str = str(k)
            k_lower = k_str.lower().strip()
            if k_lower in FORBIDDEN_SECRET_KEYS:
                continue

            if k_lower == "evidence" and isinstance(v, dict):
                ev_clean = {
                    ev_k: (_sanitize_string(ev_v) if isinstance(ev_v, str) else ev_v)
                    for ev_k, ev_v in v.items()
                    if ev_k in ALLOWED_EVIDENCE_KEYS
                }
                if "signals" in ev_clean and isinstance(ev_clean["signals"], list):
                    ev_clean["signals"] = [
                        _sanitize_string(sig) if isinstance(sig, str) else sig
                        for sig in ev_clean["signals"]
                    ]
                cleaned[k_str] = ev_clean
            elif isinstance(v, str):
                cleaned[k_str] = _sanitize_string(v)
            elif isinstance(v, (dict, list)):
                cleaned[k_str] = sanitize_probe_evidence(v)
            else:
                cleaned[k_str] = v
        return cleaned
    elif isinstance(data, list):
        return [
            _sanitize_string(item) if isinstance(item, str) else sanitize_probe_evidence(item)
            for item in data
        ]
    elif isinstance(data, str):
        return _sanitize_string(data)
    return data


def merge_probe_media(
    existing_media: dict[str, Any] | None,
    new_media: dict[str, Any] | None,
) -> dict[str, Any]:
    """Perform structural additive merge of probe media JSON payloads.

    Retains existing platform observations and legacy simple keys while atomically
    updating probed platforms with sanitized evidence-grade results.
    """
    merged: dict[str, Any] = copy.deepcopy(existing_media or {})
    if not new_media:
        return merged

    for plat_key, new_val in new_media.items():
        if isinstance(new_val, dict):
            sanitized_val = sanitize_probe_evidence(new_val)
            merged[plat_key] = sanitized_val
        else:
            merged[plat_key] = new_val

    return merged

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
        media = copy.deepcopy(result.get("media") or {})
        if result.get("identity_evidence") and "_identity" not in media:
            media["_identity"] = {
                "confidence": result.get("identity_confidence") or "verified",
                "identity_evidence": result["identity_evidence"],
            }
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
            existing.media = merge_probe_media(existing.media, media)
            existing.error = _sanitize_string(error) if isinstance(error, str) else error
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
                media=merge_probe_media(None, media),
                error=_sanitize_string(error) if isinstance(error, str) else error,
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
        media_val = item.media or {}
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
            "media": media_val,
            "error": item.error,
            "checked_at": item.checked_at,
        }
        if "_identity" in media_val and isinstance(media_val["_identity"], dict):
            ident = media_val["_identity"]
            if "identity_evidence" in ident:
                data["identity_evidence"] = ident["identity_evidence"]
            if "confidence" in ident:
                data["identity_confidence"] = ident["confidence"]
        results_map[item.node_key] = data
        if item.name:
            results_map[item.name] = data
    return results_map


MAX_SUMMARY_PAYLOAD_BYTES = 51200
STANDARD_MEDIA_PLATFORMS = (
    "youtube",
    "netflix",
    "disney",
    "chatgpt",
    "claude",
    "bilibili",
    "meta_ai",
    "gemini",
)


def serialize_probe_summary_item(item: NodeProbeResult) -> dict[str, Any]:
    """序列化单个节点的轻量级摘要（严格仅包含状态、时延、测速、国家、IP 与流媒体布尔摘要）"""
    media_dict = item.media if isinstance(item.media, dict) else {}
    platforms = list(STANDARD_MEDIA_PLATFORMS)
    for k in media_dict.keys():
        if not k.startswith("_") and k not in platforms:
            platforms.append(k)

    media_matrix = {
        p: is_media_full_unlocked(media_dict.get(p))
        for p in platforms
    }

    return {
        "status": item.status or "unknown",
        "latency_ms": item.latency_ms,
        "speed_mbps": round(item.speed_mbps, 2) if item.speed_mbps is not None else None,
        "country": item.country,
        "ip": item.ip,
        "media": media_matrix,
    }


def serialize_probe_detail_item(item: NodeProbeResult) -> dict[str, Any]:
    """序列化单个节点的完整详细探测记录，包含诊断证据链，并执行全量敏感凭据脱敏"""
    media_val = item.media or {}
    data: dict[str, Any] = {
        "node_key": item.node_key,
        "name": item.name,
        "server": item.server,
        "port": item.port,
        "type": item.type,
        "status": item.status,
        "latency_ms": item.latency_ms,
        "speed_mbps": round(item.speed_mbps, 2) if item.speed_mbps is not None else None,
        "ip": item.ip,
        "country": item.country,
        "asn": item.asn,
        "organization": item.organization,
        "media": sanitize_probe_evidence(media_val),
        "error": item.error,
        "checked_at": item.checked_at,
    }
    if "_identity" in media_val and isinstance(media_val["_identity"], dict):
        ident = media_val["_identity"]
        if "identity_evidence" in ident:
            data["identity_evidence"] = sanitize_probe_evidence(ident["identity_evidence"])
        if "confidence" in ident:
            data["identity_confidence"] = ident["confidence"]
    return sanitize_probe_evidence(data)


async def get_db_probe_detail(db: AsyncSession, node_key: str) -> dict[str, Any] | None:
    """通过精确 node_key 查询单个节点的完整探测详情"""
    res = await db.execute(select(NodeProbeResult).where(NodeProbeResult.node_key == node_key))
    item = res.scalar_one_or_none()
    if not item:
        return None
    return serialize_probe_detail_item(item)


async def get_paged_db_probe_summary(
    db: AsyncSession,
    cursor: str | None = None,
    limit: int = 100,
) -> dict[str, Any]:
    """从数据库按游标与分页限额加载节点轻量级摘要，确保 UTF-8 字节不超过 51,200 字节"""
    stmt = select(NodeProbeResult).order_by(NodeProbeResult.node_key.asc())
    if cursor:
        stmt = stmt.where(NodeProbeResult.node_key > cursor)
    stmt = stmt.limit(limit + 1)
    res = await db.execute(stmt)
    candidates = list(res.scalars().all())

    results_map: dict[str, dict[str, Any]] = {}
    last_key: str | None = None
    has_more = False

    for item in candidates:
        if len(results_map) >= limit:
            has_more = True
            break
        summary = serialize_probe_summary_item(item)
        candidate_map = {**results_map, item.node_key: summary}
        candidate_envelope = {
            "results": candidate_map,
            "next_cursor": item.node_key,
            "has_more": True,
        }
        candidate_bytes = len(json.dumps(candidate_envelope, ensure_ascii=False).encode("utf-8"))
        if candidate_bytes > MAX_SUMMARY_PAYLOAD_BYTES:
            if not results_map:
                from fastapi import HTTPException
                raise HTTPException(
                    status_code=500,
                    detail="Single probe summary exceeds maximum payload budget",
                )
            has_more = True
            break
        results_map = candidate_map
        last_key = item.node_key

    if not has_more:
        next_cursor = None
    else:
        next_cursor = last_key

    return {
        "results": results_map,
        "next_cursor": next_cursor,
        "has_more": has_more,
    }


def is_node_credential_missing(node: dict[str, Any]) -> bool:
    """Check whether a proxy node dictionary is missing critical credentials or has redacted secrets."""
    if not isinstance(node, dict):
        return False

    for _, v in node.items():
        if isinstance(v, str) and ("[REDACTED]" in v or "[REDACTED_UUID]" in v):
            return True
        if isinstance(v, dict):
            for _, sub_v in v.items():
                if isinstance(sub_v, str) and ("[REDACTED]" in sub_v or "[REDACTED_UUID]" in sub_v):
                    return True

    node_type = str(node.get("type") or "").strip().lower()

    if node_type in ("ss", "shadowsocks"):
        return not bool(node.get("password")) or not bool(node.get("cipher"))

    if node_type == "vmess":
        return not bool(node.get("uuid"))

    if node_type == "vless":
        if not node.get("uuid"):
            return True
        reality = node.get("reality-opts") or node.get("reality_opts")
        if isinstance(reality, dict):
            pub_key = (
                reality.get("public-key")
                or reality.get("public_key")
                or reality.get("publicKey")
                or node.get("public-key")
                or node.get("public_key")
            )
            if not pub_key:
                return True
        return False

    if node_type == "trojan":
        return not bool(node.get("password"))

    if node_type in ("hysteria2", "hy2"):
        return not bool(
            node.get("password")
            or node.get("auth")
            or node.get("token")
            or node.get("auth-str")
            or node.get("auth_str")
        )

    if node_type == "tuic":
        has_uuid = bool(node.get("uuid"))
        has_pwd = bool(node.get("password") or node.get("token"))
        return not (has_uuid or has_pwd)

    if node_type == "wireguard":
        has_priv = bool(node.get("private-key") or node.get("private_key"))
        has_pub = bool(node.get("public-key") or node.get("public_key"))
        return not (has_priv and has_pub)

    return False


def _merge_candidate_with_node(candidate: dict[str, Any], node: dict[str, Any]) -> dict[str, Any]:
    """Merge database candidate configuration into incoming node, restoring credentials."""
    merged = dict(candidate)
    for k, v in node.items():
        if v is None or v == "" or v in ("[REDACTED]", "[REDACTED_UUID]"):
            continue
        if isinstance(v, dict) and isinstance(candidate.get(k), dict):
            sub_merged = dict(candidate[k])
            for sub_k, sub_v in v.items():
                if sub_v not in (None, "", "[REDACTED]", "[REDACTED_UUID]"):
                    sub_merged[sub_k] = sub_v
            merged[k] = sub_merged
        else:
            merged[k] = v
    return merged


class _CandidateIndex:
    """In-memory multi-index for fast candidate node resolution without N+1 queries."""

    def __init__(self) -> None:
        self.by_logical_id: dict[str, dict[str, Any]] = {}
        self.by_fingerprint: dict[str, dict[str, Any]] = {}
        self.by_sub_name: dict[tuple[int, str], dict[str, Any]] = {}
        self.by_sub_server_port: dict[tuple[int, str, int], dict[str, Any]] = {}
        self.by_name_server_port: dict[tuple[str, str, int], dict[str, Any]] = {}
        self.by_name: dict[str, dict[str, Any]] = {}
        self.by_server_port: dict[tuple[str, int], dict[str, Any]] = {}

    def add_candidate(
        self,
        cand: dict[str, Any],
        sub_id: int | None = None,
        logical_id: str | None = None,
        fingerprint: str | None = None,
    ) -> None:
        if not isinstance(cand, dict):
            return
        name = str(cand.get("name") or "").strip()
        server = str(cand.get("server") or "").strip()
        port = cand.get("port")
        port_int = 0
        if port is not None:
            try:
                port_int = int(port)
            except (ValueError, TypeError):
                port_int = 0

        lid = logical_id or cand.get("logical_id") or cand.get("id")
        if lid:
            self.by_logical_id[str(lid)] = cand

        fp = fingerprint or cand.get("payload_fingerprint")
        if fp:
            self.by_fingerprint[str(fp)] = cand

        if sub_id is not None:
            if name:
                self.by_sub_name[(sub_id, name)] = cand
            if server and port_int:
                self.by_sub_server_port[(sub_id, server, port_int)] = cand

        if name and server and port_int:
            self.by_name_server_port[(name, server, port_int)] = cand

        if name and name not in self.by_name:
            self.by_name[name] = cand

        if server and port_int and (server, port_int) not in self.by_server_port:
            self.by_server_port[(server, port_int)] = cand

    def find_match(self, node: dict[str, Any]) -> dict[str, Any] | None:
        lid = node.get("logical_id") or node.get("id")
        if lid and str(lid) in self.by_logical_id:
            return self.by_logical_id[str(lid)]

        fp = node.get("payload_fingerprint")
        if fp and str(fp) in self.by_fingerprint:
            return self.by_fingerprint[str(fp)]

        sub_id = node.get("subscription_id")
        name = str(node.get("name") or "").strip()
        server = str(node.get("server") or "").strip()
        port = node.get("port")
        port_int = 0
        if port is not None:
            try:
                port_int = int(port)
            except (ValueError, TypeError):
                port_int = 0

        if sub_id is not None:
            try:
                sub_id_int = int(sub_id)
                if name and (sub_id_int, name) in self.by_sub_name:
                    return self.by_sub_name[(sub_id_int, name)]
                if server and port_int and (sub_id_int, server, port_int) in self.by_sub_server_port:
                    return self.by_sub_server_port[(sub_id_int, server, port_int)]
            except (ValueError, TypeError):
                pass

        if name and server and port_int and (name, server, port_int) in self.by_name_server_port:
            return self.by_name_server_port[(name, server, port_int)]

        if name and name in self.by_name:
            return self.by_name[name]

        if server and port_int and (server, port_int) in self.by_server_port:
            return self.by_server_port[(server, port_int)]

        return None


async def _do_hydrate_nodes(
    nodes: list[dict[str, Any]],
    missing_indices: list[int],
    session: AsyncSession,
) -> list[dict[str, Any]]:
    index = _CandidateIndex()

    # 1. Query Node table (if available)
    try:
        from app.models.node import Node
        node_stmt = select(Node).where(Node.lifecycle_state == "active")
        res = await session.execute(node_stmt)
        for item in res.scalars().all():
            payload = item.normalized_payload
            if isinstance(payload, dict):
                index.add_candidate(
                    payload,
                    logical_id=item.logical_id,
                    fingerprint=item.payload_fingerprint,
                )
    except Exception as exc:
        logger.debug("Node table not available or error during candidate indexing: %s", exc)

    # 2. Query Subscription table
    try:
        from app.models.subscription import Subscription
        sub_stmt = select(Subscription).where(Subscription.enabled.is_(True))
        sub_res = await session.execute(sub_stmt)
        for sub in sub_res.scalars().all():
            for raw in (sub.raw_nodes or []):
                if isinstance(raw, dict):
                    index.add_candidate(raw, sub_id=sub.id)
            for raw in (sub.manual_nodes or []):
                if isinstance(raw, dict):
                    index.add_candidate(raw, sub_id=sub.id)
            for raw in (sub.source_nodes or []):
                if isinstance(raw, dict):
                    index.add_candidate(raw, sub_id=sub.id)
    except Exception as exc:
        logger.debug("Subscription table not available or error during candidate indexing: %s", exc)

    # 3. Perform hydration for each node with missing credentials
    hydrated_nodes = list(nodes)
    for idx in missing_indices:
        target_node = nodes[idx]
        candidate = index.find_match(target_node)
        if candidate:
            hydrated = _merge_candidate_with_node(candidate, target_node)
            hydrated_nodes[idx] = hydrated
            logger.info(
                "Successfully hydrated credentials for node '%s' (%s:%s)",
                target_node.get("name"),
                target_node.get("server"),
                target_node.get("port"),
            )
        else:
            logger.warning(
                "Could not find matching candidate in database to hydrate node '%s' (%s:%s)",
                target_node.get("name"),
                target_node.get("server"),
                target_node.get("port"),
            )

    return hydrated_nodes


async def hydrate_nodes_batch(
    nodes: list[dict[str, Any]],
    db: AsyncSession | None = None,
) -> list[dict[str, Any]]:
    """Batch hydrate missing connection credentials for nodes from local storage.

    Inspects each node in the list. If any node lacks required secrets (uuid,
    password, reality-opts, etc.), retrieves full configurations from Subscription
    and Node tables in a single batch query, avoiding N+1 lookups.
    """
    if not nodes:
        return []

    missing_indices = [i for i, n in enumerate(nodes) if is_node_credential_missing(n)]
    if not missing_indices:
        return nodes

    if db is not None:
        try:
            return await _do_hydrate_nodes(nodes, missing_indices, db)
        except Exception as exc:
            logger.warning("Error during batch credential hydration with provided db session: %s", exc)
            return nodes

    try:
        from app.database import AsyncSessionLocal
        async with AsyncSessionLocal() as session:
            return await _do_hydrate_nodes(nodes, missing_indices, session)
    except Exception as exc:
        logger.warning("Error opening session for batch credential hydration: %s", exc)
        return nodes


async def probe_single_node(
    node: dict[str, Any],
    *,
    resolved_config: ResolvedProbeConfig | None = None,
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
    if resolved_config is None:
        node_budget_s = (probe_timeout_ms / 1000.0) if (probe_timeout_ms and probe_timeout_ms > 0) else None
        resolved_config = ResolvedProbeConfig(
            probe_enabled=probe_enabled,
            speedtest_enabled=speedtest_enabled,
            speedtest_url=speedtest_url,
            speedtest_max_bytes=speedtest_max_bytes,
            speedtest_timeout_s=speedtest_timeout_s,
            media_check_enabled=media_check_enabled,
            media_platforms=tuple(media_platforms if media_platforms is not None else ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"]),
            service_timeout_s=2.0,
            node_timeout_s=node_budget_s,
        )

    if is_node_credential_missing(node):
        hydrated_nodes = await hydrate_nodes_batch([node], db=db)
        if hydrated_nodes:
            node = hydrated_nodes[0]

    if use_cache:
        cached = get_cached_result(node)
        if cached:
            # 如果请求要求测速但缓存没有测速数据，则继续执行
            if not (resolved_config.speedtest_enabled and cached.get("speed_mbps") is None):
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

    if not resolved_config.probe_enabled:
        base_result["status"] = "skipped"
        return base_result

    async def _execute_probe() -> None:
        node_started = time.monotonic()
        node_deadline = (
            node_started + resolved_config.node_timeout_s
            if resolved_config.node_timeout_s is not None and resolved_config.node_timeout_s > 0
            else None
        )
        try:
            import inspect
            runner_kwargs: dict[str, Any] = {}
            try:
                sig = inspect.signature(spawn_node_runner)
                if "service_timeout_s" in sig.parameters or any(p.kind == inspect.Parameter.VAR_KEYWORD for p in sig.parameters.values()):
                    runner_kwargs["service_timeout_s"] = resolved_config.service_timeout_s
            except (ValueError, TypeError):
                runner_kwargs["service_timeout_s"] = resolved_config.service_timeout_s

            async with spawn_node_runner(node, **runner_kwargs) as runner_ctx:
                proxy_url = runner_ctx["proxy_url"]

                # 1. 基础握手与 204 往返延迟（独立 service_timeout_s）
                transport = await check_transport(proxy_url, timeout_s=resolved_config.service_timeout_s)
                base_result["latency_ms"] = transport.get("latency_ms")
                if transport.get("status") == "timeout":
                    base_result["status"] = "timeout"
                    base_result["error"] = transport.get("error") or "Transport timed out"
                    return
                elif transport.get("status") != "ok":
                    base_result["status"] = "fail"
                    base_result["error"] = transport.get("error") or "Transport 204 failed"
                    return

                base_result["status"] = "ok"

                # 2. 真实出口 IP 与地理位置共识（独立 service_timeout_s）
                geo = await check_geo_identity(proxy_url, timeout_s=resolved_config.service_timeout_s)
                base_result["ip"] = geo.get("ip") or None
                base_result["country"] = geo.get("country") or None
                base_result["asn"] = geo.get("asn")
                base_result["organization"] = geo.get("organization") or None
                if geo.get("identity_evidence"):
                    base_result["identity_evidence"] = geo.get("identity_evidence")
                if geo.get("confidence"):
                    base_result["identity_confidence"] = geo.get("confidence")

                # 3. 流媒体与 AI 解锁探测（单会话多路复用，受 media_timeout_s 与剩余 node_timeout_s 约束）
                if resolved_config.media_check_enabled and resolved_config.media_platforms:
                    configured_media_timeout = float(
                        getattr(resolved_config, "media_timeout_s", None)
                        or resolved_config.service_timeout_s
                    )
                    if node_deadline is not None:
                        remaining_node_budget = max(0.01, node_deadline - time.monotonic())
                        effective_media_timeout = min(configured_media_timeout, remaining_node_budget)
                    else:
                        effective_media_timeout = configured_media_timeout

                    media_res = await check_media_unlock(
                        proxy_url,
                        platforms=list(resolved_config.media_platforms),
                        timeout_s=effective_media_timeout,
                    )
                    base_result["media"] = media_res

                # 4. 可选带宽测速（受 min(speedtest_timeout_s, service_timeout_s) 约束）
                if resolved_config.speedtest_enabled:
                    effective_speed_timeout = min(
                        resolved_config.speedtest_timeout_s,
                        resolved_config.service_timeout_s,
                    )
                    speed_res = await check_download_speed(
                        proxy_url,
                        speedtest_url=resolved_config.speedtest_url,
                        max_bytes=resolved_config.speedtest_max_bytes,
                        timeout_s=effective_speed_timeout,
                    )
                    base_result["speed_mbps"] = speed_res.get("speed_mbps")

        except (asyncio.TimeoutError, TimeoutError) as exc:
            base_result["status"] = "timeout"
            base_result["error"] = str(exc)
        except Exception as exc:
            base_result["status"] = "fail"
            base_result["error"] = str(exc)

    try:
        if resolved_config.node_timeout_s is not None and resolved_config.node_timeout_s > 0:
            try:
                await asyncio.wait_for(_execute_probe(), timeout=resolved_config.node_timeout_s)
            except (asyncio.TimeoutError, TimeoutError):
                base_result["status"] = "timeout"
                base_result["error"] = f"Node probe exceeded total budget of {int(resolved_config.node_timeout_s * 1000)}ms"
        else:
            await _execute_probe()
    except asyncio.CancelledError:
        base_result["status"] = "timeout"
        base_result["error"] = "Node probe cancelled"
        raise
    finally:
        set_cached_result(node, base_result)
        if db:
            try:
                await save_probe_result_to_db(db, base_result)
            except Exception as exc:
                logger.debug("Failed saving single probe result to db: %s", exc)

    return base_result


async def probe_batch_nodes(
    nodes: list[dict[str, Any]],
    *,
    resolved_config: ResolvedProbeConfig | None = None,
    probe_enabled: bool = True,
    speedtest_enabled: bool = False,
    media_check_enabled: bool = True,
    media_platforms: list[str] | None = None,
    speedtest_url: str = "https://speed.cloudflare.com/__down?bytes=5000000",
    speedtest_max_bytes: int = 5242880,
    speedtest_timeout_s: float = 5.0,
    probe_timeout_ms: int = 4000,
    concurrency: int = 10,
    use_cache: bool = True,
    db: AsyncSession | None = None,
) -> dict[str, Any]:
    """批量并发探测节点能力"""
    if resolved_config is None:
        node_budget_s = (probe_timeout_ms / 1000.0) if (probe_timeout_ms and probe_timeout_ms > 0) else None
        resolved_config = ResolvedProbeConfig(
            probe_enabled=probe_enabled,
            speedtest_enabled=speedtest_enabled,
            speedtest_url=speedtest_url,
            speedtest_max_bytes=speedtest_max_bytes,
            speedtest_timeout_s=speedtest_timeout_s,
            media_check_enabled=media_check_enabled,
            media_platforms=tuple(media_platforms if media_platforms is not None else ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"]),
            concurrency=concurrency,
            service_timeout_s=2.0,
            node_timeout_s=node_budget_s,
        )

    # 批量凭据回填：在并发执行前单次完成数据库检索，避免循环 N+1 查询与并发 session 争用
    hydrated_nodes = await hydrate_nodes_batch(nodes, db=db)

    sem = asyncio.Semaphore(max(1, min(resolved_config.concurrency, 20)))

    async def _worker(n: dict[str, Any]) -> dict[str, Any]:
        async with sem:
            return await probe_single_node(
                n,
                resolved_config=resolved_config,
                use_cache=use_cache,
                db=None,
            )

    results = await asyncio.gather(*[_worker(n) for n in hydrated_nodes], return_exceptions=False)

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
