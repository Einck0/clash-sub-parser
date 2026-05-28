from functools import lru_cache

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    app_name: str = "Clash Subscription Parser"
    app_version: str = "1.0.0"
    api_prefix: str = "/api"
    host: str = "0.0.0.0"
    port: int = 18080

    database_url: str = "sqlite+aiosqlite:////data/clash_sub_parser.db"
    request_timeout_seconds: int = 30
    request_max_bytes: int = 5 * 1024 * 1024
    request_user_agent: str = "ClashforWindows/0.20"
    request_trust_env: bool = False
    allow_private_fetch_urls: bool = False
    fetch_proxy_enabled: bool = False
    fetch_proxy_url: str = ""
    default_proxy_test_url: str = "http://www.gstatic.com/generate_204"
    scheduler_enabled: bool = True
    auth_enabled: bool = False
    auth_token: str = ""
    auth_cookie_secure: bool = False
    cors_allow_origins: list[str] = []
    cors_allow_methods: list[str] = ["*"]
    cors_allow_headers: list[str] = ["*"]

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        env_prefix="CLASH_",
    )


@lru_cache
def get_settings() -> Settings:
    return Settings()
