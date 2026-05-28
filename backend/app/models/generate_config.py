from sqlalchemy import Boolean, Integer
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class GenerateConfig(Base):
    __tablename__ = "generate_config"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, default=1)
    enabled: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    subscriptions: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    node_groups: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    rules: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    dns: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    exclude_node_proxies: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
