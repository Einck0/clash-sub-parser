from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Literal

from app.models.node import Node
from app.models.source import NodeSourceLink, Source


@dataclass
class CompiledNodeItem:
    node_logical_id: str
    display_name: str
    proxy_dict: dict[str, Any]
    source_name: str
    source_logical_id: str
    payload_fingerprint: str
    original_order: int


def compile_deduplicated_nodes(
    entries: list[tuple[Node, NodeSourceLink, Source]],
    mode: Literal["keep_first", "disambiguate_names", "strict_unique"] = "disambiguate_names",
    dedup_fingerprints: bool = True,
) -> list[CompiledNodeItem]:
    """在编译期执行显式去重策略，保留来源信息并处理同名冲突"""
    seen_fingerprints: set[str] = set()
    name_counts: dict[str, int] = {}
    compiled: list[CompiledNodeItem] = []

    for node, link, source in entries:
        # 指纹去重（相同节点 payload 跨源只保留首个）
        if dedup_fingerprints:
            if node.payload_fingerprint in seen_fingerprints:
                continue
            seen_fingerprints.add(node.payload_fingerprint)

        base_name = link.display_name.strip() or node.name.strip()
        count = name_counts.get(base_name, 0) + 1
        name_counts[base_name] = count

        if count > 1:
            if mode == "keep_first":
                # 同名直接丢弃后续
                continue
            elif mode == "disambiguate_names":
                # 同名自动追加编号后缀
                final_name = f"{base_name} ({count})"
            else:
                final_name = base_name
        else:
            final_name = base_name

        # 构建 Clash 目标代理字典
        proxy_dict = dict(node.normalized_payload)
        proxy_dict["name"] = final_name

        compiled.append(
            CompiledNodeItem(
                node_logical_id=node.logical_id,
                display_name=final_name,
                proxy_dict=proxy_dict,
                source_name=source.name,
                source_logical_id=source.logical_id,
                payload_fingerprint=node.payload_fingerprint,
                original_order=link.original_order,
            )
        )

    return compiled
