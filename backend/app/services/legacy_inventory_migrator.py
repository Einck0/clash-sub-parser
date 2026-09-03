from __future__ import annotations

from datetime import datetime
import hashlib
import json
import logging
from typing import Any

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.quarantine import QuarantineRecord
from app.models.subscription import Subscription
from app.repositories.node_repository import NodeRepository
from app.repositories.source_repository import SourceRepository
from app.services.node_normalizer import extract_node_identity

logger = logging.getLogger(__name__)


def _sanitize_source_url(url: str | None) -> str | None:
    if not url:
        return None
    if url.startswith("manual://"):
        return "manual://nodes"
    # 脱敏 URL 中的敏感参数
    try:
        from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

        parts = urlsplit(url)
        if not parts.query:
            return f"{parts.scheme}://{parts.netloc}{parts.path}"
        params = parse_qsl(parts.query)
        redacted_params = [(k, "***" if any(s in k.lower() for s in ("token", "key", "secret", "auth", "pwd", "password")) else v) for k, v in params]
        return urlunsplit((parts.scheme, parts.netloc, parts.path, urlencode(redacted_params), parts.fragment))
    except Exception:
        return "[redacted-url]"


async def migrate_legacy_subscriptions_to_inventory(session: AsyncSession) -> dict[str, Any]:
    """将遗留 subscriptions 表中的节点与来源迁移至新规范化库存与世代"""
    stmt = select(Subscription).order_by(Subscription.id.asc())
    result = await session.execute(stmt)
    legacy_subs = list(result.scalars().all())

    sources_count = 0
    revisions_count = 0
    nodes_count = 0
    quarantined_count = 0
    category_breakdown: dict[str, int] = {
        "remote_nodes": 0,
        "manual_nodes": 0,
        "quarantined_nodes": 0,
    }

    for sub in legacy_subs:
        kind = "manual" if (sub.url and sub.url.startswith("manual://")) else "subscription"

        # 查找或创建对应 Source 实体
        existing_src = await SourceRepository.get_by_name(session, sub.name)
        if not existing_src:
            src = await SourceRepository.create_source(
                session=session,
                name=sub.name,
                kind=kind,
                url=sub.url,
                update_interval=sub.update_interval,
                enabled=sub.enabled,
            )
            sources_count += 1
        else:
            src = existing_src

        # 搜集需要迁移的节点字典
        raw_list = sub.source_nodes or []
        if not raw_list and sub.raw_nodes:
            raw_list = sub.raw_nodes
        if not raw_list and sub.manual_nodes:
            raw_list = sub.manual_nodes

        manual_list = sub.manual_nodes or []

        combined_nodes: list[tuple[dict[str, Any], str]] = []
        for n in raw_list:
            if isinstance(n, dict):
                combined_nodes.append((n, "remote_nodes"))
        if kind == "subscription" and manual_list and manual_list != raw_list:
            for n in manual_list:
                if isinstance(n, dict):
                    combined_nodes.append((n, "manual_nodes"))

        valid_links: list[tuple[int, str, int]] = []
        order_idx = 0

        for raw_node, category in combined_nodes:
            name, protocol, server, port, normalized, fingerprint = extract_node_identity(raw_node)
            if not server or port <= 0:
                # 记录无法映射的无效节点
                quarantine = QuarantineRecord(
                    category="unmappable_legacy_node",
                    source_table="subscriptions",
                    source_record_id=str(sub.id),
                    payload_json=raw_node,
                    reason=f"遗留节点缺少有效 server 或 port (server='{server}', port={port})",
                    created_at=datetime.utcnow(),
                )
                session.add(quarantine)
                quarantined_count += 1
                category_breakdown["quarantined_nodes"] += 1
                continue

            node, _ = await NodeRepository.upsert_by_fingerprint(
                session=session,
                name=name,
                protocol=protocol,
                server=server,
                port=port,
                normalized_payload=normalized,
                payload_fingerprint=fingerprint,
            )
            valid_links.append((node.id, name, order_idx))
            order_idx += 1
            nodes_count += 1
            category_breakdown[category] += 1

        # 为该源创建 active 状态的 SourceRevision
        payload_hash = hashlib.sha256(
            json.dumps([item[1] for item in valid_links], ensure_ascii=False).encode("utf-8")
        ).hexdigest()

        rev = await SourceRepository.create_revision(
            session=session,
            source_id=src.id,
            status="active",
            payload_hash=payload_hash,
            node_count=len(valid_links),
            fetched_at=sub.last_fetched_at or datetime.utcnow(),
        )
        revisions_count += 1

        if valid_links:
            await NodeRepository.link_nodes_to_revision(session, rev.id, valid_links)

    await session.commit()

    return {
        "sources_migrated": sources_count,
        "revisions_created": revisions_count,
        "nodes_migrated": nodes_count,
        "nodes_quarantined": quarantined_count,
        "categories": category_breakdown,
    }
