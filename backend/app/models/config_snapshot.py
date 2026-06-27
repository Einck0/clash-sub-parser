from datetime import datetime, timezone
from sqlalchemy import Column, Integer, String, Text, DateTime
from app.database import Base

class ConfigSnapshot(Base):
    __tablename__ = "config_snapshots"
    id = Column(Integer, primary_key=True, index=True)
    label = Column(String(200), default="")
    description = Column(String(500), default="")
    snapshot_data = Column(Text, nullable=False)  # JSON string of exported config
    created_at = Column(DateTime, default=lambda: datetime.now(timezone.utc))
