from pydantic import BaseModel, Field, field_validator, model_validator


TARGET_TYPES = frozenset({"subscription", "node_group", "node"})
DIALER_TYPES = frozenset({"node", "node_group"})


class ProxyChainBindingBase(BaseModel):
    target_type: str
    target_id: int | None = None
    target_name: str | None = None
    dialer_type: str
    dialer_ref: str = Field(min_length=1, max_length=255)
    enabled: bool = True
    sort_order: int = 0
    note: str | None = None

    @field_validator("target_type")
    @classmethod
    def _target_type(cls, value: str) -> str:
        v = str(value or "").strip().lower()
        if v not in TARGET_TYPES:
            raise ValueError(f"target_type must be one of {sorted(TARGET_TYPES)}")
        return v

    @field_validator("dialer_type")
    @classmethod
    def _dialer_type(cls, value: str) -> str:
        v = str(value or "").strip().lower()
        if v not in DIALER_TYPES:
            raise ValueError(f"dialer_type must be one of {sorted(DIALER_TYPES)}")
        return v

    @field_validator("dialer_ref")
    @classmethod
    def _dialer_ref(cls, value: str) -> str:
        v = str(value or "").strip()
        if not v:
            raise ValueError("dialer_ref is required")
        return v

    @field_validator("target_name")
    @classmethod
    def _target_name(cls, value: str | None) -> str | None:
        if value is None:
            return None
        v = str(value).strip()
        return v or None

    @model_validator(mode="after")
    def _shape(self):
        if self.target_type == "node":
            if not self.target_name:
                raise ValueError("target_name is required when target_type=node")
        else:
            if self.target_id is None:
                raise ValueError(
                    "target_id is required when target_type is subscription or node_group"
                )
        return self


class ProxyChainBindingCreate(ProxyChainBindingBase):
    pass


class ProxyChainBindingUpdate(BaseModel):
    target_type: str | None = None
    target_id: int | None = None
    target_name: str | None = None
    dialer_type: str | None = None
    dialer_ref: str | None = None
    enabled: bool | None = None
    sort_order: int | None = None
    note: str | None = None

    @field_validator("target_type")
    @classmethod
    def _target_type(cls, value: str | None) -> str | None:
        if value is None:
            return None
        v = str(value).strip().lower()
        if v not in TARGET_TYPES:
            raise ValueError(f"target_type must be one of {sorted(TARGET_TYPES)}")
        return v

    @field_validator("dialer_type")
    @classmethod
    def _dialer_type(cls, value: str | None) -> str | None:
        if value is None:
            return None
        v = str(value).strip().lower()
        if v not in DIALER_TYPES:
            raise ValueError(f"dialer_type must be one of {sorted(DIALER_TYPES)}")
        return v

    @field_validator("dialer_ref")
    @classmethod
    def _dialer_ref(cls, value: str | None) -> str | None:
        if value is None:
            return None
        v = str(value).strip()
        if not v:
            raise ValueError("dialer_ref cannot be empty")
        return v

    @field_validator("target_name")
    @classmethod
    def _target_name(cls, value: str | None) -> str | None:
        if value is None:
            return None
        v = str(value).strip()
        return v or None


class ProxyChainBindingRead(ProxyChainBindingBase):
    id: int

    model_config = {"from_attributes": True}


class FinalNodeItem(BaseModel):
    name: str
    subscription_id: int | None = None
    subscription_name: str | None = None
    type: str | None = None
    server: str | None = None


class NodeLedgerItem(FinalNodeItem):
    dialer_proxy: str | None = None
    chain_source: str | None = None  # node | node_group | subscription
