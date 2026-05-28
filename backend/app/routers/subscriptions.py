from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.subscription import (
    ManualNodeCreate,
    SubscriptionCreate,
    SubscriptionRead,
    SubscriptionUpdate,
)
from app.services.subscription_service import (
    collect_all_subscription_nodes,
    create_manual_node_subscription,
    create_subscription,
    delete_subscription,
    fetch_subscription_nodes,
    get_subscription,
    list_subscriptions,
    update_subscription,
)

router = APIRouter(prefix="/subscriptions", tags=["subscriptions"])


@router.get("", response_model=list[SubscriptionRead])
async def get_subscriptions(
    db: AsyncSession = Depends(get_db),
) -> list[SubscriptionRead]:
    return await list_subscriptions(db)


@router.post("", response_model=SubscriptionRead, status_code=status.HTTP_201_CREATED)
async def create_subscription_endpoint(
    payload: SubscriptionCreate,
    db: AsyncSession = Depends(get_db),
) -> SubscriptionRead:
    return await create_subscription(db, payload)


@router.post("/manual-node", response_model=SubscriptionRead, status_code=status.HTTP_201_CREATED)
async def create_manual_node_endpoint(
    payload: ManualNodeCreate,
    db: AsyncSession = Depends(get_db),
) -> SubscriptionRead:
    return await create_manual_node_subscription(db, payload)


@router.patch("/{subscription_id}", response_model=SubscriptionRead)
async def update_subscription_endpoint(
    subscription_id: int,
    payload: SubscriptionUpdate,
    db: AsyncSession = Depends(get_db),
) -> SubscriptionRead:
    item = await get_subscription(db, subscription_id)
    if not item:
        raise HTTPException(status_code=404, detail="Subscription not found")
    return await update_subscription(db, item, payload)


@router.delete("/{subscription_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_subscription_endpoint(
    subscription_id: int, db: AsyncSession = Depends(get_db)
) -> None:
    item = await get_subscription(db, subscription_id)
    if not item:
        raise HTTPException(status_code=404, detail="Subscription not found")
    await delete_subscription(db, item)


@router.post("/{subscription_id}/fetch", response_model=SubscriptionRead)
async def fetch_subscription_endpoint(
    subscription_id: int, db: AsyncSession = Depends(get_db)
) -> SubscriptionRead:
    item = await get_subscription(db, subscription_id)
    if not item:
        raise HTTPException(status_code=404, detail="Subscription not found")
    try:
        return await fetch_subscription_nodes(db, item)
    except HTTPException:
        raise
    except Exception as exc:
        refreshed = await get_subscription(db, subscription_id)
        detail = (refreshed.last_fetch_error if refreshed else None) or str(exc) or type(exc).__name__
        raise HTTPException(status_code=502, detail=detail) from exc


@router.get("/{subscription_id}/nodes", response_model=list[dict])
async def get_subscription_nodes(
    subscription_id: int, db: AsyncSession = Depends(get_db)
) -> list[dict]:
    item = await get_subscription(db, subscription_id)
    if not item:
        raise HTTPException(status_code=404, detail="Subscription not found")
    return item.raw_nodes or []


@router.get("/nodes/all", response_model=list[dict])
async def get_all_subscription_nodes(db: AsyncSession = Depends(get_db)) -> list[dict]:
    return await collect_all_subscription_nodes(db)
