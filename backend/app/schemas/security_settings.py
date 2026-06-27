from pydantic import BaseModel, Field, model_validator


class SecuritySettingsRead(BaseModel):
    auth_enabled: bool
    protect_frontend: bool
    protect_api: bool
    protect_exports: bool
    has_token: bool
    fetch_proxy_enabled: bool
    fetch_proxy_url: str


class AuthCheckRequest(BaseModel):
    token: str = Field(default="", max_length=256)


class AuthCheckRead(BaseModel):
    ok: bool


class SecuritySettingsUpdate(BaseModel):
    auth_enabled: bool | None = None
    protect_frontend: bool | None = None
    protect_api: bool | None = None
    protect_exports: bool | None = None
    fetch_proxy_enabled: bool | None = None
    fetch_proxy_url: str | None = Field(default=None, max_length=512)
    token: str | None = Field(default=None, min_length=8, max_length=256)

    @model_validator(mode="after")
    def ensure_useful_update(self):
        if self.auth_enabled is None and self.protect_frontend is None and self.protect_api is None and self.protect_exports is None and self.fetch_proxy_enabled is None and self.fetch_proxy_url is None and self.token is None:
            raise ValueError("No settings provided")
        if self.fetch_proxy_enabled and not (self.fetch_proxy_url and self.fetch_proxy_url.strip()):
            raise ValueError("开启订阅代理前必须填写代理地址")
        return self
