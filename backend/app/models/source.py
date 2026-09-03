from __future__ import annotations

from datetime import datetime
from typing import TYPE_CHECKING

from sqlalchemy import Boolean, DateTime, ForeignKey, Integer, String, Text
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.database import Base

if TYPE_CHECKING:
    from app.models.node import Node


class Source(Base):
    __tablename__ = "sources"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    logical_id: Mapped[str] = mapped_column(String(36), unique=True, nullable=False, index=True)
    name: Mapped[str] = mapped_column(String(120), nullable=False)
    kind: Mapped[str] = mapped_column(String(32), nullable=False)
    url: Mapped[str | None] = mapped_column(Text, nullable=True)
    update_interval: Mapped[int | None] = mapped_column(Integer, nullable=True)
    enabled: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False)

    revisions: Mapped[list[SourceRevision]] = relationship(
        back_populates="source",
        cascade="all, delete-orphan",
        order_by="desc(SourceRevision.created_at)",
    )


class SourceRevision(Base):
    __tablename__ = "source_revisions"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    source_id: Mapped[int] = mapped_column(Integer, ForeignKey("sources.id", ondelete="CASCADE"), nullable=False, index=True)
    revision_id: Mapped[str] = mapped_column(String(36), unique=True, nullable=False, index=True)
    status: Mapped[str] = mapped_column(String(32), nullable=False)
    payload_hash: Mapped[str] = mapped_column(String(64), nullable=False)
    node_count: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    error_summary: Mapped[str | None] = mapped_column(Text, nullable=True)
    fetched_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)

    source: Mapped[Source] = relationship(back_populates="revisions")
    node_links: Mapped[list[NodeSourceLink]] = relationship(
        back_populates="source_revision",
        cascade="all, delete-orphan",
        order_by="NodeSourceLink.original_order",
    )


class NodeSourceLink(Base):
    __tablename__ = "node_source_links"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    source_revision_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("source_revisions.id", ondelete="CASCADE"), nullable=False, index=True
    )
    node_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("nodes.id", ondelete="CASCADE"), nullable=False, index=True
    )
    display_name: Mapped[str] = mapped_column(String(255), nullable=False)
    original_order: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)

    source_revision: Mapped[SourceRevision] = relationship(back_populates="node_links")
    node: Mapped[Node] = relationship(back_populates="source_links")
