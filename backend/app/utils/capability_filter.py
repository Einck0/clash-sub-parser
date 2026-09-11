"""节点探测结果过滤与能力匹配工具"""
from __future__ import annotations

from typing import Any, Mapping

from app.services.node_identity import canonical_node_key


DISQUALIFIED_VERDICTS = {
    "originals_only",
    "unsupported_region",
    "blocked",
    "challenge",
    "rate_limited",
    "unknown",
}

DISQUALIFIED_STATUSES = {
    "partial",
    "originals",
    "originals_only",
    "restricted",
    "ip_blocked",
    "challenged",
    "rate_limited",
    "timeout",
    "transport_error",
    "inconclusive",
    "disabled",
    "fail",
    "failed",
    "blocked",
    "unknown",
}

DISQUALIFIED_CONFIDENCES = {
    "conflicted",
    "unavailable",
}


def get_probe_result_for_node(
    probe_map: Mapping[str, dict[str, Any]], node: Mapping[str, Any]
) -> dict[str, Any] | None:
    """按节点完整身份读取探测结果，不使用显示名别名"""
    return probe_map.get(canonical_node_key(node))


def deduplicate_nodes_by_key(nodes: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """按规范节点身份去重，保留同名但身份不同的节点"""
    seen: set[str] = set()
    result: list[dict[str, Any]] = []
    for node in nodes:
        if not isinstance(node, dict) or not str(node.get("name") or "").strip():
            continue
        key = canonical_node_key(node)
        if key in seen:
            continue
        seen.add(key)
        result.append(node)
    return result


def is_media_full_unlocked(item: Any) -> bool:
    """Check whether a platform probe outcome satisfies the full-unlock requirement.

    Conforms to OpenSpec evidence-grade capability probing:
    - Verified full or generic available passes.
    - Historical accepted full and generic ok values pass until superseded.
    - Netflix partial/originals_only, restricted, ip_blocked, challenged,
      rate_limited, timeout, transport_error, and inconclusive never pass.
    - Conflicted or unavailable confidence never passes.
    """
    if not isinstance(item, dict):
        return bool(item is True)

    status = str(item.get("status") or "").lower().strip()
    verdict = str(item.get("verdict") or "").lower().strip()
    confidence = str(item.get("confidence") or "").lower().strip()
    observation_kind = str(item.get("observation_kind") or "").lower().strip()
    unlocked = item.get("unlocked")

    if observation_kind == "region_signal":
        return False
    if verdict in DISQUALIFIED_VERDICTS:
        return False
    if status in DISQUALIFIED_STATUSES:
        return False
    if confidence in DISQUALIFIED_CONFIDENCES:
        return False
    if status == "verified" and verdict in ("full", "available"):
        return True
    if status in ("full", "ok"):
        return True
    if unlocked is True and (verdict in ("full", "available") or not verdict):
        return True
    return False


def is_node_capability_qualified(
    probe_data: dict[str, Any] | None,
    *,
    min_speed_mbps: float | None = None,
    required_media: list[str] | None = None,
) -> bool:
    """检查单个节点的探测结果是否满足测速门槛和流媒体解锁需求"""
    has_speed_req = min_speed_mbps is not None and min_speed_mbps > 0
    has_media_req = bool(required_media)
    if not has_speed_req and not has_media_req:
        return True
    if not probe_data or probe_data.get("status") != "ok":
        return False
    if has_speed_req and min_speed_mbps is not None:
        node_speed = probe_data.get("speed_mbps")
        if node_speed is None or float(node_speed) < float(min_speed_mbps):
            return False
    if has_media_req:
        media_map = probe_data.get("media") or {}
        for platform in required_media or []:
            plat_key = platform.lower().strip()
            item = media_map.get(plat_key)
            if item is None and platform in media_map:
                item = media_map.get(platform)
            if not is_media_full_unlocked(item):
                return False
    return True


def filter_nodes_by_capabilities(
    nodes: list[dict[str, Any]],
    probe_map: dict[str, dict[str, Any]],
    *,
    min_speed_mbps: float | None = None,
    required_media: list[str] | None = None,
) -> list[dict[str, Any]]:
    """根据能力对节点列表进行过滤"""
    has_speed_req = min_speed_mbps is not None and min_speed_mbps > 0
    has_media_req = bool(required_media)
    if not has_speed_req and not has_media_req:
        return nodes
    return [
        node
        for node in nodes
        if is_node_capability_qualified(
            get_probe_result_for_node(probe_map, node),
            min_speed_mbps=min_speed_mbps,
            required_media=required_media,
        )
    ]
