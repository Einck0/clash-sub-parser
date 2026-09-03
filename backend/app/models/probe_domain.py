from __future__ import annotations

from datetime import datetime
from typing import TYPE_CHECKING

from sqlalchemy import DateTime, Float, ForeignKey, Integer, JSON, String, Text
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.database import Base

if TYPE_CHECKING:
    from app.models.node import Node


class ProbeProfile(Base):
    __tablename__ = "probe_profiles"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    logical_id: Mapped[str] = mapped_column(String(36), unique=True, nullable=False, index=True)
    name: Mapped[str] = mapped_column(String(120), unique=True, nullable=False)
    platforms_json: Mapped[list[str]] = mapped_column(JSON, default=list, nullable=False)
    timeout_s: Mapped[int] = mapped_column(Integer, default=10, nullable=False)
    concurrency: Mapped[int] = mapped_column(Integer, default=5, nullable=False)
    max_bytes: Mapped[int] = mapped_column(Integer, default=50000000, nullable=False)
    min_speed_mbps: Mapped[float | None] = mapped_column(Float, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False)

    jobs: Mapped[list[ProbeJob]] = relationship(
        back_populates="profile",
        cascade="all, delete-orphan",
        order_by="desc(ProbeJob.queued_at)",
    )


class ProbeJob(Base):
    __tablename__ = "probe_jobs"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    logical_id: Mapped[str] = mapped_column(String(36), unique=True, nullable=False, index=True)
    profile_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("probe_profiles.id", ondelete="CASCADE"), nullable=False, index=True
    )
    trigger_type: Mapped[str] = mapped_column(String(32), nullable=False)
    status: Mapped[str] = mapped_column(String(32), nullable=False)
    queued_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    started_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    finished_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    summary_json: Mapped[dict | None] = mapped_column(JSON, nullable=True)

    profile: Mapped[ProbeProfile] = relationship(back_populates="jobs")
    observations: Mapped[list[ProbeObservation]] = relationship(
        back_populates="job",
        cascade="all, delete-orphan",
    )


class ProbeObservation(Base):
    __tablename__ = "probe_observations"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    job_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("probe_jobs.id", ondelete="CASCADE"), nullable=False, index=True
    )
    node_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("nodes.id", ondelete="CASCADE"), nullable=False, index=True
    )
    status: Mapped[str] = mapped_column(String(32), nullable=False)
    latency_ms: Mapped[int | None] = mapped_column(Integer, nullable=True)
    speed_mbps: Mapped[float | None] = mapped_column(Float, nullable=True)
    egress_ip: Mapped[str | None] = mapped_column(String(128), nullable=True)
    country: Mapped[str | None] = mapped_column(String(32), nullable=True)
    asn: Mapped[int | None] = mapped_column(Integer, nullable=True)
    organization: Mapped[str | None] = mapped_column(String(255), nullable=True)
    media_results_json: Mapped[dict] = mapped_column(JSON, default=dict, nullable=False)
    error: Mapped[str | None] = mapped_column(Text, nullable=True)
    observed_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False, index=True)

    job: Mapped[ProbeJob] = relationship(back_populates="observations")
    node: Mapped[Node] = relationship(back_populates="observations")
