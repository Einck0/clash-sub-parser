from datetime import datetime

from pydantic import BaseModel, Field


class SubscriptionBase(BaseModel):
    name: str
    url: str
    update_interval: int | None = Field(default=None, ge=1)
    is_primary: bool = False
    node_prefix: str | None = None
    filter_regex: list[str] = Field(default_factory=list)
    include_node_names: list[str] = Field(default_factory=list)
    exclude_node_names: list[str] = Field(default_factory=list)
    manual_nodes: list[dict] = Field(default_factory=list)


class SubscriptionCreate(SubscriptionBase):
    manual_node_links: str | None = None


class SubscriptionUpdate(BaseModel):
    name: str | None = None
    url: str | None = None
    update_interval: int | None = Field(default=None, ge=1)
    is_primary: bool | None = None
    node_prefix: str | None = None
    filter_regex: list[str] | None = None
    include_node_names: list[str] | None = None
    exclude_node_names: list[str] | None = None
    manual_nodes: list[dict] | None = None
    manual_node_links: str | None = None


class ManualNodeCreate(BaseModel):
    name: str
    node_links: str
    node_prefix: str | None = None
    is_primary: bool = False


class SubscriptionRead(SubscriptionBase):
    id: int
    source_nodes: list[dict] = Field(default_factory=list)
    raw_nodes: list[dict] = Field(default_factory=list)
    last_fetched_at: datetime | None = None
    last_fetch_error: str | None = None
    fetch_failed_count: int = 0
    fetch_comments: list[str] = Field(default_factory=list)
    subscription_userinfo: str | None = None
    profile_update_interval: str | None = None
    profile_web_page_url: str | None = None

    model_config = {"from_attributes": True}
