import base64
from app.services.probe.converter import clash_to_singbox_outbound, generate_singbox_config


def test_convert_shadowsocks():
    node = {
        "name": "SS-Node",
        "type": "ss",
        "server": "198.51.100.1",
        "port": 8388,
        "cipher": "aes-128-gcm",
        "password": "secret-password",
    }
    ob = clash_to_singbox_outbound(node, tag="custom-tag")
    assert ob is not None
    assert ob["type"] == "shadowsocks"
    assert ob["tag"] == "custom-tag"
    assert ob["server"] == "198.51.100.1"
    assert ob["server_port"] == 8388
    assert ob["method"] == "aes-128-gcm"
    assert ob["password"] == "secret-password"


def test_convert_vmess_ws_tls():
    node = {
        "name": "VMess-WS",
        "type": "vmess",
        "server": "198.51.100.2",
        "port": 443,
        "uuid": "00000000-0000-0000-0000-000000000000",
        "alterId": 0,
        "cipher": "auto",
        "tls": True,
        "servername": "example.com",
        "network": "ws",
        "ws-opts": {"path": "/chat", "headers": {"Host": "example.com"}},
    }
    ob = clash_to_singbox_outbound(node)
    assert ob is not None
    assert ob["type"] == "vmess"
    assert ob["server"] == "198.51.100.2"
    assert ob["tls"]["enabled"] is True
    assert ob["tls"]["server_name"] == "example.com"
    assert ob["transport"]["type"] == "ws"
    assert ob["transport"]["path"] == "/chat"
    assert ob["transport"]["headers"]["Host"] == "example.com"


def test_convert_vless_reality():
    raw_32 = b"01234567890123456789012345678901"
    b64_key = base64.b64encode(raw_32).decode("ascii")
    node = {
        "name": "VLESS-Reality",
        "type": "vless",
        "server": "198.51.100.3",
        "port": 443,
        "uuid": "00000000-0000-0000-0000-000000000000",
        "flow": "xtls-rprx-vision",
        "tls": True,
        "servername": "yahoo.com",
        "reality-opts": {"public-key": b64_key, "short-id": "abcd12"},
        "client-fingerprint": "chrome",
    }
    ob = clash_to_singbox_outbound(node)
    assert ob is not None
    assert ob["type"] == "vless"
    assert ob["flow"] == "xtls-rprx-vision"
    assert ob["tls"]["reality"]["enabled"] is True
    assert ob["tls"]["reality"]["public_key"] == b64_key.rstrip("=")
    assert ob["tls"]["utls"]["fingerprint"] == "chrome"


def test_convert_invalid_node():
    assert clash_to_singbox_outbound({}) is None
    assert clash_to_singbox_outbound({"type": "unknown", "server": "1.1.1.1", "port": 443}) is None
    assert clash_to_singbox_outbound({"type": "ss", "server": "1.1.1.1", "port": 0}) is None


def test_generate_singbox_config():
    ob = {"type": "direct", "tag": "test-proxy"}
    cfg = generate_singbox_config(ob, listen_port=21050)
    assert cfg["inbounds"][0]["listen_port"] == 21050
    assert cfg["inbounds"][0]["type"] == "mixed"
    assert cfg["outbounds"][0]["tag"] == "test-proxy"
    assert cfg["route"]["final"] == "test-proxy"


def test_convert_wireguard():
    node = {
        "name": "WARP-Test",
        "type": "wireguard",
        "server": "engage.cloudflareclient.com",
        "port": 2408,
        "ip": "172.16.0.2",
        "ipv6": "2606:4700:110:83d9:a49a:cf41:b523:fdf",
        "private-key": "KEVev0U71/ZYwXB/9LrubTLDcNprrrqwHzCfAwWSPWg=",
        "public-key": "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
        "mtu": 1280,
    }
    ep = clash_to_singbox_outbound(node)
    assert ep is not None
    assert ep["type"] == "wireguard"
    assert ep["name"] == "wg0"
    assert "172.16.0.2/32" in ep["address"]
    assert ep["peers"][0]["address"] == "engage.cloudflareclient.com"
    assert ep["peers"][0]["port"] == 2408

    cfg = generate_singbox_config(ep, listen_port=21051)
    assert "endpoints" in cfg
    assert cfg["endpoints"][0]["type"] == "wireguard"
    assert cfg["route"]["final"] == "proxy-out"
