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
    probe_concurrency: Mapped[int] = mapped_column(Integer, default=10, nullable=False)
    probe_service_timeout_ms: Mapped[int] = mapped_column(Integer, default=2000, nullable=False)
    probe_timeout_ms: Mapped[int] = mapped_column(Integer, default=3000, nullable=False)
    probe_cron_enabled: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    probe_cron_interval_minutes: Mapped[int] = mapped_column(Integer, default=60, nullable=False)

    def __init__(self, **kwargs):
        kwargs.setdefault("id", 1)
        kwargs.setdefault("probe_enabled", True)
        kwargs.setdefault("probe_interval_minutes", 0)
        kwargs.setdefault("speedtest_enabled", False)
        kwargs.setdefault("speedtest_url", "https://speed.cloudflare.com/__down?bytes=5000000")
        kwargs.setdefault("speedtest_timeout_s", 5)
        kwargs.setdefault("speedtest_max_bytes", 5242880)
        kwargs.setdefault("speedtest_min_speed_mbps", 0.0)
        kwargs.setdefault("media_check_enabled", True)
        kwargs.setdefault("media_platforms", ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"])
        kwargs.setdefault("media_timeout_s", 5)
        kwargs.setdefault("probe_concurrency", 10)
        kwargs.setdefault("probe_service_timeout_ms", 2000)
        kwargs.setdefault("probe_timeout_ms", 3000)
        kwargs.setdefault("probe_cron_enabled", True)
        kwargs.setdefault("probe_cron_interval_minutes", 60)
        super().__init__(**kwargs)
