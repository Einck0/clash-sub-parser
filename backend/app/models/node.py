from __future__ import annotations

from datetime import datetime
from typing import TYPE_CHECKING

from sqlalchemy import DateTime, Integer, JSON, String
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.database import Base

if TYPE_CHECKING:
    from app.models.probe_domain import ProbeObservation
    from app.models.source import NodeSourceLink


class Node(Base):
    __tablename__ = "nodes"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    logical_id: Mapped[str] = mapped_column(String(36), unique=True, nullable=False, index=True)
    name: Mapped[str] = mapped_column(String(255), nullable=False)
    protocol: Mapped[str] = mapped_column(String(64), nullable=False)
    server: Mapped[str] = mapped_column(String(255), nullable=False)
    port: Mapped[int] = mapped_column(Integer, nullable=False)
    normalized_payload: Mapped[dict] = mapped_column(JSON, nullable=False)
    payload_fingerprint: Mapped[str] = mapped_column(String(64), nullable=False, index=True)
    lifecycle_state: Mapped[str] = mapped_column(String(32), default="active", nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False)

    source_links: Mapped[list[NodeSourceLink]] = relationship(
        back_populates="node",
        cascade="all, delete-orphan",
    )
    observations: Mapped[list[ProbeObservation]] = relationship(
        back_populates="node",
        cascade="all, delete-orphan",
        order_by="desc(ProbeObservation.observed_at)",
    )
