from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.rule import RuleCreate, RuleRead, RuleReorder, RuleUpdate
from app.services.rule_service import (
    batch_rules,
    create_rule,
    delete_rule,
    get_rule,
    list_rules,
    reorder_rules,
    update_rule,
)


router = APIRouter(prefix="/rules", tags=["rules"])


@router.get("", response_model=list[RuleRead])
async def get_rules(db: AsyncSession = Depends(get_db)) -> list[RuleRead]:
    return await list_rules(db)


@router.get("/{rule_id}", response_model=RuleRead)
async def get_rule_endpoint(
    rule_id: int,
    db: AsyncSession = Depends(get_db),
) -> RuleRead:
    item = await get_rule(db, rule_id)
    if not item:
        raise HTTPException(status_code=404, detail="Rule not found")
    return item


@router.post("", response_model=RuleRead, status_code=status.HTTP_201_CREATED)
async def create_rule_endpoint(
    payload: RuleCreate, db: AsyncSession = Depends(get_db)
) -> RuleRead:
    return await create_rule(db, payload)


@router.patch("/{rule_id}", response_model=RuleRead)
async def update_rule_endpoint(
    rule_id: int,
    payload: RuleUpdate,
    db: AsyncSession = Depends(get_db),
) -> RuleRead:
    item = await get_rule(db, rule_id)
    if not item:
        raise HTTPException(status_code=404, detail="Rule not found")
    return await update_rule(db, item, payload)


@router.delete("/{rule_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_rule_endpoint(
    rule_id: int, db: AsyncSession = Depends(get_db)
) -> None:
    item = await get_rule(db, rule_id)
    if item:
        await delete_rule(db, item)


@router.post("/reorder", response_model=list[RuleRead])
async def reorder_rules_endpoint(
    payload: RuleReorder, db: AsyncSession = Depends(get_db)
) -> list[RuleRead]:
    return await reorder_rules(db, payload)


@router.post("/batch")
async def batch_rules_endpoint(
    payload: dict, db: AsyncSession = Depends(get_db)
) -> list[RuleRead]:
    """Batch create/update/delete rules in a single request.
    
    Payload format:
    {
        "delete": [id1, id2, ...],
        "create": [{...RuleCreate fields...}, ...],
        "update": [{"id": 1, ...RuleUpdate fields...}, ...],
        "reorder": [{"id": 1, "sort_order": 0}, ...]
    }
    """
    return await batch_rules(db, payload)
