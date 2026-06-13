from datetime import datetime

from sqlalchemy import JSON, Boolean, DateTime, Integer, String, Text
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class Subscription(Base):
    __tablename__ = "subscriptions"

    id: Mapped[int] = mapped_column(primary_key=True, index=True)
    name: Mapped[str] = mapped_column(String(120), unique=True, nullable=False)
    url: Mapped[str] = mapped_column(Text, nullable=False)
    update_interval: Mapped[int | None] = mapped_column(Integer, nullable=True)
    is_primary: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False, index=True)
    node_prefix: Mapped[str | None] = mapped_column(String(120), nullable=True)
    filter_regex: Mapped[list[str]] = mapped_column(JSON, default=list, nullable=False)
    include_node_names: Mapped[list[str]] = mapped_column(JSON, default=list, nullable=False)
    exclude_node_names: Mapped[list[str]] = mapped_column(JSON, default=list, nullable=False)
    source_nodes: Mapped[list[dict]] = mapped_column(JSON, default=list, nullable=False)
    manual_nodes: Mapped[list[dict]] = mapped_column(JSON, default=list, nullable=False)
    raw_nodes: Mapped[list[dict]] = mapped_column(JSON, default=list, nullable=False)
    last_fetched_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    last_fetch_error: Mapped[str | None] = mapped_column(Text, nullable=True)
    fetch_failed_count: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    fetch_comments: Mapped[list[str]] = mapped_column(
        JSON, default=list, nullable=False
    )
    subscription_userinfo: Mapped[str | None] = mapped_column(Text, nullable=True)
    profile_update_interval: Mapped[str | None] = mapped_column(String(40), nullable=True)
    profile_web_page_url: Mapped[str | None] = mapped_column(Text, nullable=True)
