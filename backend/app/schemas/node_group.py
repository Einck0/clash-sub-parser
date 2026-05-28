from pydantic import BaseModel, Field


class NodeGroupBase(BaseModel):
    name: str
    kind: str = Field(default="manual", pattern="^(regex|manual|other)$")
    group_type: str = Field(
        default="select", pattern="^(select|url-test|fallback|load-balance)$"
    )
    sort_order: int = 0
    regex_rules: list[str] = Field(default_factory=list)
    include_nodes: list[str] = Field(default_factory=list)
    include_group_ids: list[int] = Field(default_factory=list)
    include_group_nodes_ids: list[int] = Field(default_factory=list)
    include_entries: list[dict] = Field(default_factory=list)
    add_fallback: bool = True
    exclude_nodes: list[str] = Field(default_factory=list)
    url_test_config: dict = Field(default_factory=dict)
    load_balance_config: dict = Field(default_factory=dict)
    fallback_config: dict = Field(default_factory=dict)


class NodeGroupCreate(NodeGroupBase):
    pass


class NodeGroupUpdate(BaseModel):
    name: str | None = None
    kind: str | None = Field(default=None, pattern="^(regex|manual|other)$")
    group_type: str | None = Field(
        default=None, pattern="^(select|url-test|fallback|load-balance)$"
    )
    sort_order: int | None = None
    regex_rules: list[str] | None = None
    include_nodes: list[str] | None = None
    include_group_ids: list[int] | None = None
    include_group_nodes_ids: list[int] | None = None
    include_entries: list[dict] | None = None
    add_fallback: bool | None = None
    exclude_nodes: list[str] | None = None
    url_test_config: dict | None = None
    load_balance_config: dict | None = None
    fallback_config: dict | None = None


class NodeGroupRead(NodeGroupBase):
    id: int

    model_config = {"from_attributes": True}


class NodeGroupReorderItem(BaseModel):
    id: int
    sort_order: int


class NodeGroupReorder(BaseModel):
    items: list[NodeGroupReorderItem]
