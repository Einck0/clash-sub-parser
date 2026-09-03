from sqlalchemy import Float, Integer, JSON, String, Text
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class NodeProbeResult(Base):
    """节点探测与测速持久化结果表"""
    __tablename__ = "node_probe_results"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    node_key: Mapped[str] = mapped_column(String(255), unique=True, nullable=False, index=True)
    name: Mapped[str] = mapped_column(String(255), nullable=False, index=True)
    server: Mapped[str] = mapped_column(String(255), nullable=False, index=True)
    port: Mapped[int | None] = mapped_column(Integer, nullable=True)
    type: Mapped[str] = mapped_column(String(64), nullable=False)
    status: Mapped[str] = mapped_column(String(32), nullable=False)
    latency_ms: Mapped[int | None] = mapped_column(Integer, nullable=True)
    speed_mbps: Mapped[float | None] = mapped_column(Float, nullable=True)
    ip: Mapped[str | None] = mapped_column(String(128), nullable=True)
    country: Mapped[str | None] = mapped_column(String(32), nullable=True)
    asn: Mapped[int | None] = mapped_column(Integer, nullable=True)
    organization: Mapped[str | None] = mapped_column(String(255), nullable=True)
    media: Mapped[dict] = mapped_column(JSON, default=dict, nullable=False)
    error: Mapped[str | None] = mapped_column(Text, nullable=True)
    checked_at: Mapped[int] = mapped_column(Integer, nullable=False, index=True)
