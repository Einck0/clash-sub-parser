from pydantic import BaseModel


class RuleCategoryBase(BaseModel):
    name: str
    sort_order: int = 0


class RuleCategoryCreate(RuleCategoryBase):
    pass


class RuleCategoryUpdate(BaseModel):
    name: str | None = None
    sort_order: int | None = None


class RuleCategoryRead(RuleCategoryBase):
    id: int
    rule_count: int = 0

    model_config = {"from_attributes": True}


class RuleCategoryReorderItem(BaseModel):
    id: int
    sort_order: int


class RuleCategoryReorder(BaseModel):
    items: list[RuleCategoryReorderItem]
