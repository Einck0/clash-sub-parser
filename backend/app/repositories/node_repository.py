from __future__ import annotations

from datetime import datetime
from typing import Any
import uuid

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.models.node import Node
from app.models.source import NodeSourceLink, Source, SourceRevision


class NodeRepository:
    """规范化节点库存与来源关联仓储"""

    @staticmethod
    async def upsert_by_fingerprint(
        session: AsyncSession,
        name: str,
        protocol: str,
        server: str,
        port: int,
        normalized_payload: dict[str, Any],
        payload_fingerprint: str,
        lifecycle_state: str = "active",
        logical_id: str | None = None,
    ) -> tuple[Node, bool]:
        """按指纹匹配或创建节点"""
        stmt = select(Node).where(Node.payload_fingerprint == payload_fingerprint)
        result = await session.execute(stmt)
        existing = result.scalar_one_or_none()

        if existing:
            # 保持已有 logical_id 稳定不变
            existing.name = name
            existing.protocol = protocol
            existing.server = server
            existing.port = port
            existing.normalized_payload = normalized_payload
            existing.lifecycle_state = lifecycle_state
            existing.updated_at = datetime.utcnow()
            await session.flush()
            return existing, False

        node = Node(
            logical_id=logical_id or uuid.uuid4().hex,
            name=name,
            protocol=protocol,
            server=server,
            port=port,
            normalized_payload=normalized_payload,
            payload_fingerprint=payload_fingerprint,
            lifecycle_state=lifecycle_state,
            created_at=datetime.utcnow(),
            updated_at=datetime.utcnow(),
        )
        session.add(node)
        await session.flush()
        return node, True

    @staticmethod
    async def get_by_logical_id(
        session: AsyncSession,
        logical_id: str,
        include_links: bool = False,
    ) -> Node | None:
        stmt = select(Node).where(Node.logical_id == logical_id)
        if include_links:
            stmt = stmt.options(selectinload(Node.source_links))
        result = await session.execute(stmt)
        return result.scalar_one_or_none()

    @staticmethod
    async def get_by_fingerprint(
        session: AsyncSession,
        fingerprint: str,
    ) -> Node | None:
        stmt = select(Node).where(Node.payload_fingerprint == fingerprint)
        result = await session.execute(stmt)
        return result.scalar_one_or_none()

    @staticmethod
    async def list_nodes(
        session: AsyncSession,
        lifecycle_state: str | None = "active",
        protocol: str | None = None,
        limit: int = 100,
        offset: int = 0,
    ) -> list[Node]:
        stmt = select(Node).order_by(Node.id)
        if lifecycle_state:
            stmt = stmt.where(Node.lifecycle_state == lifecycle_state)
        if protocol:
            stmt = stmt.where(Node.protocol == protocol)
        stmt = stmt.limit(limit).offset(offset)
        result = await session.execute(stmt)
        return list(result.scalars().all())

    @staticmethod
    async def count_nodes(
        session: AsyncSession,
        lifecycle_state: str | None = "active",
        protocol: str | None = None,
    ) -> int:
        stmt = select(func.count(Node.id))
        if lifecycle_state:
            stmt = stmt.where(Node.lifecycle_state == lifecycle_state)
        if protocol:
            stmt = stmt.where(Node.protocol == protocol)
        result = await session.execute(stmt)
        return result.scalar_one() or 0

    @staticmethod
    async def link_nodes_to_revision(
        session: AsyncSession,
        source_revision_id: int,
        links: list[tuple[int, str, int]],
    ) -> None:
        """关联节点至源修订"""
        now = datetime.utcnow()
        for node_id, display_name, original_order in links:
            link = NodeSourceLink(
                source_revision_id=source_revision_id,
                node_id=node_id,
                display_name=display_name,
                original_order=original_order,
                created_at=now,
            )
            session.add(link)
        await session.flush()

    @staticmethod
    async def get_nodes_for_revision(
        session: AsyncSession,
        source_revision_id: int,
    ) -> list[tuple[Node, NodeSourceLink]]:
        stmt = (
            select(Node, NodeSourceLink)
            .join(NodeSourceLink, Node.id == NodeSourceLink.node_id)
            .where(NodeSourceLink.source_revision_id == source_revision_id)
            .order_by(NodeSourceLink.original_order)
        )
        result = await session.execute(stmt)
        return [(row[0], row[1]) for row in result.all()]

    @staticmethod
    async def get_active_nodes_with_provenance(
        session: AsyncSession,
    ) -> list[tuple[Node, NodeSourceLink, SourceRevision, Source]]:
        """获取所有已启用源当前活动修订的节点与来源信息"""
        stmt = (
            select(Node, NodeSourceLink, SourceRevision, Source)
            .join(NodeSourceLink, Node.id == NodeSourceLink.node_id)
            .join(SourceRevision, NodeSourceLink.source_revision_id == SourceRevision.id)
            .join(Source, SourceRevision.source_id == Source.id)
            .where(
                Source.enabled.is_(True),
                SourceRevision.status == "active",
                Node.lifecycle_state == "active",
            )
            .order_by(Source.id, NodeSourceLink.original_order)
        )
        result = await session.execute(stmt)
        return [(row[0], row[1], row[2], row[3]) for row in result.all()]
