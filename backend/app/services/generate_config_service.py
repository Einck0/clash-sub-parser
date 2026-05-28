from sqlalchemy.ext.asyncio import AsyncSession

from app.models.generate_config import GenerateConfig
from app.schemas.generate_config import GenerateConfigUpdate


async def get_generate_config(db: AsyncSession) -> GenerateConfig:
    item = await db.get(GenerateConfig, 1)
    if item:
        return item
    item = GenerateConfig(id=1)
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def update_generate_config(db: AsyncSession, payload: GenerateConfigUpdate) -> GenerateConfig:
    item = await get_generate_config(db)
    for key, value in payload.model_dump(exclude_unset=True).items():
        if value is not None:
            setattr(item, key, bool(value))
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


def generate_config_to_switches(item: GenerateConfig) -> dict:
    return {
        "enabled": item.enabled,
        "subscriptions": item.subscriptions,
        "node_groups": item.node_groups,
        "rules": item.rules,
        "dns": item.dns,
        "exclude_node_proxies": item.exclude_node_proxies,
    }
