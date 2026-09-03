from __future__ import annotations

import hashlib
import json
from typing import Any


def _clean_value(val: Any) -> Any:
    if isinstance(val, dict):
        return {k: _clean_value(v) for k, v in sorted(val.items()) if v is not None}
    if isinstance(val, list):
        return [_clean_value(item) for item in val]
    if isinstance(val, str):
        return val.strip()
    return val


def normalize_node_payload(raw_node: dict[str, Any]) -> dict[str, Any]:
    """将各类代理节点字典规范化为协议无关的确定性结构"""
    if not isinstance(raw_node, dict):
        return {}

    # 提取并规范化基础协议与网络字段
    protocol = str(raw_node.get("type") or raw_node.get("protocol") or "").strip().lower()
    server = str(raw_node.get("server") or raw_node.get("server_name") or "").strip()
    try:
        port = int(raw_node.get("port") or 0)
    except (ValueError, TypeError):
        port = 0

    # 规范化特定字段名
    normalized: dict[str, Any] = {
        "type": protocol,
        "server": server,
        "port": port,
    }

    # 忽略展示与动态临时字段
    ignored_keys = {
        "name",
        "tag",
        "sub_id",
        "source",
        "status",
        "latency",
        "latency_ms",
        "speed",
        "speed_mbps",
        "checked_at",
        "original_order",
    }

    for k, v in sorted(raw_node.items()):
        if k in ignored_keys or v is None:
            continue
        clean_k = k.strip()
        if clean_k in ("type", "server", "port"):
            continue

        # 统一常用协议别名
        if clean_k in ("method", "cipher") and protocol in ("ss", "shadowsocks"):
            normalized["cipher"] = str(v).strip().lower()
        elif clean_k in ("sni", "servername"):
            normalized["sni"] = str(v).strip()
        elif clean_k == "alpn" and isinstance(v, list):
            normalized["alpn"] = sorted([str(x).strip() for x in v if x])
        elif clean_k in ("skip-cert-verify", "skip_cert_verify", "insecure"):
            normalized["skip_cert_verify"] = bool(v)
        elif clean_k in ("udp", "udp_relay"):
            normalized["udp"] = bool(v)
        else:
            normalized[clean_k] = _clean_value(v)

    return {k: v for k, v in sorted(normalized.items())}


def compute_payload_fingerprint(
    normalized_payload: dict[str, Any],
    version: str = "v1",
) -> str:
    """计算规范化 payload 的版本化 SHA-256 安全指纹"""
    serialized = json.dumps(
        normalized_payload,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    digest = hashlib.sha256(serialized.encode("utf-8")).hexdigest()
    return f"{version}:{digest}"


def extract_node_identity(raw_node: dict[str, Any]) -> tuple[str, str, str, int, dict[str, Any], str]:
    """提取节点的展示名称、协议、地址、端口、规范化 payload 与安全指纹"""
    name = str(raw_node.get("name") or raw_node.get("tag") or "Unnamed").strip()
    normalized = normalize_node_payload(raw_node)
    protocol = normalized.get("type", "unknown")
    server = normalized.get("server", "")
    port = normalized.get("port", 0)
    fingerprint = compute_payload_fingerprint(normalized)
    return name, protocol, server, port, normalized, fingerprint
