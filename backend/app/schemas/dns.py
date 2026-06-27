from pydantic import BaseModel, Field


class DnsConfigRead(BaseModel):
    id: int
    raw_yaml: str
    enabled: bool

    model_config = {"from_attributes": True}


class DnsConfigUpdate(BaseModel):
    raw_yaml: str = Field(default="", max_length=65536)
    enabled: bool = True
