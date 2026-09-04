"""Clash 节点到 sing-box outbound 配置转换器.

支持将主流代理节点（Shadowsocks、VMess、VLESS Reality、Trojan、Hysteria2、TUIC、HTTP 与 SOCKS5）
精确转换为 sing-box 1.14+ 合法的 outbound JSON 结构，并提供详尽的容错回退与参数校验。
"""

from __future__ import annotations

import base64
import json
import logging
import re
from typing import Any

logger = logging.getLogger(__name__)


def _is_valid_base64(val: str) -> bool:
    """Check whether a string is valid Base64."""
    if not val or not isinstance(val, str):
        return False
    normalized = val.replace("-", "+").replace("_", "/")
    pad = len(normalized) % 4
    if pad:
        normalized += "=" * (4 - pad)
    try:
        decoded = base64.b64decode(normalized)
        return len(decoded) > 0
    except Exception:
        return False


def _normalize_alpn(val: Any) -> list[str] | None:
    """Normalize ALPN configuration into a list of strings."""
    if not val:
        return None
    if isinstance(val, list):
        items = [str(x).strip() for x in val if x and str(x).strip()]
        return items if items else None
    if isinstance(val, str):
        items = [s.strip() for s in val.split(",") if s.strip()]
        return items if items else None
    return None


def _parse_mbps(val: Any) -> int | None:
    """Parse bandwidth values like '100 Mbps', '100M', or 100 into integer Mbps."""
    if val is None:
        return None
    try:
        if isinstance(val, (int, float)):
            return int(val)
        s = str(val).strip().lower()
        match = re.match(r"^(\d+)", s)
        if match:
            return int(match.group(1))
    except Exception:
        pass
    return None


def _extract_port(node: dict[str, Any]) -> int:
    """Extract and validate port, supporting port-hopping syntax in ports/mport."""
    try:
        port = int(node.get("port") or 0)
        if 0 < port <= 65535:
            return port
    except Exception:
        pass

    ports_val = node.get("ports") or node.get("mport")
    if ports_val:
        try:
            first_part = str(ports_val).split(",")[0].strip()
            if "-" in first_part:
                p = int(first_part.split("-")[0].strip())
            else:
                p = int(first_part)
            if 0 < p <= 65535:
                return p
        except Exception:
            pass
    return 0


def clash_to_singbox_outbound(node: dict[str, Any], tag: str = "proxy-out") -> dict[str, Any] | None:
    """Convert a single Clash proxy node dictionary to sing-box outbound configuration.

    Args:
        node: Clash proxy node dictionary.
        tag: Outbound tag identifier.

    Returns:
        sing-box outbound dictionary, or None if validation fails or protocol unsupported.
    """
    if not isinstance(node, dict):
        logger.warning("clash_to_singbox_outbound received invalid node (not a dict): %r", type(node))
        return None

    node_type = str(node.get("type") or "").strip().lower()
    node_name = str(node.get("name") or "unnamed").strip()
    server = str(node.get("server") or "").strip()
    server_port = _extract_port(node)

    # 1. Shadowsocks
    if node_type in ("ss", "shadowsocks"):
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("Shadowsocks node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        cipher = str(node.get("cipher") or "").strip()
        password = str(node.get("password") or "")
        if not cipher:
            logger.warning("Shadowsocks node '%s' missing cipher", node_name)
            return None
        if not password:
            logger.warning("Shadowsocks node '%s' missing password", node_name)
            return None
        outbound: dict[str, Any] = {
            "type": "shadowsocks",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "method": cipher,
            "password": password,
        }
        plugin = node.get("plugin")
        plugin_opts = node.get("plugin-opts") or node.get("plugin_opts")
        if plugin:
            outbound["plugin"] = str(plugin)
        if plugin_opts:
            outbound["plugin_opts"] = plugin_opts if isinstance(plugin_opts, str) else json.dumps(plugin_opts)
        return outbound

    # 2. VMess
    if node_type == "vmess":
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("VMess node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        uuid = str(node.get("uuid") or "").strip()
        if not uuid:
            logger.warning("VMess node '%s' missing uuid", node_name)
            return None
        alter_id = int(node.get("alterId") or node.get("alter_id") or 0)
        security = str(node.get("cipher") or "auto").strip() or "auto"
        outbound = {
            "type": "vmess",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "uuid": uuid,
            "alter_id": alter_id,
            "security": security,
        }
        # TLS configuration
        tls_enabled = bool(node.get("tls"))
        sni = str(node.get("servername") or node.get("sni") or node.get("server_name") or "").strip()
        if tls_enabled or sni:
            tls_conf: dict[str, Any] = {"enabled": True}
            if sni:
                tls_conf["server_name"] = sni
            if node.get("skip-cert-verify") or node.get("skip_cert_verify") or node.get("insecure"):
                tls_conf["insecure"] = True
            alpn = _normalize_alpn(node.get("alpn"))
            if alpn:
                tls_conf["alpn"] = alpn
            outbound["tls"] = tls_conf

        # Transport configuration (ws, grpc, http)
        network = str(node.get("network") or "tcp").strip().lower()
        if network == "ws":
            ws_opts = node.get("ws-opts") or node.get("ws_opts") or {}
            transport: dict[str, Any] = {"type": "ws"}
            if isinstance(ws_opts, dict):
                path = ws_opts.get("path")
                if path:
                    transport["path"] = str(path)
                headers = ws_opts.get("headers")
                if isinstance(headers, dict) and headers:
                    transport["headers"] = {str(k): str(v) for k, v in headers.items()}
            outbound["transport"] = transport
        elif network == "grpc":
            grpc_opts = node.get("grpc-opts") or node.get("grpc_opts") or {}
            transport = {"type": "grpc"}
            if isinstance(grpc_opts, dict) and grpc_opts.get("grpc-service-name"):
                transport["service_name"] = str(grpc_opts["grpc-service-name"])
            outbound["transport"] = transport
        elif network in ("http", "h2"):
            http_opts = node.get("http-opts") or node.get("http_opts") or {}
            transport = {"type": "http"}
            if isinstance(http_opts, dict):
                path = http_opts.get("path")
                if isinstance(path, list) and path:
                    transport["path"] = str(path[0])
                elif isinstance(path, str) and path:
                    transport["path"] = path
            outbound["transport"] = transport
        return outbound

    # 3. VLESS
    if node_type == "vless":
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("VLESS node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        uuid = str(node.get("uuid") or "").strip()
        if not uuid:
            logger.warning("VLESS node '%s' missing uuid", node_name)
            return None
        flow = str(node.get("flow") or "").strip()
        if flow.lower() in ("xtls-rprx-vision", "xtls-rprx-vision-udp443"):
            flow = "xtls-rprx-vision"
        outbound = {
            "type": "vless",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "uuid": uuid,
        }
        if flow:
            outbound["flow"] = flow

        tls_conf = {"enabled": True}
        sni = str(node.get("servername") or node.get("sni") or node.get("server_name") or "").strip()
        if sni:
            tls_conf["server_name"] = sni
        if node.get("skip-cert-verify") or node.get("skip_cert_verify") or node.get("insecure"):
            tls_conf["insecure"] = True

        # Reality configuration
        reality_opts = node.get("reality-opts") or node.get("reality_opts") or {}
        if not isinstance(reality_opts, dict):
            reality_opts = {}
        pub_key = (
            reality_opts.get("public-key")
            or reality_opts.get("public_key")
            or reality_opts.get("publicKey")
            or node.get("public-key")
            or node.get("public_key")
        )
        short_id = (
            reality_opts.get("short-id")
            or reality_opts.get("short_id")
            or reality_opts.get("shortId")
            or node.get("short-id")
            or node.get("short_id")
            or ""
        )

        if pub_key:
            pub_key_str = str(pub_key).strip().rstrip("=")
            short_id_str = str(short_id).strip()
            tls_conf["reality"] = {
                "enabled": True,
                "public_key": pub_key_str,
                "short_id": short_id_str,
            }
            fp = str(
                node.get("client-fingerprint")
                or node.get("client_fingerprint")
                or node.get("fingerprint")
                or reality_opts.get("fingerprint")
                or "chrome"
            ).strip() or "chrome"
            tls_conf["utls"] = {"enabled": True, "fingerprint": fp}
        else:
            fp = str(
                node.get("client-fingerprint")
                or node.get("client_fingerprint")
                or node.get("fingerprint")
                or ""
            ).strip()
            if fp:
                tls_conf["utls"] = {"enabled": True, "fingerprint": fp}

        alpn = _normalize_alpn(node.get("alpn"))
        if alpn:
            tls_conf["alpn"] = alpn

        outbound["tls"] = tls_conf

        # Transport layer (ws, grpc)
        network = str(node.get("network") or "tcp").strip().lower()
        if network == "ws":
            ws_opts = node.get("ws-opts") or node.get("ws_opts") or {}
            transport = {"type": "ws"}
            if isinstance(ws_opts, dict):
                path = ws_opts.get("path")
                if path:
                    transport["path"] = str(path)
                headers = ws_opts.get("headers")
                if isinstance(headers, dict) and headers:
                    transport["headers"] = {str(k): str(v) for k, v in headers.items()}
            outbound["transport"] = transport
        elif network == "grpc":
            grpc_opts = node.get("grpc-opts") or node.get("grpc_opts") or {}
            transport = {"type": "grpc"}
            if isinstance(grpc_opts, dict) and grpc_opts.get("grpc-service-name"):
                transport["service_name"] = str(grpc_opts["grpc-service-name"])
            outbound["transport"] = transport
        return outbound

    # 4. Trojan
    if node_type == "trojan":
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("Trojan node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        password = str(node.get("password") or "").strip()
        if not password:
            logger.warning("Trojan node '%s' missing password", node_name)
            return None
        outbound = {
            "type": "trojan",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "password": password,
        }
        tls_conf = {"enabled": True}
        sni = str(node.get("sni") or node.get("servername") or node.get("server_name") or "").strip()
        if sni:
            tls_conf["server_name"] = sni
        if node.get("skip-cert-verify") or node.get("skip_cert_verify") or node.get("insecure"):
            tls_conf["insecure"] = True
        alpn = _normalize_alpn(node.get("alpn"))
        if alpn:
            tls_conf["alpn"] = alpn
        outbound["tls"] = tls_conf

        network = str(node.get("network") or "tcp").strip().lower()
        if network == "ws":
            ws_opts = node.get("ws-opts") or node.get("ws_opts") or {}
            transport = {"type": "ws"}
            if isinstance(ws_opts, dict):
                path = ws_opts.get("path")
                if path:
                    transport["path"] = str(path)
                headers = ws_opts.get("headers")
                if isinstance(headers, dict) and headers:
                    transport["headers"] = {str(k): str(v) for k, v in headers.items()}
            outbound["transport"] = transport
        return outbound

    # 5. Hysteria2
    if node_type in ("hysteria2", "hy2"):
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("Hysteria2 node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        password = str(
            node.get("password")
            or node.get("auth")
            or node.get("token")
            or node.get("auth-str")
            or node.get("auth_str")
            or ""
        ).strip()
        if not password:
            logger.warning("Hysteria2 node '%s' missing password/auth", node_name)
            return None
        outbound = {
            "type": "hysteria2",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "password": password,
        }
        tls_conf = {"enabled": True}
        sni = str(node.get("sni") or node.get("servername") or node.get("server_name") or "").strip()
        if sni:
            tls_conf["server_name"] = sni
        if node.get("skip-cert-verify") or node.get("skip_cert_verify") or node.get("insecure"):
            tls_conf["insecure"] = True
        alpn = _normalize_alpn(node.get("alpn"))
        if alpn:
            tls_conf["alpn"] = alpn
        outbound["tls"] = tls_conf

        up_mbps = _parse_mbps(node.get("up") or node.get("up-mbps") or node.get("up_mbps"))
        down_mbps = _parse_mbps(node.get("down") or node.get("down-mbps") or node.get("down_mbps"))
        if up_mbps is not None:
            outbound["up_mbps"] = up_mbps
        if down_mbps is not None:
            outbound["down_mbps"] = down_mbps

        obfs_raw = node.get("obfs")
        if isinstance(obfs_raw, dict):
            obfs_type = obfs_raw.get("type")
            obfs_pass = obfs_raw.get("password") or obfs_raw.get("pass")
        else:
            obfs_type = obfs_raw
            obfs_pass = (
                node.get("obfs-password")
                or node.get("obfs_password")
                or node.get("obfs-pass")
                or node.get("obfs_pass")
            )
        if obfs_type and obfs_pass:
            outbound["obfs"] = {
                "type": str(obfs_type),
                "password": str(obfs_pass),
            }
        return outbound

    # 6. TUIC
    if node_type == "tuic":
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("TUIC node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        uuid = str(node.get("uuid") or "").strip()
        password = str(node.get("password") or node.get("token") or "").strip()
        if not uuid and password:
            uuid = password
        if not password and uuid:
            password = uuid
        if not uuid or not password:
            logger.warning("TUIC node '%s' missing uuid/password", node_name)
            return None
        outbound = {
            "type": "tuic",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "uuid": uuid,
            "password": password,
        }
        cc = str(
            node.get("congestion-controller")
            or node.get("congestion_controller")
            or node.get("congestion-control")
            or node.get("congestion_control")
            or ""
        ).strip()
        if cc:
            outbound["congestion_control"] = cc
        udp_mode = str(node.get("udp-relay-mode") or node.get("udp_relay_mode") or "").strip()
        if udp_mode:
            outbound["udp_relay_mode"] = udp_mode

        tls_conf = {"enabled": True}
        sni = str(node.get("sni") or node.get("servername") or node.get("server_name") or "").strip()
        if sni:
            tls_conf["server_name"] = sni
        if node.get("skip-cert-verify") or node.get("skip_cert_verify") or node.get("insecure"):
            tls_conf["insecure"] = True
        alpn = _normalize_alpn(node.get("alpn"))
        if alpn:
            tls_conf["alpn"] = alpn
        outbound["tls"] = tls_conf
        return outbound

    # 7. WireGuard
    if node_type == "wireguard":
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("WireGuard node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        private_key = str(node.get("private-key") or node.get("private_key") or "").strip()
        public_key = str(node.get("public-key") or node.get("public_key") or "").strip()
        if not private_key or not public_key:
            logger.warning("WireGuard node '%s' missing private_key or public_key", node_name)
            return None

        ip_list = []
        ip = node.get("ip")
        if ip:
            ip_str = str(ip).strip()
            if "/" not in ip_str:
                ip_str = f"{ip_str}/32"
            ip_list.append(ip_str)
        ipv6 = node.get("ipv6")
        if ipv6:
            ipv6_str = str(ipv6).strip()
            if "/" not in ipv6_str:
                ipv6_str = f"{ipv6_str}/128"
            ip_list.append(ipv6_str)
        if not ip_list:
            ip_list = ["172.16.0.2/32"]

        peer: dict[str, Any] = {
            "address": server,
            "port": server_port,
            "public_key": public_key,
            "allowed_ips": ["0.0.0.0/0", "::/0"],
        }
        psk = node.get("preshared-key") or node.get("preshared_key") or node.get("pre-shared-key")
        if psk:
            peer["pre_shared_key"] = str(psk).strip()

        reserved = node.get("reserved")
        if reserved:
            if isinstance(reserved, list):
                peer["reserved"] = [int(x) for x in reserved]
            elif isinstance(reserved, str) and _is_valid_base64(reserved):
                try:
                    peer["reserved"] = list(base64.b64decode(reserved))
                except Exception:
                    pass

        endpoint: dict[str, Any] = {
            "type": "wireguard",
            "tag": tag,
            "system": False,
            "name": "wg0",
            "address": ip_list,
            "private_key": private_key,
            "peers": [peer],
        }
        mtu = node.get("mtu")
        if mtu:
            try:
                endpoint["mtu"] = int(mtu)
            except Exception:
                pass
        return endpoint

    # 8. HTTP and SOCKS5
    if node_type in ("http", "https"):
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("HTTP node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        outbound = {
            "type": "http",
            "tag": tag,
            "server": server,
            "server_port": server_port,
        }
        user = node.get("username")
        pwd = node.get("password")
        if user:
            outbound["username"] = str(user)
        if pwd:
            outbound["password"] = str(pwd)
        if node_type == "https" or node.get("tls"):
            outbound["tls"] = {"enabled": True}
        return outbound

    if node_type in ("socks", "socks5"):
        if not server or server_port <= 0 or server_port > 65535:
            logger.warning("SOCKS node '%s' has invalid server/port (%s:%s)", node_name, server, server_port)
            return None
        outbound = {
            "type": "socks",
            "tag": tag,
            "server": server,
            "server_port": server_port,
        }
        user = node.get("username")
        pwd = node.get("password")
        if user:
            outbound["username"] = str(user)
        if pwd:
            outbound["password"] = str(pwd)
        return outbound

    logger.warning("Node '%s' has unsupported protocol '%s'", node_name, node_type)
    return None


def generate_singbox_config(outbound: dict[str, Any], listen_port: int) -> dict[str, Any]:
    """生成单节点运行的完整 sing-box 配置 JSON"""
    cfg: dict[str, Any] = {
        "log": {
            "level": "error",
            "disabled": False,
        },
        "inbounds": [
            {
                "type": "mixed",
                "tag": "mixed-in",
                "listen": "127.0.0.1",
                "listen_port": listen_port,
            }
        ],
        "outbounds": [
            {
                "type": "direct",
                "tag": "direct",
            },
        ],
        "route": {
            "rules": [],
            "final": outbound.get("tag", "proxy-out"),
        },
    }
    if outbound.get("type") == "wireguard":
        cfg["endpoints"] = [outbound]
    else:
        cfg["outbounds"].insert(0, outbound)
    return cfg
