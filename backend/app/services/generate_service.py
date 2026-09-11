from copy import deepcopy
import json
from typing import Any, Literal
from urllib.parse import quote

from fastapi import HTTPException, Request
from fastapi.responses import PlainTextResponse
import yaml
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.models.dns import DnsConfig
from app.models.node_group import NodeGroup
from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.models.subscription import Subscription
from app.services.probe.service import get_all_db_probe_results
from app.utils.capability_filter import (
    deduplicate_nodes_by_key,
    get_probe_result_for_node,
    is_node_capability_qualified,
)
from app.utils.group_utils import resolve_group_members, with_fallback
from app.services.proxy_chain_service import apply_bindings_to_nodes

settings = get_settings()
BUILTIN_PROXIES = ["DIRECT", "PASS", "REJECT"]
MATCH_RULE_TYPES = {"MATCH"}


class _QuotedYamlString(str):
    pass


class _ClashYamlDumper(yaml.SafeDumper):
    pass


def _represent_quoted_yaml_string(dumper, value):
    return dumper.represent_scalar("tag:yaml.org,2002:str", str(value), style="'")


_ClashYamlDumper.add_representer(_QuotedYamlString, _represent_quoted_yaml_string)


def _dump_clash_yaml(payload: dict) -> str:
    output = deepcopy(payload)
    for proxy in output.get("proxies") or []:
        if not isinstance(proxy, dict):
            continue
        reality_opts = proxy.get("reality-opts")
        if not isinstance(reality_opts, dict):
            continue
        short_id = reality_opts.get("short-id")
        if isinstance(short_id, str):
            reality_opts["short-id"] = _QuotedYamlString(short_id)
    return yaml.dump(output, Dumper=_ClashYamlDumper, sort_keys=False, allow_unicode=True)


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
            ),
            "stats": {
                "proxies": 0,
                "proxy_groups": 0,
                "rules": 0,
                "dialer_proxy": 0,
                "dialer_samples": [],
            },
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

    proxies = output.get("proxies") or []
    dialered = [
        {"name": p.get("name"), "dialer-proxy": p.get("dialer-proxy")}
        for p in proxies
        if isinstance(p, dict) and p.get("dialer-proxy") and p.get("name")
    ]
    stats = {
        "proxies": len(proxies),
        "proxy_groups": len(output.get("proxy-groups") or []),
        "rules": len(output.get("rules") or []),
        "dialer_proxy": len(dialered),
        "dialer_samples": dialered[:8],
    }

    primary_comments = await _get_primary_comments(db)
    body = _dump_clash_yaml(output)
    if primary_comments:
        return {"yaml": "\n".join(primary_comments) + "\n" + body, "stats": stats}
    return {"yaml": body, "stats": stats}


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


async def generate_subscription_payload(
    db: AsyncSession,
    subscription_id: int,
    target: str = "clash",
) -> dict:
    """Generate configuration payload for a single subscription across targets.

    Args:
        db: Database session.
        subscription_id: Subscription database ID.
        target: Target identifier ('clash', 'mihomo', 'stash', 'shadowrocket', 'sing-box', 'yaml').

    Returns:
        Dictionary with content, target, and payload.

    Raises:
        HTTPException: If subscription not found or target invalid.
    """
    import base64
    target_lower = str(target).strip().lower()
    if target_lower == "script":
        raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")

    item = await db.get(Subscription, subscription_id)
    if not item:
        raise HTTPException(status_code=404, detail="Subscription not found")
    sub_nodes = list(item.raw_nodes or [])
    if item.filter_min_speed_mbps is not None or item.filter_media_unlock:
        probe_map = await get_all_db_probe_results(db)
        sub_nodes = [
            n for n in sub_nodes
            if is_node_capability_qualified(
                get_probe_result_for_node(probe_map, n),
                min_speed_mbps=item.filter_min_speed_mbps,
                required_media=item.filter_media_unlock,
            )
        ]
    nodes = await apply_bindings_to_nodes(db, sub_nodes)

    if target_lower in ("clash", "mihomo", "stash", "yaml"):
        payload = {"proxies": nodes}
        body = _dump_clash_yaml(payload)
        return {"yaml": body, "content": body, "target": target_lower}
    elif target_lower == "shadowrocket":
        from app.services.compiler_target_adapters import node_to_shadowrocket_link
        lines = [node_to_shadowrocket_link(n) for n in nodes]
        valid_lines = [line for line in lines if line]
        body = base64.b64encode(("\n".join(valid_lines) + ("\n" if valid_lines else "")).encode("utf-8")).decode("utf-8")
        return {"payload": body, "content": body, "target": "shadowrocket"}
    elif target_lower == "sing-box":
        from app.services.probe.converter import clash_to_singbox_outbound
        sb_outbounds = [{"type": "direct", "tag": "DIRECT"}, {"type": "block", "tag": "REJECT"}]
        for n in nodes:
            name = str(n.get("name") or "")
            if name:
                ob = clash_to_singbox_outbound(n, tag=name)
                if ob:
                    sb_outbounds.append(ob)
        cfg = {"outbounds": sb_outbounds}
        body = json.dumps(cfg, indent=2, ensure_ascii=False)
        return {"json": body, "content": body, "target": "sing-box"}
    else:
        raise HTTPException(status_code=400, detail=f"Unsupported target: {target}")


async def _collect_all_nodes(db: AsyncSession) -> list[dict]:
    result = await db.execute(
        select(Subscription).where(Subscription.enabled.is_(True))
    )
    probe_map = await get_all_db_probe_results(db)
    nodes: list[dict] = []
    for sub in result.scalars().all():
        sub_nodes = list(sub.raw_nodes or [])
        if sub.filter_min_speed_mbps is not None or sub.filter_media_unlock:
            sub_nodes = [
                n for n in sub_nodes
                if is_node_capability_qualified(
                    get_probe_result_for_node(probe_map, n),
                    min_speed_mbps=sub.filter_min_speed_mbps,
                    required_media=sub.filter_media_unlock,
                )
            ]
        nodes.extend(sub_nodes)
    return await apply_bindings_to_nodes(db, deduplicate_nodes_by_key(nodes))


async def _collect_node_groups(db: AsyncSession, all_nodes: list[dict]) -> list[dict]:
    result = await db.execute(
        select(NodeGroup).order_by(NodeGroup.sort_order.asc(), NodeGroup.id.asc())
    )
    groups = list(result.scalars().all())
    probe_map = await get_all_db_probe_results(db)
    resolved = resolve_group_members(groups, all_nodes, leaves_only=False, probe_map=probe_map)

    result_groups = []
    for group in groups:
        proxies = with_fallback(list(resolved.get(group.id, [])), group.add_fallback)

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
        select(Subscription.fetch_comments).where(
            Subscription.is_primary.is_(True),
            Subscription.enabled.is_(True),
        )
    )
    comments = result.scalar_one_or_none()
    return comments or []


async def get_primary_subscription_headers(db: AsyncSession) -> dict[str, str]:
    result = await db.execute(
        select(
            Subscription.subscription_userinfo,
            Subscription.profile_update_interval,
            Subscription.profile_web_page_url,
        ).where(
            Subscription.is_primary.is_(True),
            Subscription.enabled.is_(True),
        )
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


async def generate_target(db: AsyncSession, target: str, switches: dict | None = None) -> dict:
    """Generate configuration for a given target.

    Args:
        db: Database session.
        target: Target identifier ('clash', 'mihomo', 'stash', 'shadowrocket', 'sing-box', 'yaml').
        switches: Generation module switches.

    Returns:
        Dictionary with content, target, and stats.

    Raises:
        HTTPException: If target is deprecated or unsupported.
    """
    target_lower = str(target).strip().lower()
    if target_lower == "script":
        raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")

    if target_lower not in ("clash", "mihomo", "stash", "shadowrocket", "sing-box", "yaml"):
        raise HTTPException(status_code=400, detail=f"Unsupported target: {target}")

    s = _default_switches(switches)
    if not s["enabled"]:
        empty_content = ""
        if target_lower in ("clash", "mihomo", "stash", "yaml"):
            empty_content = yaml.safe_dump({"proxies": [], "proxy-groups": [], "rules": []}, allow_unicode=True)
        elif target_lower == "sing-box":
            empty_content = json.dumps({"inbounds": [], "outbounds": [], "route": {"rules": []}}, indent=2)
        elif target_lower == "shadowrocket":
            empty_content = ""
        return {
            "target": target_lower,
            "content": empty_content,
            "yaml": empty_content if target_lower in ("clash", "mihomo", "stash", "yaml") else "",
            "stats": {"proxies": 0, "proxy_groups": 0, "rules": 0, "dialer_proxy": 0, "dialer_samples": []},
        }

    if target_lower in ("clash", "yaml"):
        res = await generate_yaml(db, switches)
        res["target"] = target_lower
        res["content"] = res.get("yaml", "")
        return res

    all_nodes = await _collect_all_nodes(db)
    from app.services.legacy_config_importer import build_logical_bundle_from_db
    from app.services.canonical_compiler import CanonicalGraphResolver
    from app.services.compiler_target_adapters import render_compiled_target

    bundle = await build_logical_bundle_from_db(db, bundle_name="generated-run")
    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=all_nodes)
    comp_res = resolver.compile()

    rendered = render_compiled_target(target_lower, comp_res, bundle, all_nodes)

    primary_comments = await _get_primary_comments(db)
    if primary_comments and target_lower in ("mihomo", "stash"):
        rendered = "\n".join(primary_comments) + "\n" + rendered

    stats = {
        "proxies": len(all_nodes),
        "proxy_groups": len(comp_res.groups),
        "rules": len(comp_res.rules),
        "dialer_proxy": 0,
        "dialer_samples": [],
    }
    return {
        "target": target_lower,
        "content": rendered,
        "yaml": rendered if target_lower in ("mihomo", "stash") else "",
        "json": rendered if target_lower == "sing-box" else "",
        "payload": rendered if target_lower == "shadowrocket" else "",
        "stats": stats,
    }


async def render_current(db: AsyncSession, kind: str) -> str:
    """Render configuration for given target using current database state.

    Args:
        db: Database session.
        kind: Target kind ('clash', 'mihomo', 'stash', 'shadowrocket', 'sing-box', 'yaml').

    Returns:
        Rendered configuration string.

    Raises:
        HTTPException: If kind is deprecated or invalid.
    """
    from app.services.generate_config_service import generate_config_to_switches, get_generate_config

    kind_lower = str(kind).strip().lower()
    config = await get_generate_config(db)
    switches = generate_config_to_switches(config)
    if kind_lower == "script":
        res = await generate_script(db, switches)
        return str(res.get("script") or "")
    res = await generate_target(db, kind_lower, switches)
    return str(res.get("content") or res.get("yaml") or res.get("payload") or res.get("json") or "")


async def file_response(
    db: AsyncSession,
    content: str,
    kind: str,
    disposition: Literal["inline", "attachment"] = "inline",
) -> PlainTextResponse:
    """Build unified HTTP file response with Content-Disposition, media type, and subscription headers.

    Args:
        db: Database session.
        content: File body content.
        kind: Target kind ('clash', 'mihomo', 'stash', 'shadowrocket', 'sing-box', 'yaml').
        disposition: 'inline' or 'attachment'.

    Returns:
        PlainTextResponse with appropriate headers.

    Raises:
        HTTPException: If kind is deprecated or invalid.
    """
    kind_lower = str(kind).strip().lower()
    if kind_lower == "script":
        raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")

    if kind_lower in ("clash", "mihomo", "yaml"):
        filename = "config.yaml"
        media_type = "application/x-yaml"
    elif kind_lower == "stash":
        filename = "stash.yaml"
        media_type = "application/x-yaml"
    elif kind_lower == "shadowrocket":
        filename = "sub.txt"
        media_type = "text/plain; charset=utf-8"
    elif kind_lower == "sing-box":
        filename = "config.json"
        media_type = "application/json"
    else:
        raise HTTPException(status_code=400, detail=f"Unknown file kind: {kind}")

    headers = {
        "Content-Disposition": f'{disposition}; filename="{filename}"',
        "Cache-Control": "private, no-cache",
    }
    headers.update(await get_primary_subscription_headers(db))
    return PlainTextResponse(
        content=content,
        media_type=media_type,
        headers=headers,
    )


async def get_quick_export(
    db: AsyncSession,
    request: Request,
    subscription_id: int | None = None,
    target: str | None = None,
) -> dict:
    """Generate QuickExport payloads, client wake-up schemes, and QR payloads.

    Args:
        db: Database session.
        request: FastAPI HTTP request to resolve base URL and token.
        subscription_id: Optional ID for single-subscription export.
        target: Optional specific target filter.

    Returns:
        QuickExport payload containing URLs, schemes, and QR payloads for 5 targets.

    Raises:
        HTTPException: If subscription not found.
    """
    import base64

    base_url = str(request.base_url).rstrip("/")
    token = request.query_params.get("token")

    target_defs = [
        {"target": "clash", "name": "Clash", "ext": "yaml"},
        {"target": "mihomo", "name": "Mihomo", "ext": "yaml"},
        {"target": "stash", "name": "Stash", "ext": "yaml"},
        {"target": "shadowrocket", "name": "Shadowrocket", "ext": "txt"},
        {"target": "sing-box", "name": "Sing-box", "ext": "json"},
    ]

    sub_name: str | None = None
    if subscription_id is not None:
        sub = await db.get(Subscription, subscription_id)
        if not sub:
            raise HTTPException(status_code=404, detail="Subscription not found")
        scope = "subscription"
        sub_name = sub.name or f"Sub-{subscription_id}"
    else:
        scope = "merged"
        sub_name = "ClashSubParser"

    items: list[dict[str, Any]] = []
    for td in target_defs:
        t_key = td["target"]
        t_name = td["name"]

        if scope == "subscription":
            q_parts = [f"target={t_key}"]
            if token:
                q_parts.append(f"token={token}")
            q_str = "&".join(q_parts)
            export_url = f"{base_url}/api/generate/subscription/{subscription_id}?{q_str}"
        else:
            q_str = f"?token={token}" if token else ""
            export_url = f"{base_url}/api/generate/{t_key}{q_str}"

        # Build client scheme URL
        if t_key in ("clash", "mihomo"):
            scheme_url = f"clash://install-config?url={quote(export_url, safe='')}&name={quote(sub_name)}"
            qr_payload = export_url
        elif t_key == "stash":
            scheme_url = f"stash://install-config?url={quote(export_url, safe='')}&name={quote(sub_name)}"
            qr_payload = export_url
        elif t_key == "shadowrocket":
            b64_sub = base64.b64encode(export_url.encode("utf-8")).decode("utf-8")
            scheme_url = f"sub://{b64_sub}"
            qr_payload = scheme_url
        elif t_key == "sing-box":
            scheme_url = f"sing-box://import-remote-profile?url={quote(export_url, safe='')}#{quote(sub_name)}"
            qr_payload = export_url
        else:
            scheme_url = export_url
            qr_payload = export_url

        items.append({
            "target": t_key,
            "name": t_name,
            "url": export_url,
            "scheme_url": scheme_url,
            "qrcode_payload": qr_payload,
        })

    targets_dict = {it["target"]: it for it in items}
    response_data: dict[str, Any] = {
        "scope": scope,
        "subscription_id": subscription_id,
        "subscription_name": sub_name if scope == "subscription" else None,
        "targets": targets_dict,
        "items": items,
    }

    if target:
        t_lower = str(target).strip().lower()
        if t_lower in targets_dict:
            response_data["selected_target"] = targets_dict[t_lower]

    return response_data
