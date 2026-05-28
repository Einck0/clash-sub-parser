from sqlalchemy import JSON, Boolean, Integer, String
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class NodeGroup(Base):
    __tablename__ = "node_groups"

    id: Mapped[int] = mapped_column(primary_key=True, index=True)
    name: Mapped[str] = mapped_column(String(120), unique=True, nullable=False)
    kind: Mapped[str] = mapped_column(String(20), nullable=False, default="manual")
    group_type: Mapped[str] = mapped_column(
        String(20), nullable=False, default="select"
    )
    sort_order: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    regex_rules: Mapped[list[str]] = mapped_column(JSON, default=list, nullable=False)
    include_nodes: Mapped[list[str]] = mapped_column(JSON, default=list, nullable=False)
    include_group_ids: Mapped[list[int]] = mapped_column(
        JSON, default=list, nullable=False
    )
    include_group_nodes_ids: Mapped[list[int]] = mapped_column(
        JSON, default=list, nullable=False
    )
    include_entries: Mapped[list[dict]] = mapped_column(
        JSON, default=list, nullable=False
    )
    add_fallback: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    exclude_nodes: Mapped[list[str]] = mapped_column(JSON, default=list, nullable=False)
    url_test_config: Mapped[dict] = mapped_column(JSON, default=dict, nullable=False)
    load_balance_config: Mapped[dict] = mapped_column(
        JSON, default=dict, nullable=False
    )
    fallback_config: Mapped[dict] = mapped_column(JSON, default=dict, nullable=False)
