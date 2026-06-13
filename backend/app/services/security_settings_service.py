import hashlib
import secrets

from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.models.security_settings import SecuritySettings
from app.schemas.security_settings import SecuritySettingsRead, SecuritySettingsUpdate


def hash_token(token: str) -> str:
    if not token:
        return ""
    return hashlib.sha256(token.encode("utf-8")).hexdigest()


def token_matches(raw_token: str, token_hash: str) -> bool:
    if not raw_token or not token_hash:
        return False
    computed = hashlib.sha256(raw_token.encode("utf-8")).hexdigest()
    return secrets.compare_digest(computed, token_hash)


async def get_security_settings(db: AsyncSession) -> SecuritySettings:
    item = await db.get(SecuritySettings, 1)
    if item:
        return item

    settings = get_settings()
    item = SecuritySettings(
        id=1,
        auth_enabled=settings.auth_enabled,
        protect_frontend=True,
        protect_api=True,
        protect_exports=True,
        token_hash=hash_token(settings.auth_token),
        fetch_proxy_enabled=settings.fetch_proxy_enabled,
        fetch_proxy_url=settings.fetch_proxy_url,
    )
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


def to_read(item: SecuritySettings) -> SecuritySettingsRead:
    return SecuritySettingsRead(
        auth_enabled=item.auth_enabled,
        protect_frontend=item.protect_frontend,
        protect_api=item.protect_api,
        protect_exports=item.protect_exports,
        has_token=bool(item.token_hash),
        fetch_proxy_enabled=item.fetch_proxy_enabled,
        fetch_proxy_url=item.fetch_proxy_url or "",
    )


async def update_security_settings(db: AsyncSession, payload: SecuritySettingsUpdate) -> SecuritySettings:
    item = await get_security_settings(db)
    data = payload.model_dump(exclude_unset=True)
    token = data.pop("token", None)

    for key, value in data.items():
        setattr(item, key, value)

    if token is not None:
        item.token_hash = hash_token(token)

    if item.auth_enabled and not item.token_hash:
        raise ValueError("开启鉴权前必须设置至少 8 位 token")
    if item.fetch_proxy_enabled and not (item.fetch_proxy_url or '').strip():
        raise ValueError("开启订阅代理前必须填写代理地址")

    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


def get_fetch_proxy_config(item: SecuritySettings | None) -> tuple[bool, str]:
    if not item:
        return False, ""
    return bool(item.fetch_proxy_enabled), (item.fetch_proxy_url or "").strip()
