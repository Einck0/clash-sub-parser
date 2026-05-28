from sqlalchemy import Boolean, Integer, String
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class SecuritySettings(Base):
    __tablename__ = "security_settings"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, default=1)
    auth_enabled: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    protect_frontend: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    protect_api: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    protect_exports: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    token_hash: Mapped[str] = mapped_column(String(128), default="", nullable=False)
    fetch_proxy_enabled: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    fetch_proxy_url: Mapped[str] = mapped_column(String(512), default="", nullable=False)
