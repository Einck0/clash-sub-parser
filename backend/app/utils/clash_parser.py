import json
import re
from urllib.parse import parse_qs, unquote, urlparse

import yaml

from app.utils.base64_decode import try_base64_decode


def parse_subscription_content(raw_content: str) -> tuple[list[dict], list[str]]:
    decoded = try_base64_decode(raw_content)
    comments = _extract_header_comments(decoded)

    proxies = _parse_yaml_proxies(decoded)
    if proxies:
        return proxies, comments

    links = parse_node_links(decoded)
    if links:
        return links, comments

    raise ValueError("Unable to parse subscription content as YAML or protocol links")


def parse_node_links(raw_content: str) -> list[dict]:
    """Parse one or more share links into Clash-compatible proxy dicts."""
    return _parse_protocol_links(raw_content)


def _extract_header_comments(content: str) -> list[str]:
    comments: list[str] = []
    for line in content.splitlines():
        stripped = line.strip()
        if stripped.startswith("#"):
            comments.append(line)
            continue
        if stripped == "":
            if comments:
                comments.append(line)
                continue
        break
    return comments


def _parse_yaml_proxies(content: str) -> list[dict]:
    try:
        data = yaml.safe_load(content)
    except Exception:
        return []

    if not isinstance(data, dict):
        return []
    proxies = data.get("proxies", [])
    if isinstance(proxies, list):
        normalized = [proxy for proxy in proxies if isinstance(proxy, dict)]
        return normalized
    return []


def _parse_protocol_links(content: str) -> list[dict]:
    nodes: list[dict] = []
    for line in content.splitlines():
        item = line.strip()
        if not item:
            continue
        if item.startswith("ss://"):
            parsed = _parse_ss(item)
        elif item.startswith("trojan://"):
            parsed = _parse_trojan(item)
        elif item.startswith("vless://"):
            parsed = _parse_vless(item)
        elif item.startswith("vmess://"):
            parsed = _parse_vmess(item)
        else:
            parsed = None
        if parsed:
            nodes.append(parsed)
    return nodes


def _parse_ss(link: str) -> dict | None:
    try:
        no_scheme = link[5:]
        body, _, tag = no_scheme.partition("#")
        base_and_host = body.split("@")
        if len(base_and_host) != 2:
            return None
        userinfo = base_and_host[0]
        server_port = base_and_host[1].split("?")[0]

        decoded = try_base64_decode(userinfo)
        method, password = decoded.split(":", 1)
        server, port = server_port.split(":", 1)
        return {
            "name": unquote(tag) if tag else f"ss-{server}:{port}",
            "type": "ss",
            "server": server,
            "port": int(port),
            "cipher": method,
            "password": password,
        }
    except Exception:
        return None


def _parse_trojan(link: str) -> dict | None:
    try:
        parsed = urlparse(link)
        server = parsed.hostname or ""
        port = parsed.port or 443
        password = parsed.username or ""
        query = parse_qs(parsed.query)
        network = _pick_first(query, "type") or "tcp"
        node = {
            "name": unquote(parsed.fragment) if parsed.fragment else f"trojan-{server}:{port}",
            "type": "trojan",
            "server": server,
            "port": port,
            "password": password,
            "network": network,
            "sni": _pick_first(query, "sni", "host"),
            "skip-cert-verify": _pick_first(query, "allowInsecure", "allow_insecure") in {"1", "true", "True"},
        }
        if network == "ws":
            ws_opts = {}
            if path := _pick_first(query, "path"):
                ws_opts["path"] = path
            if host := _pick_first(query, "host"):
                ws_opts["headers"] = {"Host": host}
            if ws_opts:
                node["ws-opts"] = ws_opts
        return node
    except Exception:
        return None


def _parse_vless(link: str) -> dict | None:
    try:
        parsed = urlparse(link)
        query = parse_qs(parsed.query)
        server = parsed.hostname or ""
        port = parsed.port or 443
        security = (_pick_first(query, "security") or "").lower()
        network = _pick_first(query, "type") or "tcp"
        node = {
            "name": unquote(parsed.fragment) if parsed.fragment else f"vless-{server}:{port}",
            "type": "vless",
            "server": server,
            "port": port,
            "uuid": parsed.username or "",
            "network": network,
            "tls": security in {"tls", "reality"},
        }

        if flow := _pick_first(query, "flow"):
            node["flow"] = flow
        if sni := _pick_first(query, "sni", "peer"):
            node["servername"] = sni
        if fp := _pick_first(query, "fp"):
            node["client-fingerprint"] = fp
        if _pick_first(query, "allowInsecure", "allow_insecure") in {"1", "true", "True"}:
            node["skip-cert-verify"] = True
        if security == "reality":
            reality_opts = {}
            if pbk := _pick_first(query, "pbk", "public-key", "publicKey"):
                reality_opts["public-key"] = pbk
            if sid := _pick_first(query, "sid", "short-id", "shortId"):
                reality_opts["short-id"] = sid
            if spx := _pick_first(query, "spx"):
                reality_opts["spider-x"] = spx
            if reality_opts:
                node["reality-opts"] = reality_opts

        if network == "ws":
            ws_opts = {}
            if path := _pick_first(query, "path"):
                ws_opts["path"] = path
            if host := _pick_first(query, "host"):
                ws_opts["headers"] = {"Host": host}
            if ws_opts:
                node["ws-opts"] = ws_opts
        elif network == "grpc":
            grpc_opts = {}
            if service_name := _pick_first(query, "serviceName", "service-name"):
                grpc_opts["grpc-service-name"] = service_name
            if grpc_opts:
                node["grpc-opts"] = grpc_opts

        return node
    except Exception:
        return None


def _parse_vmess(link: str) -> dict | None:
    try:
        payload = try_base64_decode(link[8:])
        data = json.loads(payload)
        network = data.get("net", "tcp")
        node = {
            "name": data.get("ps") or f"vmess-{data.get('add')}:{data.get('port')}",
            "type": "vmess",
            "server": data.get("add"),
            "port": int(data.get("port", 443)),
            "uuid": data.get("id"),
            "alterId": int(data.get("aid", 0)),
            "cipher": data.get("scy", "auto"),
            "network": network,
            "tls": str(data.get("tls", "")).lower() == "tls",
            "servername": data.get("sni") or data.get("host"),
        }
        # Transport layer parameters
        if network == "ws":
            ws_opts = {}
            if path := data.get("path"):
                ws_opts["path"] = path
            if host := data.get("host"):
                ws_opts["headers"] = {"Host": host}
            if ws_opts:
                node["ws-opts"] = ws_opts
        elif network == "grpc":
            grpc_opts = {}
            if service_name := data.get("path") or data.get("serviceName"):
                grpc_opts["grpc-service-name"] = service_name
            if grpc_opts:
                node["grpc-opts"] = grpc_opts
        elif network in ("h2", "http"):
            h2_opts = {}
            if path := data.get("path"):
                h2_opts["path"] = path
            if host := data.get("host"):
                h2_opts["host"] = [host]
            if h2_opts:
                node["h2-opts"] = h2_opts
        return node
    except Exception:
        return None


def _pick_first(query: dict[str, list[str]], *keys: str) -> str | None:
    for key in keys:
        values = query.get(key)
        if values:
            return values[0]
    return None


def compile_regex(regex_list: list[str]) -> list[re.Pattern]:
    return [re.compile(item) for item in regex_list if item]
