import json
import re

import yaml
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.models.dns import DnsConfig
from app.models.node_group import NodeGroup
from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.models.subscription import Subscription
from app.utils.dedup import deduplicate_nodes

settings = get_settings()
BUILTIN_PROXIES = ["DIRECT", "PASS", "REJECT"]
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
}


def _default_switches(switches: dict | None) -> dict:
    data = switches or {}
    master = _as_bool(data.get("enabled", True))
    return {
        "enabled": master,
        "subscriptions": _as_bool(data.get("subscriptions", True)),
        "node_groups": _as_bool(data.get("node_groups", True)),
        "rules": _as_bool(data.get("rules", True)),
        "dns": _as_bool(data.get("dns", True)),
    }


def _as_bool(value) -> bool:
    if isinstance(value, bool):
        return value
    return str(value).strip().lower() in {"1", "true", "yes", "on"}


async def generate_yaml(db: AsyncSession, switches: dict | None = None) -> dict:
    s = _default_switches(switches)
    if not s["enabled"]:
        return {
            "yaml": yaml.safe_dump(
                {"proxies": [], "proxy-groups": [], "rules": []}, allow_unicode=True
            )
        }

    output: dict = {}

    all_nodes = await _collect_all_nodes(db)
    if s["subscriptions"]:
        output["proxies"] = all_nodes
    if s["node_groups"]:
        output["proxy-groups"] = await _collect_node_groups(db, all_nodes)
    if s["rules"]:
        output["rules"] = await _collect_rules(db)
    if s["dns"]:
        dns_obj = await _get_dns_object(db)
        if dns_obj:
            output["dns"] = dns_obj
        else:
            dns_raw = await _get_dns_raw(db)
            if dns_raw:
                output["dns_raw"] = dns_raw

    primary_comments = await _get_primary_comments(db)
    body = yaml.safe_dump(output, sort_keys=False, allow_unicode=True)
    if primary_comments:
        return {"yaml": "\n".join(primary_comments) + "\n" + body}
    return {"yaml": body}


async def generate_script(db: AsyncSession, switches: dict | None = None) -> dict:
    s = _default_switches(switches)
    data = switches or {}
    exclude_node_proxies = _as_bool(data.get("exclude_node_proxies", True))
    payload: dict = {"moduleSwitches": s, "proxyGroups": [], "rules": [], "dns": None}
    all_nodes = await _collect_all_nodes(db)

    if s["enabled"] and s["node_groups"]:
        payload["proxyGroups"] = await _collect_node_groups(
            db,
            [] if exclude_node_proxies else all_nodes,
        )
    if s["enabled"] and s["rules"]:
        payload["rules"] = await _collect_rules(db)
    if s["enabled"] and s["dns"]:
        payload["dns"] = await _get_dns_object(db)

    modules_json = json.dumps(payload, ensure_ascii=False, indent=2)
    script = "function main(params) {\n"
    script += "  const modules = " + modules_json.replace("\n", "\n  ") + ";\n"
    script += "  if (modules.moduleSwitches.enabled && modules.moduleSwitches.node_groups) {\n"
    script += "    params['proxy-groups'] = modules.proxyGroups;\n"
    script += "  }\n"
    script += (
        "  if (modules.moduleSwitches.enabled && modules.moduleSwitches.rules) {\n"
    )
    script += "    params.rules = modules.rules;\n"
    script += "  }\n"
    script += "  if (modules.moduleSwitches.enabled && modules.moduleSwitches.dns && modules.dns) {\n"
    script += "    params.dns = modules.dns;\n"
    script += "  }\n"
    script += "  return params;\n"
    script += "}\n"
    return {"script": script}


async def generate_subscription_payload(db: AsyncSession, subscription_id: int) -> dict:
    item = await db.get(Subscription, subscription_id)
    if not item:
        return {"yaml": ""}
    payload = {"proxies": item.raw_nodes or []}
    return {"yaml": yaml.safe_dump(payload, allow_unicode=True, sort_keys=False)}


async def _collect_all_nodes(db: AsyncSession) -> list[dict]:
    result = await db.execute(select(Subscription.raw_nodes))
    nodes: list[dict] = []
    for row in result.scalars().all():
        nodes.extend(row or [])
    return deduplicate_nodes(nodes)


async def _collect_node_groups(db: AsyncSession, all_nodes: list[dict]) -> list[dict]:
    result = await db.execute(
        select(NodeGroup).order_by(NodeGroup.sort_order.asc(), NodeGroup.id.asc())
    )
    groups = list(result.scalars().all())
    mapping = {group.id: group for group in groups}
    all_node_names = [node.get("name", "") for node in all_nodes if node.get("name")]

    resolved_cache: dict[int, list[str]] = {}

    def resolve_group_nodes(group_id: int, trail: set[int]) -> list[str]:
        if group_id in resolved_cache:
            return resolved_cache[group_id]
        if group_id in trail:
            return []
        trail.add(group_id)
        group = mapping.get(group_id)
        if not group:
            trail.remove(group_id)
            return []

        selected: list[str] = []

        entries = _resolve_entries(group)
        for entry in entries:
            entry_type = entry.get("type")
            entry_value = entry.get("value")
            if entry_type == "node":
                selected.append(str(entry_value))
            elif entry_type == "group_nodes":
                try:
                    child_id = int(entry_value)
                except Exception:
                    continue
                selected.extend(resolve_group_nodes(child_id, trail))
            elif entry_type == "group":
                try:
                    ref_id = int(entry_value)
                except Exception:
                    continue
                if ref_id in mapping:
                    selected.append(mapping[ref_id].name)

        for regex_rule in group.regex_rules or []:
            try:
                pattern = re.compile(regex_rule)
            except Exception:
                continue
            selected.extend([name for name in all_node_names if pattern.search(name)])

        excluded = set(group.exclude_nodes or [])
        merged = [item for item in _dedup_names(selected) if item not in excluded]
        resolved_cache[group_id] = merged
        trail.remove(group_id)
        return merged

    result_groups = []
    for group in mapping.values():
        proxies = _with_fallback(_dedup_names(resolve_group_nodes(group.id, set())), group.add_fallback)

        payload = {
            "name": group.name,
            "type": group.group_type,
            "proxies": proxies,
        }
        if group.group_type in {"url-test", "fallback", "load-balance"}:
            payload["url"] = (group.url_test_config or {}).get(
                "url", settings.default_proxy_test_url
            )
            payload["interval"] = int(
                (group.url_test_config or {}).get("interval", 300)
            )
            payload["tolerance"] = int(
                (group.url_test_config or {}).get("tolerance", 50)
            )
        result_groups.append(payload)
    return result_groups


async def _collect_rules(db: AsyncSession) -> list[str]:
    result = await db.execute(
        select(Rule)
        .outerjoin(RuleCategory, Rule.category == RuleCategory.name)
        .where(Rule.enabled.is_(True))
        .order_by(RuleCategory.sort_order.asc().nulls_last(), Rule.sort_order.asc(), Rule.id.asc())
    )
    lines: list[str] = []
    for rule in result.scalars().all():
        line = _emit_standard_rule(rule)
        if line:
            lines.append(line)
    return lines


def _emit_standard_rule(item) -> str:
    get_value = item.get if isinstance(item, dict) else lambda key, default=None: getattr(item, key, default)
    rule_type = str(get_value("type", "")).strip().upper()
    value = str(get_value("value", "")).strip()
    proxy = str(get_value("proxy", "")).strip()
    option_items = [str(opt).strip() for opt in get_value("options", []) if str(opt).strip()]

    if rule_type in MATCH_RULE_TYPES:
        if not proxy:
            return ""
        return ",".join([rule_type, proxy, *option_items])

    if not rule_type or not value or not proxy:
        return ""
    return ",".join([rule_type, value, proxy, *option_items])


async def _get_dns_raw(db: AsyncSession) -> str:
    result = await db.execute(select(DnsConfig).where(DnsConfig.id == 1))
    item = result.scalar_one_or_none()
    if item and item.enabled:
        return item.raw_yaml
    return ""


async def _get_dns_object(db: AsyncSession) -> dict | None:
    raw = await _get_dns_raw(db)
    if not raw:
        return None
    try:
        data = yaml.safe_load(raw)
        if isinstance(data, dict):
            return _unwrap_dns_object(data)
    except Exception:
        return None
    return None


def _unwrap_dns_object(data: dict) -> dict:
    if set(data.keys()) == {"dns"} and isinstance(data.get("dns"), dict):
        return data["dns"]
    return data


async def _get_primary_comments(db: AsyncSession) -> list[str]:
    result = await db.execute(
        select(Subscription.fetch_comments).where(Subscription.is_primary.is_(True))
    )
    comments = result.scalar_one_or_none()
    return comments or []


async def get_primary_subscription_headers(db: AsyncSession) -> dict[str, str]:
    result = await db.execute(
        select(
            Subscription.subscription_userinfo,
            Subscription.profile_update_interval,
            Subscription.profile_web_page_url,
        ).where(Subscription.is_primary.is_(True))
    )
    row = result.first()
    if not row:
        return {}
    subscription_userinfo, profile_update_interval, profile_web_page_url = row
    headers: dict[str, str] = {}
    if subscription_userinfo:
        headers["Subscription-Userinfo"] = subscription_userinfo
    if profile_update_interval:
        headers["Profile-Update-Interval"] = profile_update_interval
    if profile_web_page_url:
        headers["Profile-Web-Page-Url"] = profile_web_page_url
    return headers


def _dedup_names(items: list[str]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for item in items:
        value = str(item).strip()
        if not value or value in seen:
            continue
        seen.add(value)
        result.append(value)
    return result


def _with_fallback(names: list[str], enabled: bool) -> list[str]:
    if not enabled:
        return names
    return [name for name in names if name != "REJECT"] + ["REJECT"]


def _resolve_entries(group: NodeGroup) -> list[dict]:
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
