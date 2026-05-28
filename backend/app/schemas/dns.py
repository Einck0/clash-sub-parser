from pydantic import BaseModel


class DnsConfigRead(BaseModel):
    id: int
    raw_yaml: str
    enabled: bool

    model_config = {"from_attributes": True}


class DnsConfigUpdate(BaseModel):
    raw_yaml: str
    enabled: bool = True
