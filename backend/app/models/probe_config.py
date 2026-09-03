from sqlalchemy import Boolean, Float, Integer, JSON, String
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class ProbeConfig(Base):
    """节点探测与测速全局配置"""
    __tablename__ = "probe_config"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, default=1)
    probe_enabled: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    probe_interval_minutes: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    speedtest_enabled: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    speedtest_url: Mapped[str] = mapped_column(
        String(512),
        default="https://speed.cloudflare.com/__down?bytes=5000000",
        nullable=False,
    )
    speedtest_timeout_s: Mapped[int] = mapped_column(Integer, default=5, nullable=False)
    speedtest_max_bytes: Mapped[int] = mapped_column(Integer, default=5242880, nullable=False)
    speedtest_min_speed_mbps: Mapped[float] = mapped_column(Float, default=0.0, nullable=False)
    media_check_enabled: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    media_platforms: Mapped[list[str]] = mapped_column(
        JSON,
        default=lambda: ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"],
        nullable=False,
    )
    media_timeout_s: Mapped[int] = mapped_column(Integer, default=5, nullable=False)
    probe_concurrency: Mapped[int] = mapped_column(Integer, default=5, nullable=False)
    probe_timeout_ms: Mapped[int] = mapped_column(Integer, default=3000, nullable=False)
