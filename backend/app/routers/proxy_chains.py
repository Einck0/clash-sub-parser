from typing import Any

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel, Field
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.proxy_chain import (
    FinalNodeItem,
    ProxyChainBindingCreate,
    ProxyChainBindingRead,
    ProxyChainBindingUpdate,
)
from app.services import proxy_chain_service as service

router = APIRouter(prefix="/proxy-chains", tags=["proxy-chains"])


class ProxyChainPreviewRequest(BaseModel):
    target_type: str
    target_id: int | None = None
    target_name: str | None = None
    dialer_type: str
    dialer_ref: str = Field(min_length=1)


@router.get("", response_model=list[ProxyChainBindingRead])
async def list_proxy_chains(db: AsyncSession = Depends(get_db)):
    return await service.list_bindings(db)


@router.post("", response_model=ProxyChainBindingRead, status_code=201)
async def create_proxy_chain(
    payload: ProxyChainBindingCreate, db: AsyncSession = Depends(get_db)
):
    return await service.create_binding(db, payload)


@router.patch("/{binding_id}", response_model=ProxyChainBindingRead)
async def update_proxy_chain(
    binding_id: int,
    payload: ProxyChainBindingUpdate,
    db: AsyncSession = Depends(get_db),
):
    item = await service.get_binding(db, binding_id)
    if item is None:
        raise HTTPException(status_code=404, detail="Proxy chain binding not found")
    return await service.update_binding(db, item, payload)


@router.delete("/{binding_id}", status_code=204)
async def delete_proxy_chain(binding_id: int, db: AsyncSession = Depends(get_db)):
    item = await service.get_binding(db, binding_id)
    if item is None:
        raise HTTPException(status_code=404, detail="Proxy chain binding not found")
    await service.delete_binding(db, item)
    return None


@router.get("/meta/final-nodes", response_model=list[FinalNodeItem])
async def list_final_nodes(db: AsyncSession = Depends(get_db)):
    """All processed final node names from enabled subscriptions (for pickers)."""
    return await service.list_final_nodes(db)


@router.post("/meta/preview")
async def preview_proxy_chain(
    payload: ProxyChainPreviewRequest, db: AsyncSession = Depends(get_db)
) -> dict[str, Any]:
    """Preview chain/skip counts before saving a binding."""
    return await service.preview_binding_effect(
        db,
        target_type=payload.target_type,
        target_id=payload.target_id,
        target_name=payload.target_name,
        dialer_type=payload.dialer_type,
        dialer_ref=payload.dialer_ref,
    )
