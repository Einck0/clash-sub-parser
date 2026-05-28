from pydantic import BaseModel, Field, field_validator


def normalize_rule_type(value: str) -> str:
    return str(value or "").strip().upper()


class RuleBase(BaseModel):
    name: str = ""
    category: str = "default"
    type: str
    value: str = ""
    proxy: str
    options: list[str] = Field(default_factory=list)
    sort_order: int = 0
    enabled: bool = True

    @field_validator("type")
    @classmethod
    def normalize_type(cls, value: str) -> str:
        return normalize_rule_type(value)


class RuleCreate(RuleBase):
    pass


class RuleUpdate(BaseModel):
    name: str | None = None
    category: str | None = None
    type: str | None = None
    value: str | None = None
    proxy: str | None = None
    options: list[str] | None = None
    sort_order: int | None = None
    enabled: bool | None = None

    @field_validator("type")
    @classmethod
    def normalize_type(cls, value: str | None) -> str | None:
        if value is None:
            return None
        return normalize_rule_type(value)


class RuleRead(RuleBase):
    id: int

    model_config = {"from_attributes": True}


class RuleReorderItem(BaseModel):
    id: int
    sort_order: int


class RuleReorder(BaseModel):
    items: list[RuleReorderItem]
