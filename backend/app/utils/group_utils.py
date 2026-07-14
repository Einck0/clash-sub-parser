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
    """Append REJECT as fallback node when enabled."""
    if not enabled:
        return names
    return [name for name in names if name != "REJECT"] + ["REJECT"]


def resolve_entries(group: NodeGroup) -> list[dict]:
    """Resolve group include_entries with fallback to legacy fields.

    Supports entry types:
    - node: static node name
    - group: reference another proxy-group by id (insert group name)
    - group_nodes: expand another group's resolved nodes
    - regex: virtual dynamic matcher (value is regex string). Not a frozen node.
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
    # Legacy regex_rules become virtual regex entries so ordering can be edited
    # in the unified entry list without freezing matched nodes.
    for rule in group.regex_rules or []:
        rule_text = str(rule or "").strip()
        if rule_text:
            fallback.append({"type": "regex", "value": rule_text})
    return fallback
