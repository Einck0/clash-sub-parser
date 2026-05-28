from sqlalchemy import Boolean, Text
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class DnsConfig(Base):
    __tablename__ = "dns_config"

    id: Mapped[int] = mapped_column(primary_key=True, default=1)
    raw_yaml: Mapped[str] = mapped_column(Text, default="", nullable=False)
    enabled: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
