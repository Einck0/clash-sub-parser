from datetime import datetime
from pydantic import BaseModel, Field

class ConfigSnapshotRead(BaseModel):
    id: int
    label: str
    description: str
    created_at: datetime | None = None
    model_config = {"from_attributes": True}

class ConfigSnapshotCreate(BaseModel):
    label: str = Field(default="", max_length=200)
    description: str = Field(default="", max_length=500)
