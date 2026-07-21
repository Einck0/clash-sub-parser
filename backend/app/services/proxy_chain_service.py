"""Proxy-chain v2: post-process dialer bindings over settled nodes/groups."""

from __future__ import annotations

import re
from typing import Any

from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.node_group import NodeGroup
from app.models.proxy_chain import ProxyChainBinding
from app.models.subscription import Subscription
from app.schemas.proxy_chain import ProxyChainBindingCreate, ProxyChainBindingUpdate
from app.utils.group_utils import dedup_names, resolve_entries

FORBIDDEN_DIALERS = frozenset({"DIRECT", "REJECT", "PASS"})
TARGET_PRIORITY = {"subscription": 1, "node_group": 2, "node": 3}


async def list_bindings(db: AsyncSession) -> list[ProxyChainBinding]:
    result = await db.execute(
        select(ProxyChainBinding).order_by(
            ProxyChainBinding.sort_order.asc(),
            ProxyChainBinding.id.asc(),
        )
    )
    return list(result.scalars().all())


async def get_binding(db: AsyncSession, binding_id: int) -> ProxyChainBinding | None:
    return await db.get(ProxyChainBinding, binding_id)


async def create_binding(db: AsyncSession, payload: ProxyChainBindingCreate) -> ProxyChainBinding:
    data = payload.model_dump()
    await _validate_binding_refs(db, data)
    item = ProxyChainBinding(**data)
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def update_binding(
    db: AsyncSession, item: ProxyChainBinding, payload: ProxyChainBindingUpdate
) -> ProxyChainBinding:
    data = payload.model_dump(exclude_unset=True)
    merged = {
        "target_type": data.get("target_type", item.target_type),
        "target_id": data.get("target_id", item.target_id) if "target_id" in data or "target_type" not in data else data.get("target_id", item.target_id),
        "target_name": data.get("target_name", item.target_name) if "target_name" in data or "target_type" not in data else data.get("target_name", item.target_name),
        "dialer_type": data.get("dialer_type", item.dialer_type),
        "dialer_ref": data.get("dialer_ref", item.dialer_ref),
        "enabled": data.get("enabled", item.enabled),
        "sort_order": data.get("sort_order", item.sort_order),
        "note": data.get("note", item.note),
    }
    # When target_type changes, clear the irrelevant key.
    if "target_type" in data:
        if merged["target_type"] == "node":
            merged["target_id"] = data.get("target_id", None)
            if not merged.get("target_name"):
                raise HTTPException(status_code=400, detail="target_name is required when target_type=node")
        else:
            if merged.get("target_id") is None:
                raise HTTPException(
                    status_code=400,
                    detail="target_id is required when target_type is subscription or node_group",
                )
    await _validate_binding_refs(db, merged)
    for key, value in data.items():
        setattr(item, key, value)
    # Normalize cleared fields after type switch
    if item.target_type == "node":
        item.target_id = None
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def delete_binding(db: AsyncSession, item: ProxyChainBinding) -> None:
    await db.delete(item)
    await db.commit()


async def list_final_nodes(db: AsyncSession) -> list[dict[str, Any]]:
    result = await db.execute(
        select(Subscription).where(Subscription.enabled.is_(True)).order_by(Subscription.id.asc())
    )
    out: list[dict[str, Any]] = []
    seen: set[str] = set()
    for sub in result.scalars().all():
        for node in sub.raw_nodes or []:
            if not isinstance(node, dict):
                continue
            name = str(node.get("name") or "").strip()
            if not name or name in seen:
                continue
            seen.add(name)
            out.append(
                {
                    "name": name,
                    "subscription_id": sub.id,
                    "subscription_name": sub.name,
                }
            )
    return out


async def _validate_binding_refs(db: AsyncSession, data: dict[str, Any]) -> None:
    target_type = data["target_type"]
    dialer_ref = str(data["dialer_ref"]).strip()
    if dialer_ref in FORBIDDEN_DIALERS:
        raise HTTPException(status_code=400, detail=f"dialer_ref cannot be {dialer_ref}")

    if target_type == "subscription":
        sub = await db.get(Subscription, int(data["target_id"]))
        if sub is None:
            raise HTTPException(status_code=400, detail="subscription target not found")
        data["target_name"] = sub.name
    elif target_type == "node_group":
        group = await db.get(NodeGroup, int(data["target_id"]))
        if group is None:
            raise HTTPException(status_code=400, detail="node_group target not found")
        data["target_name"] = group.name
    elif target_type == "node":
        name = str(data.get("target_name") or "").strip()
        if not name:
            raise HTTPException(status_code=400, detail="target_name is required when target_type=node")
        data["target_name"] = name
        data["target_id"] = None
        if name == dialer_ref and data.get("dialer_type") == "node":
            raise HTTPException(status_code=400, detail="节点不能把跳板设为自己")

    # Soft existence check for dialer (warn via 400 if completely unknown).
    known_nodes, known_groups = await _known_names(db)
    if data["dialer_type"] == "node" and dialer_ref not in known_nodes:
        raise HTTPException(status_code=400, detail=f"dialer node not found: {dialer_ref}")
    if data["dialer_type"] == "node_group" and dialer_ref not in known_groups:
        raise HTTPException(status_code=400, detail=f"dialer group not found: {dialer_ref}")

    # Membership / dialer cycle: target leaves must not include or depend on dialer.
    await _validate_no_cycle(db, data, known_nodes=known_nodes)


async def _known_names(db: AsyncSession) -> tuple[set[str], set[str]]:
    nodes: set[str] = set()
    result = await db.execute(select(Subscription).where(Subscription.enabled.is_(True)))
    for sub in result.scalars().all():
        for node in sub.raw_nodes or []:
            if isinstance(node, dict) and node.get("name"):
                nodes.add(str(node["name"]).strip())
    gres = await db.execute(select(NodeGroup.name))
    groups = {str(n).strip() for n in gres.scalars().all() if n}
    return nodes, groups


async def apply_bindings_to_nodes(db: AsyncSession, nodes: list[dict]) -> list[dict]:
    """Apply enabled bindings onto final proxy list (P0 single hop).

    Priority: node > node_group > subscription.
    Only leaf proxy nodes receive dialer-proxy.
    """
    if not nodes:
        return nodes

    bindings = [b for b in await list_bindings(db) if b.enabled]
    if not bindings:
        # Strip any accidental dialer fields from stored nodes.
        cleaned = []
        for node in nodes:
            if not isinstance(node, dict):
                continue
            copied = dict(node)
            copied.pop("dialer-proxy", None)
            cleaned.append(copied)
        return cleaned

    # Map final node name -> subscription ids that contributed it (first wins for ownership).
    sub_result = await db.execute(select(Subscription).where(Subscription.enabled.is_(True)))
    subs = list(sub_result.scalars().all())
    node_to_sub_ids: dict[str, set[int]] = {}
    for sub in subs:
        for node in sub.raw_nodes or []:
            if not isinstance(node, dict):
                continue
            name = str(node.get("name") or "").strip()
            if not name:
                continue
            node_to_sub_ids.setdefault(name, set()).add(sub.id)

    group_result = await db.execute(
        select(NodeGroup).order_by(NodeGroup.sort_order.asc(), NodeGroup.id.asc())
    )
    groups = list(group_result.scalars().all())
    group_mapping = {g.id: g for g in groups}
    all_node_names = [
        str(n.get("name") or "").strip()
        for n in nodes
        if isinstance(n, dict) and n.get("name")
    ]
    group_leaves = _resolve_all_group_leaves(groups, all_node_names)

    known_proxy_names = {n for n in all_node_names if n}
    known_group_names = {g.name for g in groups if g.name}

    # effective[name] = (priority, dialer_ref)
    effective: dict[str, tuple[int, str]] = {}

    # Apply low priority first so higher can overwrite.
    ordered = sorted(
        bindings,
        key=lambda b: (TARGET_PRIORITY.get(b.target_type, 0), b.sort_order, b.id or 0),
    )
    for binding in ordered:
        dialer = str(binding.dialer_ref or "").strip()
        if not dialer or dialer in FORBIDDEN_DIALERS:
            continue
        if binding.dialer_type == "node" and dialer not in known_proxy_names:
            continue
        if binding.dialer_type == "node_group" and dialer not in known_group_names:
            continue

        targets = _expand_targets(
            binding,
            node_to_sub_ids=node_to_sub_ids,
            group_leaves=group_leaves,
            known_proxy_names=known_proxy_names,
        )
        # Skip nodes that would form dialer membership loops with group dialers.
        if binding.dialer_type == "node_group":
            dialer_group_id = next((g.id for g in groups if g.name == dialer), None)
            dialer_leaves = set(group_leaves.get(dialer_group_id, [])) if dialer_group_id is not None else set()
            targets = [n for n in targets if n not in dialer_leaves and n != dialer]
        prio = TARGET_PRIORITY.get(binding.target_type, 0)
        for name in targets:
            if name == dialer:
                continue
            prev = effective.get(name)
            if prev is None or prio >= prev[0]:
                effective[name] = (prio, dialer)

    out: list[dict] = []
    for node in nodes:
        if not isinstance(node, dict):
            continue
        copied = dict(node)
        name = str(copied.get("name") or "").strip()
        copied.pop("dialer-proxy", None)
        hit = effective.get(name)
        if hit:
            copied["dialer-proxy"] = hit[1]
        out.append(copied)
    return out


async def _validate_no_cycle(
    db: AsyncSession,
    data: dict[str, Any],
    *,
    known_nodes: set[str],
) -> None:
    """Reject bindings that would make targets dial through a group containing themselves.

    Classic bad case:
      subscription 7li -> dialer group "链式"
      group "链式" expands to many 7li nodes
      => those nodes get dialer-proxy: 链式 while also being members of 链式
      => Clash client loop
    """
    dialer_type = data.get("dialer_type")
    dialer_ref = str(data.get("dialer_ref") or "").strip()
    if not dialer_ref:
        return

    # Build current world names for target expansion.
    sub_result = await db.execute(select(Subscription).where(Subscription.enabled.is_(True)))
    subs = list(sub_result.scalars().all())
    node_to_sub_ids: dict[str, set[int]] = {}
    all_node_names: list[str] = []
    for sub in subs:
        for node in sub.raw_nodes or []:
            if not isinstance(node, dict):
                continue
            name = str(node.get("name") or "").strip()
            if not name:
                continue
            all_node_names.append(name)
            node_to_sub_ids.setdefault(name, set()).add(sub.id)

    group_result = await db.execute(select(NodeGroup))
    groups = list(group_result.scalars().all())
    group_leaves = _resolve_all_group_leaves(groups, all_node_names)
    known_proxy_names = set(known_nodes) | {n for n in all_node_names if n}

    # Temporary binding-like object for expand.
    class _Tmp:
        pass

    tmp = _Tmp()
    tmp.target_type = data["target_type"]
    tmp.target_id = data.get("target_id")
    tmp.target_name = data.get("target_name")
    targets = set(
        _expand_targets(
            tmp,  # type: ignore[arg-type]
            node_to_sub_ids=node_to_sub_ids,
            group_leaves=group_leaves,
            known_proxy_names=known_proxy_names,
        )
    )
    if not targets:
        return

    if dialer_type == "node":
        # Subscription/group targets may include the entry node itself; that is OK.
        # Generate already skips self dialer. Only pure node-target self-ref is fatal
        # (already checked above). Reject only when target_type=node and names match.
        if data.get("target_type") == "node" and dialer_ref in targets:
            raise HTTPException(
                status_code=400,
                detail="节点不能把跳板设为自己",
            )
        return

    if dialer_type == "node_group":
        g = next((x for x in groups if x.name == dialer_ref), None)
        if g is None:
            return
        # Target group cannot dialer itself (always a full loop).
        if data.get("target_type") == "node_group" and data.get("target_id") == g.id:
            raise HTTPException(
                status_code=400,
                detail=f"会形成环：策略组「{dialer_ref}」不能把自己当跳板",
            )
        leaves = set(group_leaves.get(g.id, []))
        overlap = targets & leaves
        safe = targets - leaves
        # Hard-reject only when every target would loop. Partial overlap is OK:
        # generate skips members of the dialer group and still chains the rest.
        if targets and not safe:
            sample = "、".join(sorted(overlap)[:5])
            more = f" 等 {len(overlap)} 个" if len(overlap) > 5 else ""
            raise HTTPException(
                status_code=400,
                detail=(
                    f"会形成环：目标里的节点全部属于跳板策略组「{dialer_ref}」"
                    f"（{sample}{more}），没有可安全挂链的出口。"
                    "常见原因：落地订阅节点名命中了入口组的正则（如名字含「便宜」），"
                    "或入口组直接 include 了这批落地。"
                    "做法：入口请用「只含入口」的节点/策略组，且与落地节点集合不相交；"
                    "例如单独建入口组，不要用包含落地的「便宜/链式」。"
                ),
            )



def _expand_targets(
    binding: ProxyChainBinding,
    *,
    node_to_sub_ids: dict[str, set[int]],
    group_leaves: dict[int, list[str]],
    known_proxy_names: set[str],
) -> list[str]:
    if binding.target_type == "node":
        name = str(binding.target_name or "").strip()
        return [name] if name and name in known_proxy_names else []
    if binding.target_type == "subscription":
        sid = binding.target_id
        if sid is None:
            return []
        return [
            name
            for name, ids in node_to_sub_ids.items()
            if sid in ids and name in known_proxy_names
        ]
    if binding.target_type == "node_group":
        gid = binding.target_id
        if gid is None:
            return []
        # Only leaf proxy names, not nested group names.
        leaves = group_leaves.get(int(gid), [])
        return [n for n in leaves if n in known_proxy_names]
    return []


def _resolve_all_group_leaves(
    groups: list[NodeGroup], all_node_names: list[str]
) -> dict[int, list[str]]:
    mapping = {g.id: g for g in groups}
    group_names = {g.id: g.name for g in groups}
    name_to_id = {g.name: g.id for g in groups if g.name}
    cache: dict[int, list[str]] = {}

    def resolve(group_id: int, trail: set[int]) -> list[str]:
        if group_id in cache:
            return cache[group_id]
        if group_id in trail:
            return []
        trail.add(group_id)
        group = mapping.get(group_id)
        if not group:
            trail.remove(group_id)
            return []

        selected: list[str] = []
        excluded: set[str] = set(group.exclude_nodes or [])
        for entry in resolve_entries(group):
            entry_type = entry.get("type")
            entry_value = entry.get("value")
            if entry_type == "node":
                selected.append(str(entry_value))
            elif entry_type == "group_nodes":
                try:
                    child_id = int(entry_value)
                except Exception:
                    continue
                selected.extend(resolve(child_id, trail))
            elif entry_type == "exclude_group_nodes":
                try:
                    child_id = int(entry_value)
                except Exception:
                    continue
                if child_id == group_id:
                    continue
                excluded.update(resolve(child_id, set(trail)))
            elif entry_type == "group":
                # Inserted as group name in export; for dialer targets expand leaves.
                try:
                    ref_id = int(entry_value)
                except Exception:
                    # maybe already a name
                    ref_id = name_to_id.get(str(entry_value))
                if ref_id is not None:
                    selected.extend(resolve(ref_id, trail))
            elif entry_type == "regex":
                pattern_text = str(entry_value or "").strip()
                if not pattern_text:
                    continue
                try:
                    pattern = re.compile(pattern_text)
                except Exception:
                    continue
                selected.extend([n for n in all_node_names if pattern.search(n)])

        for raw_id in group.exclude_group_ids or []:
            try:
                exclude_id = int(raw_id)
            except Exception:
                continue
            if exclude_id == group_id:
                continue
            excluded.update(resolve(exclude_id, set(trail)))

        # Keep only real proxy node names (drop nested group labels if any slipped in).
        group_label_set = set(group_names.values())
        merged = [
            item
            for item in dedup_names(selected)
            if item not in excluded and item not in group_label_set
        ]
        cache[group_id] = merged
        trail.remove(group_id)
        return merged

    for gid in mapping:
        resolve(gid, set())
    return cache
