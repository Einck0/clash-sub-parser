"""Target configuration adapters for multi-client distribution.

This module provides deterministic rendering adapters for five core targets:
Clash, Mihomo (Clash.Meta), Stash, Shadowrocket, and Sing-box (1.14+).
Internal management tokens and non-connection metadata are stripped
to ensure secret minimization.
"""

from __future__ import annotations

import base64
from copy import deepcopy
import json
from typing import Any, Callable
from urllib.parse import quote
import yaml

from app.schemas.logical_config import CompilationResult, LogicalConfigurationBundle
from app.services.probe.converter import clash_to_singbox_outbound

# Keys that must never be leaked into exported client configurations
_INTERNAL_MGMT_KEYS = {
    "source_id",
    "source_logical_id",
    "subscription_id",
    "internal_id",
    "id",
    "admin_token",
    "management_token",
    "upstream_token",
    "secret_key",
    "token",
    "auth_token",
    "created_at",
    "updated_at",
    "raw_link",
    "fetch_url",
}


class _QuotedYamlString(str):
    """Marker string to force single-quote serialization in YAML."""

    pass


class _ClashYamlDumper(yaml.SafeDumper):
    """YAML safe dumper with support for quoted strings."""

    pass


def _represent_quoted_yaml_string(dumper: yaml.SafeDumper, value: Any) -> yaml.nodes.ScalarNode:
    """Represent quoted string as single-quoted scalar in YAML."""
    return dumper.represent_scalar("tag:yaml.org,2002:str", str(value), style="'")


_ClashYamlDumper.add_representer(_QuotedYamlString, _represent_quoted_yaml_string)


def _dump_clash_yaml(payload: dict[str, Any]) -> str:
    """Dump dictionary to YAML with single-quoted reality short-ids.

    Args:
        payload: Configuration dictionary to serialize.

    Returns:
        Formatted YAML string.
    """
    output = deepcopy(payload)
    for proxy in output.get("proxies") or []:
        if not isinstance(proxy, dict):
            continue
        reality_opts = proxy.get("reality-opts")
        if isinstance(reality_opts, dict):
            short_id = reality_opts.get("short-id")
            if isinstance(short_id, str):
                reality_opts["short-id"] = _QuotedYamlString(short_id)
    return yaml.dump(output, Dumper=_ClashYamlDumper, sort_keys=False, allow_unicode=True)


def sanitize_node_for_export(node: dict[str, Any]) -> dict[str, Any]:
    """Remove internal management fields while preserving proxy connection attributes.

    Args:
        node: Raw proxy node dictionary.

    Returns:
        Sanitized proxy node dictionary suitable for client export.
    """
    cleaned: dict[str, Any] = {}
    for k, v in node.items():
        if k in _INTERNAL_MGMT_KEYS:
            continue
        if isinstance(v, dict):
            cleaned[k] = sanitize_node_for_export(v)
        else:
            cleaned[k] = v
    return cleaned


def _build_clash_proxy_groups(result: CompilationResult) -> list[dict[str, Any]]:
    """Build standard Clash proxy-groups list from compilation result."""
    proxy_groups = []
    for g in result.groups:
        grp_dict: dict[str, Any] = {
            "name": g.name,
            "type": g.group_type,
            "proxies": g.resolved_node_names if g.resolved_node_names else ["DIRECT"],
        }
        raw_cfg = g.raw_config
        if g.group_type in ("url-test", "fallback", "load-balance"):
            url_cfg = raw_cfg.get("url_test_config") or {}
            grp_dict["url"] = url_cfg.get("url") or "https://cp.cloudflare.com/generate_204"
            grp_dict["interval"] = int(url_cfg.get("interval") or 300)
            if "tolerance" in url_cfg:
                grp_dict["tolerance"] = int(url_cfg["tolerance"])
        proxy_groups.append(grp_dict)
    return proxy_groups


def _build_clash_rules(result: CompilationResult) -> list[str]:
    """Build Clash rules list from compilation result."""
    rules_list = []
    for r in result.rules:
        rtype = str(r.get("type", "MATCH")).strip().upper()
        payload = str(r.get("payload", "")).strip()
        target = str(r.get("target", "DIRECT")).strip()
        if rtype == "MATCH":
            rules_list.append(f"MATCH,{target}")
        elif payload:
            rules_list.append(f"{rtype},{payload},{target}")
    return rules_list


def _attach_dns_config(config: dict[str, Any], bundle: LogicalConfigurationBundle) -> None:
    """Attach parsed DNS configuration to config dictionary if enabled."""
    if bundle.dns.enabled and bundle.dns.raw_yaml:
        try:
            dns_parsed = yaml.safe_load(bundle.dns.raw_yaml)
            if isinstance(dns_parsed, dict):
                if set(dns_parsed.keys()) == {"dns"} and isinstance(dns_parsed.get("dns"), dict):
                    config["dns"] = dns_parsed["dns"]
                else:
                    config["dns"] = dns_parsed
        except Exception:
            pass


def render_compiled_clash(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
    raw_nodes: list[dict[str, Any]],
) -> str:
    """Render standard Clash YAML configuration.

    Args:
        result: Compilation result from CanonicalGraphResolver.
        bundle: Logical configuration bundle.
        raw_nodes: List of proxy node dictionaries.

    Returns:
        Rendered YAML string for Clash.
    """
    cleaned_nodes = [sanitize_node_for_export(n) for n in raw_nodes]
    proxy_groups = _build_clash_proxy_groups(result)
    rules_list = _build_clash_rules(result)

    config: dict[str, Any] = {
        "port": 7890,
        "socks-port": 7891,
        "allow-lan": True,
        "mode": "rule",
        "log-level": "info",
        "proxies": cleaned_nodes,
        "proxy-groups": proxy_groups,
        "rules": rules_list,
    }

    _attach_dns_config(config, bundle)
    return _dump_clash_yaml(config)


def render_compiled_mihomo(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
    raw_nodes: list[dict[str, Any]],
) -> str:
    """Render Mihomo (Clash.Meta) YAML configuration.

    Preserves full protocol fields for Hysteria2, TUIC, WireGuard, and
    Meta extended rules such as GEOSITE and PROCESS-NAME.

    Args:
        result: Compilation result from CanonicalGraphResolver.
        bundle: Logical configuration bundle.
        raw_nodes: List of proxy node dictionaries.

    Returns:
        Rendered YAML string for Mihomo.
    """
    cleaned_nodes = [sanitize_node_for_export(n) for n in raw_nodes]
    proxy_groups = _build_clash_proxy_groups(result)
    rules_list = _build_clash_rules(result)

    config: dict[str, Any] = {
        "mixed-port": 7890,
        "allow-lan": True,
        "mode": "rule",
        "log-level": "info",
        "ipv6": True,
        "find-process-mode": "strict",
        "proxies": cleaned_nodes,
        "proxy-groups": proxy_groups,
        "rules": rules_list,
    }

    _attach_dns_config(config, bundle)
    return _dump_clash_yaml(config)


def render_compiled_stash(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
    raw_nodes: list[dict[str, Any]],
) -> str:
    """Render Stash compatible YAML configuration.

    Args:
        result: Compilation result from CanonicalGraphResolver.
        bundle: Logical configuration bundle.
        raw_nodes: List of proxy node dictionaries.

    Returns:
        Rendered YAML string for Stash.
    """
    cleaned_nodes = [sanitize_node_for_export(n) for n in raw_nodes]
    proxy_groups = _build_clash_proxy_groups(result)
    rules_list = _build_clash_rules(result)

    config: dict[str, Any] = {
        "proxies": cleaned_nodes,
        "proxy-groups": proxy_groups,
        "rules": rules_list,
    }

    _attach_dns_config(config, bundle)
    return _dump_clash_yaml(config)


def node_to_shadowrocket_link(node: dict[str, Any]) -> str | None:
    """Convert a single proxy node dictionary into a Shadowrocket compatible link.

    Args:
        node: Proxy node dictionary.

    Returns:
        Protocol URI string, or None if node cannot be serialized.
    """
    cleaned = sanitize_node_for_export(node)
    proto = str(cleaned.get("type") or cleaned.get("protocol") or "").strip().lower()
    server = str(cleaned.get("server") or "").strip()
    port = int(cleaned.get("port") or 443)
    name = str(cleaned.get("name") or f"{server}:{port}").strip()
    name_encoded = quote(name)

    if not server or port <= 0:
        return None

    # 1. Shadowsocks
    if proto in ("ss", "shadowsocks"):
        cipher = str(cleaned.get("cipher") or "aes-256-gcm").strip()
        pwd = str(cleaned.get("password") or "")
        userinfo = base64.b64encode(f"{cipher}:{pwd}".encode("utf-8")).decode("utf-8")
        link = f"ss://{userinfo}@{server}:{port}#{name_encoded}"
        plugin = cleaned.get("plugin")
        if plugin:
            plugin_opts = cleaned.get("plugin-opts") or {}
            opt_str = ";".join(f"{k}={v}" for k, v in plugin_opts.items()) if isinstance(plugin_opts, dict) else str(plugin_opts)
            link += f"?plugin={quote(f'{plugin};{opt_str}')}"
        return link

    # 2. VMess
    if proto == "vmess":
        uuid_str = str(cleaned.get("uuid") or "").strip()
        if not uuid_str:
            return None
        vmess_payload: dict[str, Any] = {
            "v": "2",
            "ps": name,
            "add": server,
            "port": port,
            "id": uuid_str,
            "aid": int(cleaned.get("alterId") or cleaned.get("alter_id") or 0),
            "scy": str(cleaned.get("cipher") or "auto"),
            "net": str(cleaned.get("network") or "tcp"),
            "type": "none",
            "host": str(cleaned.get("servername") or cleaned.get("sni") or ""),
            "tls": "tls" if cleaned.get("tls") else "",
        }
        ws_opts = cleaned.get("ws-opts") or cleaned.get("ws_opts") or {}
        if isinstance(ws_opts, dict) and ws_opts.get("path"):
            vmess_payload["path"] = str(ws_opts["path"])
        b64_json = base64.b64encode(json.dumps(vmess_payload).encode("utf-8")).decode("utf-8")
        return f"vmess://{b64_json}"

    # 3. Trojan
    if proto == "trojan":
        pwd = str(cleaned.get("password") or "").strip()
        if not pwd:
            return None
        sni = str(cleaned.get("sni") or cleaned.get("servername") or server).strip()
        params = []
        if sni:
            params.append(f"sni={quote(sni)}")
        if cleaned.get("skip-cert-verify"):
            params.append("allowInsecure=1")
        param_str = f"?{'&'.join(params)}" if params else ""
        return f"trojan://{pwd}@{server}:{port}{param_str}#{name_encoded}"

    # 4. VLESS
    if proto == "vless":
        uuid_str = str(cleaned.get("uuid") or "").strip()
        if not uuid_str:
            return None
        sni = str(cleaned.get("sni") or cleaned.get("servername") or server).strip()
        params = []
        if cleaned.get("tls"):
            params.append("security=tls")
        reality_opts = cleaned.get("reality-opts") or cleaned.get("reality_opts") or {}
        if isinstance(reality_opts, dict) and reality_opts.get("public-key"):
            params = ["security=reality"]
            params.append(f"pbk={reality_opts['public-key']}")
            if reality_opts.get("short-id"):
                params.append(f"sid={reality_opts['short-id']}")
        if sni:
            params.append(f"sni={quote(sni)}")
        flow = cleaned.get("flow")
        if flow:
            params.append(f"flow={quote(str(flow))}")
        net = cleaned.get("network")
        if net:
            params.append(f"type={quote(str(net))}")
        param_str = f"?{'&'.join(params)}" if params else ""
        return f"vless://{uuid_str}@{server}:{port}{param_str}#{name_encoded}"

    # 5. Hysteria2
    if proto in ("hysteria2", "hy2"):
        pwd = str(cleaned.get("password") or cleaned.get("auth") or "").strip()
        if not pwd:
            return None
        sni = str(cleaned.get("sni") or cleaned.get("servername") or server).strip()
        params = []
        if sni:
            params.append(f"sni={quote(sni)}")
        if cleaned.get("skip-cert-verify"):
            params.append("insecure=1")
        obfs = cleaned.get("obfs")
        if obfs:
            params.append(f"obfs={quote(str(obfs))}")
            obfs_pwd = cleaned.get("obfs-password") or cleaned.get("obfs_password")
            if obfs_pwd:
                params.append(f"obfs-password={quote(str(obfs_pwd))}")
        param_str = f"?{'&'.join(params)}" if params else ""
        return f"hysteria2://{pwd}@{server}:{port}{param_str}#{name_encoded}"

    # 6. TUIC
    if proto == "tuic":
        uuid_str = str(cleaned.get("uuid") or "").strip()
        pwd = str(cleaned.get("password") or "").strip()
        if not uuid_str or not pwd:
            return None
        sni = str(cleaned.get("sni") or cleaned.get("servername") or server).strip()
        cc = str(cleaned.get("congestion-controller") or cleaned.get("congestion_controller") or "").strip()
        params = []
        if sni:
            params.append(f"sni={quote(sni)}")
        if cc:
            params.append(f"congestion_control={quote(cc)}")
        param_str = f"?{'&'.join(params)}" if params else ""
        return f"tuic://{uuid_str}:{pwd}@{server}:{port}{param_str}#{name_encoded}"

    # 7. WireGuard
    if proto == "wireguard":
        priv_key = str(cleaned.get("private-key") or cleaned.get("private_key") or "").strip()
        pub_key = str(cleaned.get("public-key") or cleaned.get("public_key") or "").strip()
        if not priv_key or not pub_key:
            return None
        ip = str(cleaned.get("ip") or "10.0.0.2")
        params = [f"publickey={quote(pub_key)}", f"address={quote(ip)}"]
        psk = cleaned.get("preshared-key") or cleaned.get("preshared_key")
        if psk:
            params.append(f"presharedkey={quote(str(psk))}")
        return f"wireguard://{priv_key}@{server}:{port}?{'&'.join(params)}#{name_encoded}"

    # 8. HTTP / HTTPS
    if proto in ("http", "https"):
        user = cleaned.get("username") or ""
        pwd = cleaned.get("password") or ""
        auth_str = f"{user}:{pwd}@" if user or pwd else ""
        scheme = "https" if proto == "https" or cleaned.get("tls") else "http"
        return f"{scheme}://{auth_str}{server}:{port}#{name_encoded}"

    # 9. SOCKS5
    if proto in ("socks", "socks5"):
        user = cleaned.get("username") or ""
        pwd = cleaned.get("password") or ""
        auth_str = f"{user}:{pwd}@" if user or pwd else ""
        return f"socks5://{auth_str}{server}:{port}#{name_encoded}"

    return None


def render_compiled_shadowrocket(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
    raw_nodes: list[dict[str, Any]],
    as_ruleset: bool = False,
) -> str:
    """Render Shadowrocket subscription payload (base64 node links or ruleset).

    Args:
        result: Compilation result from CanonicalGraphResolver.
        bundle: Logical configuration bundle.
        raw_nodes: List of proxy node dictionaries.
        as_ruleset: When True, renders ruleset formatting instead of node links.

    Returns:
        Base64-encoded subscription string or ruleset string.
    """
    if as_ruleset:
        return render_compiled_shadowrocket_ruleset(result, bundle)

    lines: list[str] = []
    for node in raw_nodes:
        link = node_to_shadowrocket_link(node)
        if link:
            lines.append(link)

    raw_text = "\n".join(lines) + ("\n" if lines else "")
    return base64.b64encode(raw_text.encode("utf-8")).decode("utf-8")


def render_compiled_shadowrocket_ruleset(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
) -> str:
    """Render Shadowrocket ruleset routing rules format.

    Args:
        result: Compilation result from CanonicalGraphResolver.
        bundle: Logical configuration bundle.

    Returns:
        Ruleset format string for Shadowrocket.
    """
    lines: list[str] = []
    for r in result.rules:
        rtype = str(r.get("type", "MATCH")).strip().upper()
        payload = str(r.get("payload", "")).strip()
        target = str(r.get("target", "DIRECT")).strip()
        if rtype == "MATCH":
            lines.append(f"FINAL,{target}")
        elif payload:
            lines.append(f"{rtype},{payload},{target}")
    return "\n".join(lines) + ("\n" if lines else "")


def render_compiled_singbox(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
    raw_nodes: list[dict[str, Any]],
) -> str:
    """Render Sing-box 1.14+ JSON configuration.

    Converts normalized nodes to Sing-box outbounds, maps groups to selector
    and urltest outbounds, and maps rules to route.rules.

    Args:
        result: Compilation result from CanonicalGraphResolver.
        bundle: Logical configuration bundle.
        raw_nodes: List of proxy node dictionaries.

    Returns:
        Rendered JSON string for Sing-box.
    """
    cleaned_nodes = [sanitize_node_for_export(n) for n in raw_nodes]

    # Inbounds
    inbounds = [
        {
            "type": "mixed",
            "tag": "mixed-in",
            "listen": "127.0.0.1",
            "listen_port": 2080,
        }
    ]

    # Outbounds
    outbounds: list[dict[str, Any]] = [
        {"type": "direct", "tag": "DIRECT"},
        {"type": "block", "tag": "REJECT"},
    ]

    # Converted node outbounds
    for node in cleaned_nodes:
        tag_name = str(node.get("name") or "").strip()
        if not tag_name:
            continue
        sb_out = clash_to_singbox_outbound(node, tag=tag_name)
        if sb_out:
            outbounds.append(sb_out)

    # Selector and urltest outbounds
    for g in result.groups:
        tag_name = g.name
        proxies = list(g.resolved_node_names) if g.resolved_node_names else ["DIRECT"]
        if g.group_type in ("url-test", "fallback", "load-balance"):
            raw_cfg = g.raw_config.get("url_test_config") or {}
            url = raw_cfg.get("url") or "https://cp.cloudflare.com/generate_204"
            interval = raw_cfg.get("interval") or 300
            grp_out: dict[str, Any] = {
                "type": "urltest",
                "tag": tag_name,
                "outbounds": proxies,
                "url": url,
                "interval": f"{interval}s" if isinstance(interval, int) else str(interval),
            }
            if "tolerance" in raw_cfg:
                grp_out["tolerance"] = int(raw_cfg["tolerance"])
            outbounds.append(grp_out)
        else:
            outbounds.append({
                "type": "selector",
                "tag": tag_name,
                "outbounds": proxies,
            })

    # Route rules
    final_target = "DIRECT"
    route_rules: list[dict[str, Any]] = []
    for r in result.rules:
        rtype = str(r.get("type") or "MATCH").strip().upper()
        payload = str(r.get("payload") or "").strip()
        target = str(r.get("target") or "DIRECT").strip()
        if rtype == "MATCH":
            final_target = target
            continue

        rule_obj: dict[str, Any] = {"outbound": target}
        if rtype == "DOMAIN":
            rule_obj["domain"] = [payload]
        elif rtype == "DOMAIN-SUFFIX":
            rule_obj["domain_suffix"] = [payload]
        elif rtype == "DOMAIN-KEYWORD":
            rule_obj["domain_keyword"] = [payload]
        elif rtype == "DOMAIN-REGEX":
            rule_obj["domain_regex"] = [payload]
        elif rtype == "GEOSITE":
            rule_obj["geosite"] = [payload]
        elif rtype in ("IP-CIDR", "IP-CIDR6"):
            rule_obj["ip_cidr"] = [payload]
        elif rtype == "GEOIP":
            rule_obj["geoip"] = [payload]
        elif rtype == "PROCESS-NAME":
            rule_obj["process_name"] = [payload]
        elif rtype in ("PORT", "DST-PORT"):
            try:
                rule_obj["port"] = [int(payload)]
            except ValueError:
                continue
        elif rtype == "SRC-PORT":
            try:
                rule_obj["source_port"] = [int(payload)]
            except ValueError:
                continue
        else:
            # Other rule types can be preserved as domain_suffix if alphanumeric
            rule_obj["domain_suffix"] = [payload]

        route_rules.append(rule_obj)

    route_config = {
        "rules": route_rules,
        "final": final_target,
    }

    # DNS configuration
    dns_config: dict[str, Any] = {
        "servers": [
            {
                "tag": "dns-remote",
                "address": "https://1.1.1.1/dns-query",
                "detour": "DIRECT",
            },
            {
                "tag": "dns-direct",
                "address": "223.5.5.5",
                "detour": "DIRECT",
            },
        ],
        "rules": [
            {
                "outbound": "any",
                "server": "dns-direct",
            }
        ],
    }

    config: dict[str, Any] = {
        "inbounds": inbounds,
        "outbounds": outbounds,
        "route": route_config,
        "dns": dns_config,
    }

    return json.dumps(config, indent=2, ensure_ascii=False)


def render_compiled_yaml(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
    raw_nodes: list[dict[str, Any]],
) -> str:
    """Backward-compatible alias for Clash YAML rendering.

    Args:
        result: Compilation result from CanonicalGraphResolver.
        bundle: Logical configuration bundle.
        raw_nodes: List of proxy node dictionaries.

    Returns:
        Rendered YAML string for Clash.
    """
    return render_compiled_clash(result, bundle, raw_nodes)


TARGET_RENDERERS: dict[str, Callable[[CompilationResult, LogicalConfigurationBundle, list[dict[str, Any]]], str]] = {
    "clash": render_compiled_clash,
    "mihomo": render_compiled_mihomo,
    "stash": render_compiled_stash,
    "shadowrocket": render_compiled_shadowrocket,
    "sing-box": render_compiled_singbox,
    "yaml": render_compiled_clash,
}


def render_compiled_target(
    target: str,
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
    raw_nodes: list[dict[str, Any]],
) -> str:
    """Render compiled configuration according to target format.

    Args:
        target: Target identifier ('clash', 'mihomo', 'stash', 'shadowrocket', 'sing-box', 'yaml').
        result: Compilation result from CanonicalGraphResolver.
        bundle: Logical configuration bundle.
        raw_nodes: List of proxy node dictionaries.

    Returns:
        Formatted configuration string.

    Raises:
        ValueError: If target is not supported or is the deprecated SCRIPT format.
    """
    target_lower = str(target).strip().lower()
    if target_lower == "script":
        raise ValueError("SCRIPT format has been deprecated and removed")
    renderer = TARGET_RENDERERS.get(target_lower)
    if not renderer:
        raise ValueError(f"Unsupported compilation target: {target}")
    return renderer(result, bundle, raw_nodes)
