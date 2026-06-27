from fastapi import HTTPException
from sqlalchemy import delete, func, select, update
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.schemas.rule_category import (
    RuleCategoryCreate,
    RuleCategoryReorder,
    RuleCategoryUpdate,
)


def _normalize_name(name: str) -> str:
    value = str(name or "").strip()
    if not value:
        raise HTTPException(status_code=400, detail="Category name is required")
    return value


async def list_rule_categories(db: AsyncSession) -> list[dict]:
    result = await db.execute(
        select(
            RuleCategory,
            func.count(Rule.id).label("rule_count"),
        )
        .outerjoin(Rule, Rule.category == RuleCategory.name)
        .group_by(RuleCategory.id)
        .order_by(RuleCategory.sort_order.asc(), RuleCategory.id.asc())
    )
    return [
        {
            "id": category.id,
            "name": category.name,
            "sort_order": category.sort_order,
            "rule_count": int(rule_count or 0),
        }
        for category, rule_count in result.all()
    ]


async def ensure_rule_category(db: AsyncSession, name: str) -> RuleCategory:
    category_name = _normalize_name(name)
    result = await db.execute(select(RuleCategory).where(RuleCategory.name == category_name))
    item = result.scalar_one_or_none()
    if item:
        return item
    max_order = await db.scalar(select(func.max(RuleCategory.sort_order)))
    item = RuleCategory(name=category_name, sort_order=int(max_order or 0) + 10)
    db.add(item)
    try:
        await db.commit()
        await db.refresh(item)
    except IntegrityError:
        await db.rollback()
        # Another request created it concurrently; re-fetch
        result = await db.execute(select(RuleCategory).where(RuleCategory.name == category_name))
        item = result.scalar_one_or_none()
        if not item:
            raise
    return item


async def create_rule_category(db: AsyncSession, payload: RuleCategoryCreate) -> RuleCategory:
    name = _normalize_name(payload.name)
    existing = await db.scalar(select(RuleCategory.id).where(RuleCategory.name == name))
    if existing:
        raise HTTPException(status_code=400, detail="Category already exists")
    item = RuleCategory(name=name, sort_order=payload.sort_order)
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def get_rule_category(db: AsyncSession, category_id: int) -> RuleCategory | None:
    return await db.get(RuleCategory, category_id)


async def update_rule_category(
    db: AsyncSession, item: RuleCategory, payload: RuleCategoryUpdate
) -> RuleCategory:
    data = payload.model_dump(exclude_unset=True)
    old_name = item.name
    if "name" in data and data["name"] is not None:
        new_name = _normalize_name(data["name"])
        existing = await db.scalar(
            select(RuleCategory.id).where(
                RuleCategory.name == new_name,
                RuleCategory.id != item.id,
            )
        )
        if existing:
            raise HTTPException(status_code=400, detail="Category already exists")
        item.name = new_name
        await db.execute(update(Rule).where(Rule.category == old_name).values(category=new_name))
    if "sort_order" in data and data["sort_order"] is not None:
        item.sort_order = int(data["sort_order"])
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def delete_rule_category(db: AsyncSession, item: RuleCategory, delete_rules: bool = True) -> int:
    """Delete a rule category. Returns the number of associated rules affected."""
    rule_count_result = await db.execute(
        select(func.count(Rule.id)).where(Rule.category == item.name)
    )
    rule_count = rule_count_result.scalar() or 0
    if delete_rules:
        await db.execute(delete(Rule).where(Rule.category == item.name))
    else:
        await db.execute(
            update(Rule).where(Rule.category == item.name).values(category="default")
        )
    await db.delete(item)
    await db.commit()
    return rule_count


async def reorder_rule_categories(
    db: AsyncSession, payload: RuleCategoryReorder
) -> list[dict]:
    ids = [entry.id for entry in payload.items]
    result = await db.execute(select(RuleCategory).where(RuleCategory.id.in_(ids)))
    mapping = {item.id: item for item in result.scalars().all()}
    for entry in payload.items:
        if entry.id in mapping:
            mapping[entry.id].sort_order = entry.sort_order
            db.add(mapping[entry.id])
    await db.commit()
    return await list_rule_categories(db)


async def batch_rule_categories(db: AsyncSession, payload: dict) -> list[dict]:
    """Process a batch of category operations atomically."""
    # Auto-snapshot before batch changes
    from app.services.snapshot_service import create_snapshot
    try:
        await create_snapshot(db, label="auto-before-batch-categories")
    except Exception:
        pass

    # 1. Deletes (with cascade)
    delete_ids = payload.get("delete", [])
    for cat_id in delete_ids:
        item = await db.get(RuleCategory, cat_id)
        if item:
            await db.execute(delete(Rule).where(Rule.category == item.name))
            await db.delete(item)
    
    # 2. Creates
    for item_data in payload.get("create", []):
        name = _normalize_name(item_data.get("name", ""))
        existing = await db.scalar(select(RuleCategory.id).where(RuleCategory.name == name))
        if existing:
            continue
        item = RuleCategory(name=name, sort_order=item_data.get("sort_order", 0))
        db.add(item)
    
    # 3. Updates
    for item_data in payload.get("update", []):
        cat_id = item_data.get("id")
        if not cat_id:
            continue
        item = await db.get(RuleCategory, cat_id)
        if not item:
            continue
        old_name = item.name
        if "name" in item_data and item_data["name"]:
            new_name = _normalize_name(item_data["name"])
            item.name = new_name
            await db.execute(update(Rule).where(Rule.category == old_name).values(category=new_name))
        if "sort_order" in item_data and item_data["sort_order"] is not None:
            item.sort_order = int(item_data["sort_order"])
        db.add(item)
    
    # 4. Reorder
    reorder_items = payload.get("reorder", [])
    if reorder_items:
        ids = [entry["id"] for entry in reorder_items]
        result = await db.execute(select(RuleCategory).where(RuleCategory.id.in_(ids)))
        mapping = {item.id: item for item in result.scalars().all()}
        for entry in reorder_items:
            if entry["id"] in mapping:
                mapping[entry["id"]].sort_order = entry.get("sort_order", 0)
                db.add(mapping[entry["id"]])
    
    await db.commit()
    return await list_rule_categories(db)
