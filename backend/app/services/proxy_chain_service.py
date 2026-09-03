from __future__ import annotations

from typing import Any

from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.node_group import NodeGroup
from app.models.proxy_chain import ProxyChainBinding
from app.models.subscription import Subscription
from app.schemas.proxy_chain import ProxyChainBindingCreate, ProxyChainBindingUpdate
from app.utils.group_utils import resolve_group_members

FORBIDDEN_DIALERS = frozenset({"DIRECT", "REJECT", "PASS"})
TARGET_PRIORITY = {"subscription": 1, "node_group": 2, "node": 3}


async def _load_chain_world(
    db: AsyncSession,
    *,
    all_node_names: list[str] | None = None,
    known_nodes_extra: set[str] | None = None,
    order_groups: bool = True,
) -> dict[str, Any]:
    """加载链式绑定共用的订阅节点与策略组叶子

    台账预览环检测应用绑定都走这里, 避免各写一套扫库逻辑
    """
    sub_result = await db.execute(select(Subscription).where(Subscription.enabled.is_(True)))
    subs = list(sub_result.scalars().all())
    node_to_sub_ids: dict[str, set[int]] = {}
    names_from_subs: list[str] = []
    for sub in subs:
        for node in sub.raw_nodes or []:
            if not isinstance(node, dict):
                continue
            name = str(node.get("name") or "").strip()
            if not name:
                continue
            names_from_subs.append(name)
            node_to_sub_ids.setdefault(name, set()).add(sub.id)

    if order_groups:
        group_result = await db.execute(
            select(NodeGroup).order_by(NodeGroup.sort_order.asc(), NodeGroup.id.asc())
        )
    else:
        group_result = await db.execute(select(NodeGroup))
    groups = list(group_result.scalars().all())

    if all_node_names is None:
        resolve_names = list(names_from_subs)
    else:
        resolve_names = list(all_node_names)

    group_leaves = resolve_group_members(groups, resolve_names, leaves_only=True)
    known_proxy_names = {n for n in resolve_names if n}
    if known_nodes_extra:
        known_proxy_names |= set(known_nodes_extra)
    known_group_names = {g.name for g in groups if g.name}

    return {
        "subs": subs,
        "groups": groups,
        "node_to_sub_ids": node_to_sub_ids,
        "all_node_names": names_from_subs,
        "group_leaves": group_leaves,
        "known_proxy_names": known_proxy_names,
        "known_group_names": known_group_names,
    }



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
        "target_id": data.get("target_id", item.target_id),
        "target_name": data.get("target_name", item.target_name),
        "dialer_type": data.get("dialer_type", item.dialer_type),
        "dialer_ref": data.get("dialer_ref", item.dialer_ref),
        "enabled": data.get("enabled", item.enabled),
        "sort_order": data.get("sort_order", item.sort_order),
        "note": data.get("note", item.note),
    }
    # 目标类型变更时清理无关键
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
    # 类型切换后重置多余字段
    if item.target_type == "node":
        item.target_id = None
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def delete_binding(db: AsyncSession, item: ProxyChainBinding) -> None:
    await db.delete(item)
    await db.commit()


def _node_meta(node: dict[str, Any]) -> dict[str, Any]:
    port = node.get("port")
    try:
        port_val = int(port) if port is not None and str(port).strip() != "" else None
    except Exception:
        port_val = None
    tls = node.get("tls")
    if isinstance(tls, str):
        tls_val = tls.strip().lower() in {"1", "true", "yes", "on"}
    elif tls is None:
        tls_val = None
    else:
        tls_val = bool(tls)
    udp = node.get("udp")
    if isinstance(udp, str):
        udp_val = udp.strip().lower() in {"1", "true", "yes", "on"}
    elif udp is None:
        udp_val = None
    else:
        udp_val = bool(udp)
    return {
        "type": str(node.get("type") or "") or None,
        "server": str(node.get("server") or "") or None,
        "port": port_val,
        "udp": udp_val,
        "cipher": str(node.get("cipher") or "") or None,
        "network": str(node.get("network") or node.get("net") or "") or None,
        "tls": tls_val,
        "sni": str(node.get("sni") or node.get("servername") or "") or None,
    }


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
                    **_node_meta(node),
                }
            )
    return out


async def list_node_ledger(db: AsyncSession) -> list[dict[str, Any]]:
    """汇总最终节点、前置代理绑定与所属策略组"""
    nodes = await list_final_nodes(db)
    if not nodes:
        return []

    # 构建代理字典并附加 dialer-proxy
    proxies = [{"name": n["name"]} for n in nodes]
    applied = await apply_bindings_to_nodes(db, proxies)
    dialer_by_name = {
        str(p.get("name") or "").strip(): str(p.get("dialer-proxy") or "").strip() or None
        for p in applied
        if isinstance(p, dict) and p.get("name")
    }

    # 标记每个节点的生效绑定来源范围
    bindings = [b for b in await list_bindings(db) if b.enabled]
    world = await _load_chain_world(db)
    node_to_sub_ids = world["node_to_sub_ids"]
    groups = world["groups"]
    group_leaves = world["group_leaves"]
    known_proxy_names = world["known_proxy_names"]
    source_by_name: dict[str, str] = {}
    ordered = sorted(
        bindings,
        key=lambda b: (TARGET_PRIORITY.get(b.target_type, 0), b.sort_order, b.id or 0),
    )
    for binding in ordered:
        targets = _expand_targets(
            binding,
            node_to_sub_ids=node_to_sub_ids,
            group_leaves=group_leaves,
            known_proxy_names=known_proxy_names,
        )
        for name in targets:
            source_by_name[name] = binding.target_type

    # 反向映射：节点至包含该节点的策略组列表
    groups_by_node: dict[str, list[str]] = {}
    for group in groups:
        for leaf in group_leaves.get(group.id, []):
            groups_by_node.setdefault(leaf, []).append(group.name)

    out: list[dict[str, Any]] = []
    for item in nodes:
        name = item["name"]
        dialer = dialer_by_name.get(name)
        out.append(
            {
                **item,
                "dialer_proxy": dialer,
                "chain_source": source_by_name.get(name) if dialer else None,
                "group_names": groups_by_node.get(name, []),
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

    # 前置代理软校验，完全未知时提示错误
    known_nodes, known_groups = await _known_names(db)
    if data["dialer_type"] == "node" and dialer_ref not in known_nodes:
        raise HTTPException(status_code=400, detail=f"dialer node not found: {dialer_ref}")
    if data["dialer_type"] == "node_group" and dialer_ref not in known_groups:
        raise HTTPException(status_code=400, detail=f"dialer group not found: {dialer_ref}")

    # 成员与前置拨号节点循环校验，目标叶子节点不得包含或依赖前置拨号
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
    """将生效的前置代理绑定应用到最终节点列表"""
    if not nodes:
        return nodes

    bindings = [b for b in await list_bindings(db) if b.enabled]
    if not bindings:
        # 清理多余的前置代理字段
        cleaned = []
        for node in nodes:
            if not isinstance(node, dict):
                continue
            copied = dict(node)
            copied.pop("dialer-proxy", None)
            cleaned.append(copied)
        return cleaned

    # 最终节点名来自入参 proxies，订阅归属仍从数据库扫描
    all_node_names = [
        str(n.get("name") or "").strip()
        for n in nodes
        if isinstance(n, dict) and n.get("name")
    ]
    world = await _load_chain_world(db, all_node_names=all_node_names)
    node_to_sub_ids = world["node_to_sub_ids"]
    groups = world["groups"]
    group_leaves = world["group_leaves"]
    known_proxy_names = world["known_proxy_names"]
    known_group_names = world["known_group_names"]

    # 记录生效的优先级与前置拨号
    effective: dict[str, tuple[int, str]] = {}

    # 低优先级优先应用，高优先级覆盖
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
        # 跳过与前置组构成成员环路的节点
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

    # 环检测用当前启用订阅与策略组叶子
    world = await _load_chain_world(
        db,
        known_nodes_extra=set(known_nodes),
        order_groups=False,
    )
    node_to_sub_ids = world["node_to_sub_ids"]
    groups = world["groups"]
    group_leaves = world["group_leaves"]
    known_proxy_names = world["known_proxy_names"]

    # 构造临时对象用于目标展开
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
        # 目标类型为单节点且与前置拨号相同时拒绝
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
        # 目标策略组不能将自身作为前置拨号
        if data.get("target_type") == "node_group" and data.get("target_id") == g.id:
            raise HTTPException(
                status_code=400,
                detail=f"会形成环：策略组「{dialer_ref}」不能把自己当跳板",
            )
        leaves = set(group_leaves.get(g.id, []))
        overlap = targets & leaves
        safe = targets - leaves
        # 当所有目标节点均属于跳板组时硬性拒绝
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
        # 仅包含叶子代理节点名，不包含嵌套策略组名
        leaves = group_leaves.get(int(gid), [])
        return [n for n in leaves if n in known_proxy_names]
    return []


async def preview_binding_effect(
    db: AsyncSession,
    *,
    target_type: str,
    target_id: int | None = None,
    target_name: str | None = None,
    dialer_type: str,
    dialer_ref: str,
) -> dict[str, Any]:
    """预览草稿绑定的前置代理与跳过效果"""
    world = await _load_chain_world(db)
    node_to_sub_ids = world["node_to_sub_ids"]
    groups = world["groups"]
    group_leaves = world["group_leaves"]
    known_proxy_names = world["known_proxy_names"]

    class _Tmp:
        pass

    tmp = _Tmp()
    tmp.target_type = target_type
    tmp.target_id = target_id
    tmp.target_name = target_name
    targets = _expand_targets(
        tmp,  # type: ignore[arg-type]
        node_to_sub_ids=node_to_sub_ids,
        group_leaves=group_leaves,
        known_proxy_names=known_proxy_names,
    )
    dialer = str(dialer_ref or "").strip()
    skipped: list[str] = []
    chained: list[str] = []
    if dialer_type == "node_group":
        dialer_group_id = next((g.id for g in groups if g.name == dialer), None)
        dialer_leaves = (
            set(group_leaves.get(dialer_group_id, []))
            if dialer_group_id is not None
            else set()
        )
        for name in targets:
            if name == dialer or name in dialer_leaves:
                skipped.append(name)
            else:
                chained.append(name)
    else:
        for name in targets:
            if name == dialer:
                skipped.append(name)
            else:
                chained.append(name)

    return {
        "target_count": len(targets),
        "chain_count": len(chained),
        "skip_count": len(skipped),
        "chain_samples": chained[:12],
        "skip_samples": skipped[:12],
        "dialer_ref": dialer,
        "dialer_type": dialer_type,
    }
