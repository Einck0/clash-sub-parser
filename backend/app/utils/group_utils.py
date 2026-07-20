"""Shared group resolution utilities used by generate_service and node_group_service."""

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
    """Append PASS only when enabled AND the group has no real nodes."""
    cleaned = [name for name in names if name and name != "PASS"]
    if not enabled:
        return cleaned
    # Fallback means: empty group gets PASS; non-empty groups stay as-is.
    if cleaned:
        return cleaned
    return ["PASS"]


def resolve_entries(group: NodeGroup) -> list[dict]:
    """Resolve group include_entries.

    Supports entry types:
    - node: static node name
    - group: reference another proxy-group by id (insert group name)
    - group_nodes: expand another group's resolved nodes
    - regex: virtual dynamic matcher (value is regex string). Not a frozen node.

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
    return fallback
