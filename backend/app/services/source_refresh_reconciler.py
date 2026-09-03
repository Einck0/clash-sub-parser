from __future__ import annotations

from datetime import datetime
import hashlib
import logging
from typing import Any

from sqlalchemy.ext.asyncio import AsyncSession

from app.models.quarantine import QuarantineRecord
from app.repositories.node_repository import NodeRepository
from app.repositories.source_repository import SourceRepository
from app.services.node_normalizer import extract_node_identity
from app.utils.clash_parser import parse_node_links, parse_subscription_content

logger = logging.getLogger(__name__)


async def reconcile_source_refresh(
    session: AsyncSession,
    source_logical_id: str,
    raw_content: str | None = None,
    error: str | None = None,
) -> dict[str, Any]:
    """原子化协调源刷新并在成功时发布新世代，失败时保留既有活动世代"""
    source = await SourceRepository.get_by_logical_id(session, source_logical_id)
    if not source:
        raise ValueError(f"Source not found: {source_logical_id}")

    # 刷新失败时记录失败修订，不影响已有活动修订
    if error or not raw_content:
        err_msg = error or "Empty content received"
        failed_rev = await SourceRepository.create_revision(
            session=session,
            source_id=source.id,
            status="failed",
            payload_hash="",
            node_count=0,
            error_summary=err_msg,
            fetched_at=datetime.utcnow(),
        )
        await session.commit()
        return {
            "success": False,
            "revision_id": failed_rev.revision_id,
            "error": err_msg,
            "node_count": 0,
        }

    # 解析原始内容
    try:
        raw_nodes, _ = parse_subscription_content(raw_content)
    except Exception as exc:
        try:
            raw_nodes = parse_node_links(raw_content)
        except Exception:
            raw_nodes = []
        if not raw_nodes:
            parse_err = f"Failed to parse content: {exc}"
            failed_rev = await SourceRepository.create_revision(
                session=session,
                source_id=source.id,
                status="failed",
                payload_hash="",
                node_count=0,
                error_summary=parse_err,
                fetched_at=datetime.utcnow(),
            )
            await session.commit()
            return {
                "success": False,
                "revision_id": failed_rev.revision_id,
                "error": parse_err,
                "node_count": 0,
            }

    # 计算整体 payload hash
    payload_hash = hashlib.sha256(raw_content.encode("utf-8")).hexdigest()

    # 处理每个节点并提取安全指纹
    valid_links: list[tuple[int, str, int]] = []
    order = 0
    for raw_node in raw_nodes:
        if not isinstance(raw_node, dict):
            continue
        name, protocol, server, port, normalized, fingerprint = extract_node_identity(raw_node)
        if not server or port <= 0:
            # 记录无法映射的节点到隔离区
            quarantine = QuarantineRecord(
                category="unmappable_node",
                source_table="sources",
                source_record_id=source_logical_id,
                payload_json=raw_node,
                reason="Invalid server or port in node payload",
                created_at=datetime.utcnow(),
            )
            session.add(quarantine)
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
        valid_links.append((node.id, name, order))
        order += 1

    # 创建新修订并挂载节点关联
    new_rev = await SourceRepository.create_revision(
        session=session,
        source_id=source.id,
        status="pending",
        payload_hash=payload_hash,
        node_count=len(valid_links),
        fetched_at=datetime.utcnow(),
    )
    await NodeRepository.link_nodes_to_revision(session, new_rev.id, valid_links)

    # 原子发布新世代
    await SourceRepository.set_active_revision(session, source.id, new_rev.id)
    await session.commit()

    return {
        "success": True,
        "revision_id": new_rev.revision_id,
        "error": None,
        "node_count": len(valid_links),
    }
