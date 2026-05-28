from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.node_group import NodeGroup
from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.schemas.rule import RuleCreate, RuleReorder, RuleUpdate, normalize_rule_type
from app.services.rule_category_service import ensure_rule_category
from app.utils.validators import validate_rule_proxies_exist

MATCH_RULE_TYPES = {"MATCH"}
VALUE_RULE_TYPES = {
    "DOMAIN",
    "DOMAIN-SUFFIX",
    "DOMAIN-KEYWORD",
    "DOMAIN-REGEX",
    "IP-CIDR",
    "IP-CIDR6",
    "GEOIP",
    "GEOSITE",
    "PROCESS-NAME",
    "PROCESS-PATH",
    "DST-PORT",
    "SRC-IP-CIDR",
    "SRC-PORT",
    "RULE-SET",
}
KNOWN_RULE_TYPES = VALUE_RULE_TYPES | MATCH_RULE_TYPES


async def list_rules(db: AsyncSession) -> list[Rule]:
    result = await db.execute(
        select(Rule)
        .outerjoin(RuleCategory, Rule.category == RuleCategory.name)
        .order_by(RuleCategory.sort_order.asc().nulls_last(), Rule.sort_order.asc(), Rule.id.asc())
    )
    return list(result.scalars().all())


async def get_rule(db: AsyncSession, rule_id: int) -> Rule | None:
    return await db.get(Rule, rule_id)


async def create_rule(db: AsyncSession, payload: RuleCreate) -> Rule:
    data = _normalize_rule_data(payload.model_dump())
    await ensure_rule_category(db, data["category"])
    await _validate_rule_targets(db, [data["proxy"]])
    item = Rule(**data)
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def update_rule(db: AsyncSession, item: Rule, payload: RuleUpdate) -> Rule:
    current = {
        "name": item.name,
        "category": item.category,
        "type": item.type,
        "value": item.value,
        "proxy": item.proxy,
        "options": item.options or [],
        "sort_order": item.sort_order,
        "enabled": item.enabled,
    }
    current.update(payload.model_dump(exclude_unset=True))
    data = _normalize_rule_data(current)
    await ensure_rule_category(db, data["category"])
    await _validate_rule_targets(db, [data["proxy"]])

    for key, value in data.items():
        setattr(item, key, value)
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def delete_rule(db: AsyncSession, item: Rule) -> None:
    await db.delete(item)
    await db.commit()


async def reorder_rules(db: AsyncSession, payload: RuleReorder) -> list[Rule]:
    ids = [entry.id for entry in payload.items]
    result = await db.execute(select(Rule).where(Rule.id.in_(ids)))
    mapping = {item.id: item for item in result.scalars().all()}
    for entry in payload.items:
        if entry.id in mapping:
            mapping[entry.id].sort_order = entry.sort_order
            db.add(mapping[entry.id])
    await db.commit()
    return await list_rules(db)


async def _validate_rule_targets(db: AsyncSession, proxies: list[str]) -> None:
    result = await db.execute(select(NodeGroup.name))
    group_names = set(result.scalars().all())
    validate_rule_proxies_exist(proxies, group_names)


def _normalize_rule_data(data: dict) -> dict:
    rule_type = normalize_rule_type(str(data.get("type", "")).strip())
    value = str(data.get("value", "")).strip()
    proxy = str(data.get("proxy", "")).strip()
    options = [str(opt).strip() for opt in data.get("options", []) if str(opt).strip()]

    _validate_rule_item(rule_type, value, proxy)

    return {
        "name": str(data.get("name", "")).strip(),
        "category": str(data.get("category", "default")).strip() or "default",
        "type": rule_type,
        "value": "" if rule_type in MATCH_RULE_TYPES else value,
        "proxy": proxy,
        "options": options,
        "sort_order": int(data.get("sort_order", 0) or 0),
        "enabled": bool(data.get("enabled", True)),
    }


def _validate_rule_item(rule_type: str, value: str, proxy: str) -> None:
    if not rule_type:
        raise HTTPException(status_code=400, detail="Rule type is required")
    if not proxy:
        raise HTTPException(status_code=400, detail="Rule proxy is required")
    if rule_type not in KNOWN_RULE_TYPES:
        # Clash supports custom providers/types in some clients. Keep this permissive
        # but normalized, instead of rejecting user-imported rules.
        return
    if rule_type not in MATCH_RULE_TYPES and not value:
        raise HTTPException(status_code=400, detail="Rule value is required")
