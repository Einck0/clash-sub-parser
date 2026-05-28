from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
import yaml

from app.database import get_db
from app.models.dns import DnsConfig
from app.schemas.dns import DnsConfigRead, DnsConfigUpdate

router = APIRouter(prefix="/dns", tags=["dns"])


@router.get("", response_model=DnsConfigRead)
async def get_dns_config(db: AsyncSession = Depends(get_db)) -> DnsConfigRead:
    result = await db.execute(select(DnsConfig).where(DnsConfig.id == 1))
    item = result.scalar_one_or_none()
    if not item:
        item = DnsConfig(id=1, raw_yaml="", enabled=True)
        db.add(item)
        await db.commit()
        await db.refresh(item)
    return item


@router.patch("", response_model=DnsConfigRead)
async def update_dns_config(
    payload: DnsConfigUpdate, db: AsyncSession = Depends(get_db)
) -> DnsConfigRead:
    if payload.raw_yaml.strip():
        try:
            yaml.safe_load(payload.raw_yaml)
        except Exception as exc:
            raise HTTPException(
                status_code=400, detail=f"Invalid DNS YAML: {exc}"
            ) from exc
    result = await db.execute(select(DnsConfig).where(DnsConfig.id == 1))
    item = result.scalar_one_or_none()
    if not item:
        item = DnsConfig(id=1)

    item.raw_yaml = payload.raw_yaml
    item.enabled = payload.enabled

    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item
