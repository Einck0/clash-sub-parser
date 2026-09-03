"""节点探测结果过滤与能力匹配工具"""
from __future__ import annotations

from typing import Any


def is_node_capability_qualified(
    probe_data: dict[str, Any] | None,
    *,
    min_speed_mbps: float | None = None,
    required_media: list[str] | None = None,
) -> bool:
    """检查单个节点的探测结果是否满足测速门槛和流媒体解锁需求"""
    # 无任何过滤条件时直接放行
    has_speed_req = min_speed_mbps is not None and min_speed_mbps > 0
    has_media_req = bool(required_media)
    if not has_speed_req and not has_media_req:
        return True

    # 有要求但没有探测数据或探测未通过
    if not probe_data or probe_data.get("status") != "ok":
        return False

    # 1. 测速门槛校验
    if has_speed_req and min_speed_mbps is not None:
        node_speed = probe_data.get("speed_mbps")
        if node_speed is None or float(node_speed) < float(min_speed_mbps):
            return False

    # 2. 流媒体或 AI 解锁校验（多选 AND 逻辑）
    if has_media_req:
        media_map = probe_data.get("media") or {}
        for platform in required_media or []:
            plat_key = platform.lower().strip()
            item = media_map.get(plat_key)
            if not item:
                return False
            # 判断解锁状态：status 为 ok、full、originals 或 unlocked 为 True
            status = item.get("status")
            unlocked = item.get("unlocked")
            if status not in ("ok", "full", "originals") and not unlocked:
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

    filtered: list[dict[str, Any]] = []
    for node in nodes:
        name = str(node.get("name") or "").strip()
        server = str(node.get("server") or "").strip()
        port = str(node.get("port") or "")
        ntype = str(node.get("type") or "").strip()

        # 多重键匹配探针结果
        key_full = f"{name}|{ntype}|{server}:{port}"
        probe_data = probe_map.get(key_full) or probe_map.get(name)

        if is_node_capability_qualified(
            probe_data,
            min_speed_mbps=min_speed_mbps,
            required_media=required_media,
        ):
            filtered.append(node)

    return filtered
