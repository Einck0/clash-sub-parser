from pydantic import BaseModel


class GenerateConfigRead(BaseModel):
    enabled: bool = True
    subscriptions: bool = True
    node_groups: bool = True
    rules: bool = True
    dns: bool = True
    exclude_node_proxies: bool = True

    model_config = {"from_attributes": True}


class GenerateConfigUpdate(BaseModel):
    enabled: bool | None = None
    subscriptions: bool | None = None
    node_groups: bool | None = None
    rules: bool | None = None
    dns: bool | None = None
    exclude_node_proxies: bool | None = None
