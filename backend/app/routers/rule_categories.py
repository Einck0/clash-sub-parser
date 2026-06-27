from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.rule_category import (
    RuleCategoryCreate,
    RuleCategoryRead,
    RuleCategoryReorder,
    RuleCategoryUpdate,
)
from app.services.rule_category_service import (
    batch_rule_categories,
    create_rule_category,
    delete_rule_category,
    get_rule_category,
    list_rule_categories,
    reorder_rule_categories,
    update_rule_category,
)

router = APIRouter(prefix="/rule-categories", tags=["rule-categories"])


@router.get("", response_model=list[RuleCategoryRead])
async def get_rule_categories(db: AsyncSession = Depends(get_db)) -> list[dict]:
    return await list_rule_categories(db)


@router.post("", response_model=RuleCategoryRead, status_code=status.HTTP_201_CREATED)
async def create_rule_category_endpoint(
    payload: RuleCategoryCreate, db: AsyncSession = Depends(get_db)
):
    item = await create_rule_category(db, payload)
    return {"id": item.id, "name": item.name, "sort_order": item.sort_order, "rule_count": 0}


@router.patch("/{category_id}", response_model=RuleCategoryRead)
async def update_rule_category_endpoint(
    category_id: int,
    payload: RuleCategoryUpdate,
    db: AsyncSession = Depends(get_db),
):
    item = await get_rule_category(db, category_id)
    if not item:
        raise HTTPException(status_code=404, detail="Rule category not found")
    updated = await update_rule_category(db, item, payload)
    count = next(
        (cat["rule_count"] for cat in await list_rule_categories(db) if cat["id"] == updated.id),
        0,
    )
    return {"id": updated.id, "name": updated.name, "sort_order": updated.sort_order, "rule_count": count}


@router.delete("/{category_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_rule_category_endpoint(
    category_id: int,
    db: AsyncSession = Depends(get_db),
) -> None:
    item = await get_rule_category(db, category_id)
    if item:
        await delete_rule_category(db, item)


@router.post("/reorder", response_model=list[RuleCategoryRead])
async def reorder_rule_categories_endpoint(
    payload: RuleCategoryReorder,
    db: AsyncSession = Depends(get_db),
) -> list[dict]:
    return await reorder_rule_categories(db, payload)


@router.post("/batch")
async def batch_rule_categories_endpoint(
    payload: dict, db: AsyncSession = Depends(get_db)
) -> list[dict]:
    """Batch create/update/delete rule categories.
    
    Payload format:
    {
        "delete": [id1, id2, ...],
        "create": [{...RuleCategoryCreate fields...}, ...],
        "update": [{"id": 1, ...RuleCategoryUpdate fields...}, ...],
        "reorder": [{"id": 1, "sort_order": 0}, ...]
    }
    """
    return await batch_rule_categories(db, payload)
