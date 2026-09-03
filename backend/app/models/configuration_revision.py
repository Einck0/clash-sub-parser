from __future__ import annotations

from datetime import datetime

from sqlalchemy import Boolean, DateTime, Integer, JSON, String
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class ConfigurationRevision(Base):
    __tablename__ = "configuration_revisions"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    logical_id: Mapped[str] = mapped_column(String(36), unique=True, nullable=False, index=True)
    schema_version: Mapped[int] = mapped_column(Integer, default=1, nullable=False)
    bundle_json: Mapped[dict] = mapped_column(JSON, nullable=False)
    checksum: Mapped[str] = mapped_column(String(64), nullable=False)
    is_active: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False, index=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    created_by: Mapped[str] = mapped_column(String(100), default="system", nullable=False)
