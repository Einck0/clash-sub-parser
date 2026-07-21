from sqlalchemy import Boolean, Integer, String, Text
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class ProxyChainBinding(Base):
    """Post-process dialer binding: target gets dialer-proxy after nodes/groups settle."""

    __tablename__ = "proxy_chain_bindings"

    id: Mapped[int] = mapped_column(primary_key=True, index=True)
    # subscription | node_group | node
    target_type: Mapped[str] = mapped_column(String(20), nullable=False, index=True)
    # subscription_id / node_group_id; null when target_type=node
    target_id: Mapped[int | None] = mapped_column(Integer, nullable=True, index=True)
    # final node name when target_type=node; optional display cache otherwise
    target_name: Mapped[str | None] = mapped_column(String(255), nullable=True)
    # node | node_group
    dialer_type: Mapped[str] = mapped_column(String(20), nullable=False)
    # final node name or strategy group name
    dialer_ref: Mapped[str] = mapped_column(String(255), nullable=False)
    enabled: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    sort_order: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    note: Mapped[str | None] = mapped_column(Text, nullable=True)
