from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.node_group import NodeGroup
from app.models.rule import Rule
from app.models.subscription import Subscription
from app.utils.dedup import deduplicate_nodes
from app.schemas.node_group import NodeGroupCreate, NodeGroupReorder, NodeGroupUpdate
from app.utils.group_utils import dedup_names, with_fallback, resolve_entries
from app.utils.validators import ensure_group_ids_exist, validate_no_circular_reference

BUILTIN_GROUPS = ["DIRECT", "REJECT", "PASS"]


async def list_node_groups(db: AsyncSession) -> list[NodeGroup]:
    result = await db.execute(
        select(NodeGroup).order_by(NodeGroup.sort_order.asc(), NodeGroup.id.asc())
    )
    return list(result.scalars().all())


async def get_node_group(db: AsyncSession, node_group_id: int) -> NodeGroup | None:
    return await db.get(NodeGroup, node_group_id)


async def create_node_group(db: AsyncSession, payload: NodeGroupCreate) -> NodeGroup:
    data = payload.model_dump()
    _normalize_group_payload(data)
    _sync_entry_derived_fields(data)
    await _validate_node_group_relations(
        db,
        data["include_group_ids"],
        data["include_group_nodes_ids"],
        None,
    )
    item = NodeGroup(**data)
    db.add(item)
    try:
        await db.flush()
        await _validate_cycles_or_raise(db)
        await db.commit()
        await db.refresh(item)
        return item
    except Exception:
        await db.rollback()
        raise


async def update_node_group(
    db: AsyncSession, item: NodeGroup, payload: NodeGroupUpdate
) -> NodeGroup:
    data = payload.model_dump(exclude_unset=True)
    _normalize_group_payload(data)
    if "include_entries" in data:
        _sync_entry_derived_fields(data)

    include_group_ids = data.get("include_group_ids", item.include_group_ids)
    include_group_nodes_ids = data.get(
        "include_group_nodes_ids", item.include_group_nodes_ids
    )
    await _validate_node_group_relations(
        db,
        include_group_ids,
        include_group_nodes_ids,
        item.id,
    )

    for key, value in data.items():
        setattr(item, key, value)
    db.add(item)
    try:
        await db.flush()
        await _validate_cycles_or_raise(db)
        await db.commit()
        await db.refresh(item)
        return item
    except Exception:
        await db.rollback()
        raise


async def delete_node_group(db: AsyncSession, item: NodeGroup) -> None:
    if item.name in BUILTIN_GROUPS:
        raise HTTPException(status_code=400, detail="Builtin groups cannot be deleted")
    await _ensure_group_not_referenced(db, item)
    await db.delete(item)
    await db.commit()


async def reorder_node_groups(
    db: AsyncSession, payload: NodeGroupReorder
) -> list[NodeGroup]:
    ids = [entry.id for entry in payload.items]
    result = await db.execute(select(NodeGroup).where(NodeGroup.id.in_(ids)))
    mapping = {item.id: item for item in result.scalars().all()}
    for entry in payload.items:
        if entry.id in mapping:
            mapping[entry.id].sort_order = entry.sort_order
            db.add(mapping[entry.id])
    await db.commit()
    return await list_node_groups(db)


async def validate_node_groups(db: AsyncSession) -> dict[str, str]:
    await _validate_cycles_or_raise(db)
    return {"status": "ok"}


async def preview_node_groups(db: AsyncSession) -> list[dict]:
    groups = await list_node_groups(db)
    group_map = {group.id: group for group in groups}

    node_result = await db.execute(select(Subscription.raw_nodes))
    all_nodes: list[dict] = []
    for nodes in node_result.scalars().all():
        all_nodes.extend(nodes or [])
    node_names = [
        str(node.get("name", "")).strip() for node in deduplicate_nodes(all_nodes)
    ]
    node_names = [name for name in node_names if name]

    cache: dict[int, list[str]] = {}

    def resolve_nodes(group_id: int, trail: set[int]) -> list[str]:
        if group_id in cache:
            return cache[group_id]
        if group_id in trail:
            return []
        trail.add(group_id)
        group = group_map.get(group_id)
        if not group:
            trail.remove(group_id)
            return []

        selected: list[str] = []

        entries = resolve_entries(group)
        for entry in entries:
            entry_type = entry.get("type")
            entry_value = entry.get("value")
            if entry_type == "node":
                selected.append(str(entry_value))
            elif entry_type == "group":
                try:
                    ref_id = int(entry_value)
                except Exception:
                    continue
                if ref_id in group_map:
                    selected.append(group_map[ref_id].name)
            elif entry_type == "group_nodes":
                try:
                    ref_id = int(entry_value)
                except Exception:
                    continue
                selected.extend(resolve_nodes(ref_id, trail))

        for regex_rule in group.regex_rules or []:
            try:
                import re

                pattern = re.compile(regex_rule)
            except Exception:
                continue
            selected.extend([name for name in node_names if pattern.search(name)])

        excluded = set(group.exclude_nodes or [])
        merged = [name for name in dedup_names(selected) if name not in excluded]
        cache[group_id] = merged
        trail.remove(group_id)
        return merged

    preview: list[dict] = []
    for group in groups:
        include_group_names: list[str] = []
        include_group_nodes_names: list[str] = []
        entries = resolve_entries(group)
        for entry in entries:
            entry_type = entry.get("type")
            entry_value = entry.get("value")
            if entry_type == "group" and entry_value in group_map:
                include_group_names.append(group_map[entry_value].name)
            if entry_type == "group_nodes" and entry_value in group_map:
                include_group_nodes_names.append(group_map[entry_value].name)
        resolved_nodes = with_fallback(resolve_nodes(group.id, set()), group.add_fallback)
        preview.append(
            {
                "id": group.id,
                "name": group.name,
                "resolved_nodes": resolved_nodes,
                "resolved_count": len(resolved_nodes),
                "include_group_names": include_group_names,
                "include_group_nodes_names": include_group_nodes_names,
                "include_entries": entries,
            }
        )

    return preview


async def _validate_node_group_relations(
    db: AsyncSession,
    include_group_ids: list[int],
    include_group_nodes_ids: list[int],
    self_id: int | None,
) -> None:
    result = await db.execute(select(NodeGroup.id))
    existing_ids = set(result.scalars().all())
    ensure_group_ids_exist(include_group_ids, existing_ids)
    ensure_group_ids_exist(include_group_nodes_ids, existing_ids)
    if self_id is not None and (
        self_id in include_group_ids or self_id in include_group_nodes_ids
    ):
        raise HTTPException(status_code=400, detail="Node group cannot include itself")


async def _validate_cycles_or_raise(db: AsyncSession) -> None:
    result = await db.execute(
        select(
            NodeGroup.id, NodeGroup.include_group_ids, NodeGroup.include_group_nodes_ids
        )
    )
    graph = {}
    for row in result.all():
        graph[row[0]] = (row[1] or []) + (row[2] or [])
    validate_no_circular_reference(graph)


def _normalize_group_payload(data: dict) -> None:
    if "regex_rules" in data:
        data["regex_rules"] = [
            i.strip() for i in (data.get("regex_rules") or []) if i and i.strip()
        ]
    if "include_nodes" in data:
        data["include_nodes"] = [
            i.strip() for i in (data.get("include_nodes") or []) if i and i.strip()
        ]
    if "exclude_nodes" in data:
        data["exclude_nodes"] = [
            i.strip() for i in (data.get("exclude_nodes") or []) if i and i.strip()
        ]
    if "include_entries" in data:
        data["include_entries"] = _normalize_entries(data.get("include_entries") or [])


def _sync_entry_derived_fields(data: dict) -> None:
    entries = data.get("include_entries") or []
    include_nodes: list[str] = []
    include_group_ids: list[int] = []
    include_group_nodes_ids: list[int] = []

    for entry in entries:
        entry_type = entry.get("type")
        value = entry.get("value")
        if entry_type == "node":
            include_nodes.append(str(value))
        elif entry_type == "group":
            include_group_ids.append(int(value))
        elif entry_type == "group_nodes":
            include_group_nodes_ids.append(int(value))

    data["include_nodes"] = include_nodes
    data["include_group_ids"] = include_group_ids
    data["include_group_nodes_ids"] = include_group_nodes_ids


async def _ensure_group_not_referenced(db: AsyncSession, item: NodeGroup) -> None:
    group_result = await db.execute(
        select(
            NodeGroup.id,
            NodeGroup.name,
            NodeGroup.include_group_ids,
            NodeGroup.include_group_nodes_ids,
        ).where(NodeGroup.id != item.id)
    )
    for row in group_result.all():
        include_group_ids = row[2] or []
        include_group_nodes_ids = row[3] or []
        if item.id in include_group_ids or item.id in include_group_nodes_ids:
            raise HTTPException(
                status_code=400,
                detail=f"Node group is referenced by '{row[1]}'",
            )

    rule_result = await db.execute(select(Rule.name, Rule.proxy).where(Rule.proxy == item.name))
    for rule_name, _ in rule_result.all():
        raise HTTPException(
            status_code=400,
            detail=f"Node group is referenced by rule '{rule_name or item.name}'",
        )


def dedup_names(items: list[str]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for item in items:
        value = str(item).strip()
        if not value or value in seen:
            continue
        seen.add(value)
        result.append(value)
    return result


def _normalize_entries(entries: list[dict]) -> list[dict]:
    allowed_types = {"node", "group", "group_nodes"}
    normalized: list[dict] = []
    for item in entries:
        if hasattr(item, "model_dump"):
            raw = item.model_dump()
        else:
            raw = dict(item)

        entry_type = str(raw.get("type", "")).strip()
        if entry_type not in allowed_types:
            continue

        if entry_type == "node":
            value = str(raw.get("value", "")).strip()
            if value:
                normalized.append({"type": "node", "value": value})
            continue

        try:
            group_id = int(raw.get("value"))
        except Exception:
            continue
        normalized.append({"type": entry_type, "value": group_id})

    return normalized


def with_fallback(names: list[str], enabled: bool) -> list[str]:
    if not enabled:
        return names
    return [name for name in names if name != "REJECT"] + ["REJECT"]


def resolve_entries(group: NodeGroup) -> list[dict]:
    entries = list(group.include_entries or [])
    if entries:
        return entries

    fallback: list[dict] = []
    for node in group.include_nodes or []:
        fallback.append({"type": "node", "value": node})
    for group_id in group.include_group_ids or []:
        fallback.append({"type": "group", "value": group_id})
    for group_id in group.include_group_nodes_ids or []:
        fallback.append({"type": "group_nodes", "value": group_id})
    return fallback
