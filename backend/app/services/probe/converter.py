"""Clash 节点到 sing-box outbound 配置转换器

支持将主流代理节点（Shadowsocks、VMess、VLESS Reality、Trojan、Hysteria2、TUIC、HTTP 与 SOCKS5）
精确转换为 sing-box 1.14+ 合法的 outbound JSON 结构
"""
from __future__ import annotations

import base64
import json
from typing import Any


def _is_valid_base64(val: str) -> bool:
    if not val or not isinstance(val, str):
        return False
    # 处理 URL-safe 变体
    normalized = val.replace("-", "+").replace("_", "/")
    # 补齐 padding
    pad = len(normalized) % 4
    if pad:
        normalized += "=" * (4 - pad)
    try:
        decoded = base64.b64decode(normalized)
        return len(decoded) > 0
    except Exception:
        return False


def clash_to_singbox_outbound(node: dict[str, Any], tag: str = "proxy-out") -> dict[str, Any] | None:
    """将单个 Clash 节点字典转换为 sing-box outbound 或 endpoint 格式

    若缺少必需参数或协议不支持，返回 None
    """
    if not isinstance(node, dict):
        return None

    node_type = str(node.get("type") or "").strip().lower()
    server = str(node.get("server") or "").strip()
    try:
        server_port = int(node.get("port") or 0)
    except Exception:
        server_port = 0

    # 1. Shadowsocks
    if node_type in ("ss", "shadowsocks"):
        if not server or server_port <= 0 or server_port > 65535:
            return None
        cipher = str(node.get("cipher") or "").strip()
        password = str(node.get("password") or "")
        if not cipher or not password:
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
            return None
        uuid = str(node.get("uuid") or "").strip()
        if not uuid:
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
        # TLS 配置
        tls_enabled = bool(node.get("tls"))
        sni = str(node.get("servername") or node.get("sni") or "").strip()
        if tls_enabled or sni:
            tls_conf: dict[str, Any] = {"enabled": True}
            if sni:
                tls_conf["server_name"] = sni
            if node.get("skip-cert-verify"):
                tls_conf["insecure"] = True
            alpn = node.get("alpn")
            if isinstance(alpn, list):
                tls_conf["alpn"] = alpn
            outbound["tls"] = tls_conf

        # 传输协议 (ws, grpc, http)
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
            return None
        uuid = str(node.get("uuid") or "").strip()
        if not uuid:
            return None
        flow = str(node.get("flow") or "").strip()
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
        sni = str(node.get("servername") or node.get("sni") or "").strip()
        if sni:
            tls_conf["server_name"] = sni
        if node.get("skip-cert-verify"):
            tls_conf["insecure"] = True

        # Reality 配置
        reality_opts = node.get("reality-opts") or node.get("reality_opts") or {}
        if isinstance(reality_opts, dict) and reality_opts.get("public-key"):
            pub_key = str(reality_opts["public-key"]).strip().rstrip("=")
            short_id = str(reality_opts.get("short-id") or "").strip()
            tls_conf["reality"] = {
                "enabled": True,
                "public_key": pub_key,
                "short_id": short_id,
            }
            fp = str(node.get("client-fingerprint") or "chrome").strip() or "chrome"
            tls_conf["utls"] = {"enabled": True, "fingerprint": fp}
        else:
            fp = str(node.get("client-fingerprint") or "").strip()
            if fp:
                tls_conf["utls"] = {"enabled": True, "fingerprint": fp}

        alpn = node.get("alpn")
        if isinstance(alpn, list):
            tls_conf["alpn"] = alpn

        outbound["tls"] = tls_conf

        # 传输层 (ws, grpc)
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
            return None
        password = str(node.get("password") or "").strip()
        if not password:
            return None
        outbound = {
            "type": "trojan",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "password": password,
        }
        tls_conf = {"enabled": True}
        sni = str(node.get("sni") or node.get("servername") or "").strip()
        if sni:
            tls_conf["server_name"] = sni
        if node.get("skip-cert-verify"):
            tls_conf["insecure"] = True
        alpn = node.get("alpn")
        if isinstance(alpn, list):
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
            return None
        password = str(node.get("password") or node.get("auth") or "").strip()
        if not password:
            return None
        outbound = {
            "type": "hysteria2",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "password": password,
        }
        tls_conf = {"enabled": True}
        sni = str(node.get("sni") or node.get("servername") or "").strip()
        if sni:
            tls_conf["server_name"] = sni
        if node.get("skip-cert-verify"):
            tls_conf["insecure"] = True
        alpn = node.get("alpn")
        if isinstance(alpn, list):
            tls_conf["alpn"] = alpn
        outbound["tls"] = tls_conf

        obfs_type = node.get("obfs")
        obfs_pass = node.get("obfs-password") or node.get("obfs_password")
        if obfs_type and obfs_pass:
            outbound["obfs"] = {
                "type": str(obfs_type),
                "password": str(obfs_pass),
            }
        return outbound

    # 6. TUIC
    if node_type == "tuic":
        if not server or server_port <= 0 or server_port > 65535:
            return None
        uuid = str(node.get("uuid") or "").strip()
        password = str(node.get("password") or "").strip()
        if not uuid or not password:
            return None
        outbound = {
            "type": "tuic",
            "tag": tag,
            "server": server,
            "server_port": server_port,
            "uuid": uuid,
            "password": password,
        }
        cc = str(node.get("congestion-controller") or node.get("congestion_controller") or "").strip()
        if cc:
            outbound["congestion_control"] = cc
        tls_conf = {"enabled": True}
        sni = str(node.get("sni") or node.get("servername") or "").strip()
        if sni:
            tls_conf["server_name"] = sni
        if node.get("skip-cert-verify"):
            tls_conf["insecure"] = True
        outbound["tls"] = tls_conf
        return outbound

    # 7. WireGuard
    if node_type == "wireguard":
        if not server or server_port <= 0 or server_port > 65535:
            return None
        private_key = str(node.get("private-key") or node.get("private_key") or "").strip()
        public_key = str(node.get("public-key") or node.get("public_key") or "").strip()
        if not private_key or not public_key:
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

    # HTTP 和 SOCKS5 协议
    if node_type in ("http", "https"):
        if not server or server_port <= 0 or server_port > 65535:
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
