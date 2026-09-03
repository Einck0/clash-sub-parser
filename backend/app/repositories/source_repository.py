from __future__ import annotations

from datetime import datetime
import uuid
from typing import Literal

from sqlalchemy import select, update
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.models.source import Source, SourceRevision


class SourceRepository:
    """源与源修订仓储管理"""

    @staticmethod
    async def create_source(
        session: AsyncSession,
        name: str,
        kind: Literal["subscription", "manual"] = "subscription",
        url: str | None = None,
        update_interval: int | None = None,
        enabled: bool = True,
        logical_id: str | None = None,
    ) -> Source:
        source = Source(
            logical_id=logical_id or uuid.uuid4().hex,
            name=name,
            kind=kind,
            url=url,
            update_interval=update_interval,
            enabled=enabled,
            created_at=datetime.utcnow(),
            updated_at=datetime.utcnow(),
        )
        session.add(source)
        await session.flush()
        return source

    @staticmethod
    async def get_by_logical_id(
        session: AsyncSession,
        logical_id: str,
        include_revisions: bool = False,
    ) -> Source | None:
        stmt = select(Source).where(Source.logical_id == logical_id)
        if include_revisions:
            stmt = stmt.options(selectinload(Source.revisions))
        result = await session.execute(stmt)
        return result.scalar_one_or_none()

    @staticmethod
    async def get_by_name(
        session: AsyncSession,
        name: str,
    ) -> Source | None:
        stmt = select(Source).where(Source.name == name)
        result = await session.execute(stmt)
        return result.scalar_one_or_none()

    @staticmethod
    async def list_sources(
        session: AsyncSession,
        kind: str | None = None,
        enabled_only: bool = False,
    ) -> list[Source]:
        stmt = select(Source).order_by(Source.name)
        if kind:
            stmt = stmt.where(Source.kind == kind)
        if enabled_only:
            stmt = stmt.where(Source.enabled.is_(True))
        result = await session.execute(stmt)
        return list(result.scalars().all())

    @staticmethod
    async def update_source(
        session: AsyncSession,
        logical_id: str,
        name: str | None = None,
        url: str | None = None,
        update_interval: int | None = None,
        enabled: bool | None = None,
    ) -> Source | None:
        source = await SourceRepository.get_by_logical_id(session, logical_id)
        if not source:
            return None
        if name is not None:
            source.name = name
        if url is not None:
            source.url = url
        if update_interval is not None:
            source.update_interval = update_interval
        if enabled is not None:
            source.enabled = enabled
        source.updated_at = datetime.utcnow()
        await session.flush()
        return source

    @staticmethod
    async def delete_by_logical_id(
        session: AsyncSession,
        logical_id: str,
    ) -> bool:
        source = await SourceRepository.get_by_logical_id(session, logical_id)
        if not source:
            return False
        await session.delete(source)
        await session.flush()
        return True

    @staticmethod
    async def create_revision(
        session: AsyncSession,
        source_id: int,
        status: str,
        payload_hash: str,
        node_count: int,
        revision_id: str | None = None,
        error_summary: str | None = None,
        fetched_at: datetime | None = None,
    ) -> SourceRevision:
        revision = SourceRevision(
            source_id=source_id,
            revision_id=revision_id or uuid.uuid4().hex,
            status=status,
            payload_hash=payload_hash,
            node_count=node_count,
            error_summary=error_summary,
            fetched_at=fetched_at or datetime.utcnow(),
            created_at=datetime.utcnow(),
        )
        session.add(revision)
        await session.flush()
        return revision

    @staticmethod
    async def get_active_revision(
        session: AsyncSession,
        source_id: int,
    ) -> SourceRevision | None:
        stmt = (
            select(SourceRevision)
            .where(
                SourceRevision.source_id == source_id,
                SourceRevision.status == "active",
            )
            .order_by(SourceRevision.created_at.desc())
            .limit(1)
        )
        result = await session.execute(stmt)
        return result.scalar_one_or_none()

    @staticmethod
    async def get_revision_by_id(
        session: AsyncSession,
        revision_id: str,
        include_links: bool = False,
    ) -> SourceRevision | None:
        stmt = select(SourceRevision).where(SourceRevision.revision_id == revision_id)
        if include_links:
            stmt = stmt.options(selectinload(SourceRevision.node_links))
        result = await session.execute(stmt)
        return result.scalar_one_or_none()

    @staticmethod
    async def set_active_revision(
        session: AsyncSession,
        source_id: int,
        new_revision_pk: int,
    ) -> None:
        # 将旧的 active 修订置为 superseded
        await session.execute(
            update(SourceRevision)
            .where(
                SourceRevision.source_id == source_id,
                SourceRevision.status == "active",
                SourceRevision.id != new_revision_pk,
            )
            .values(status="superseded")
        )
        # 将新修订置为 active
        await session.execute(
            update(SourceRevision)
            .where(SourceRevision.id == new_revision_pk)
            .values(status="active")
        )
        await session.flush()
