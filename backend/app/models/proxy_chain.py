from sqlalchemy import Boolean, Integer, String, Text
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class ProxyChainBinding(Base):
    """前置代理链绑定规则"""

    __tablename__ = "proxy_chain_bindings"

    id: Mapped[int] = mapped_column(primary_key=True, index=True)
    # subscription 或 node_group 或 node
    target_type: Mapped[str] = mapped_column(String(20), nullable=False, index=True)
    # subscription_id 或 node_group_id，target_type为node时为空
    target_id: Mapped[int | None] = mapped_column(Integer, nullable=True, index=True)
    # target_type为node时为节点名，其余为展示缓存
    target_name: Mapped[str | None] = mapped_column(String(255), nullable=True)
    # node 或 node_group
    dialer_type: Mapped[str] = mapped_column(String(20), nullable=False)
    # 前置节点名或策略组名
    dialer_ref: Mapped[str] = mapped_column(String(255), nullable=False)
    enabled: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    sort_order: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    note: Mapped[str | None] = mapped_column(Text, nullable=True)
