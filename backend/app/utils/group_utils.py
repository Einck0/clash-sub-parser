"""Shared group resolution utilities used by generate/chain/preview services."""

from __future__ import annotations

import re
from typing import Iterable

from app.models.node_group import NodeGroup


def dedup_names(items: list[str]) -> list[str]:
    """Deduplicate a list of names preserving order."""
    seen: set[str] = set()
    result: list[str] = []
    for item in items:
        value = str(item).strip()
        if not value or value in seen:
            continue
        seen.add(value)
        result.append(value)
    return result


def with_fallback(names: list[str], enabled: bool) -> list[str]:
    """Preserve explicit members and append PASS only when the group is empty."""
    cleaned = dedup_names([name for name in names if name])
    if not enabled or cleaned:
        return cleaned
    return ["PASS"]


def resolve_entries(group: NodeGroup) -> list[dict]:
    """Resolve group include_entries.

    Supports entry types:
    - node: static node name
    - group: reference another proxy-group by id (insert group name)
    - group_nodes: expand another group's resolved nodes
    - regex: virtual dynamic matcher (value is regex string). Not a frozen node.
    - exclude_group_nodes: expand another group's resolved nodes and subtract them (ordered)

    After migration, include_entries is the source of truth. Keep a tiny
    fallback only for completely empty groups with legacy include_* fields.
    """
    entries = list(group.include_entries or [])
    if entries:
        return entries

    fallback: list[dict] = []
    for name in group.include_nodes or []:
        fallback.append({"type": "node", "value": name})
    for group_id in group.include_group_ids or []:
        fallback.append({"type": "group", "value": group_id})
    for group_id in group.include_group_nodes_ids or []:
        fallback.append({"type": "group_nodes", "value": group_id})
    for group_id in group.exclude_group_ids or []:
        fallback.append({"type": "exclude_group_nodes", "value": group_id})
    return fallback


def resolve_group_members(
    groups: Iterable[NodeGroup],
    all_node_names: list[str],
    *,
    leaves_only: bool = False,
) -> dict[int, list[str]]:
    """Resolve every group's ordered member list from include_entries.

    Shared by generate, preview, and proxy-chain cycle checks.

    - leaves_only=False: export/preview mode — `group` entries keep nested
      group names as members (Clash proxy-group references).
    - leaves_only=True: expand nested groups to leaf proxy names only (for
      dialer target expansion / membership cycle checks).
    """
    group_list = list(groups)
    mapping = {g.id: g for g in group_list}
    name_to_id = {g.name: g.id for g in group_list if g.name}
    group_label_set = {g.name for g in group_list if g.name}
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
                try:
                    ref_id = int(entry_value)
                except Exception:
                    ref_id = name_to_id.get(str(entry_value))
                if ref_id is None:
                    continue
                if leaves_only:
                    selected.extend(resolve(ref_id, trail))
                elif ref_id in mapping:
                    selected.append(mapping[ref_id].name)
            elif entry_type == "regex":
                pattern_text = str(entry_value or "").strip()
                if not pattern_text:
                    continue
                try:
                    pattern = re.compile(pattern_text)
                except Exception:
                    continue
                selected.extend(
                    [name for name in all_node_names if name and pattern.search(name)]
                )

        # Legacy mirror field still honored if present.
        for raw_id in group.exclude_group_ids or []:
            try:
                exclude_id = int(raw_id)
            except Exception:
                continue
            if exclude_id == group_id:
                continue
            excluded.update(resolve(exclude_id, set(trail)))

        merged = [item for item in dedup_names(selected) if item not in excluded]
        if leaves_only:
            merged = [item for item in merged if item not in group_label_set]
        cache[group_id] = merged
        trail.remove(group_id)
        return merged

    for gid in mapping:
        resolve(gid, set())
    return cache
