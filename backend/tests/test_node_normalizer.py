from __future__ import annotations

from app.services.node_normalizer import (
    compute_payload_fingerprint,
    extract_node_identity,
    normalize_node_payload,
)


def test_renamed_nodes_share_identical_fingerprint():
    node_a = {
        "name": "香港 01 [VIP]",
        "type": "ss",
        "server": "hk.example.com",
        "port": 443,
        "cipher": "aes-256-gcm",
        "password": "p@ssword",
    }
    node_b = {
        "name": "HK-Node-Fast-01",
        "type": "ss",
        "server": "hk.example.com",
        "port": 443,
        "cipher": "aes-256-gcm",
        "password": "p@ssword",
    }

    norm_a = normalize_node_payload(node_a)
    norm_b = normalize_node_payload(node_b)
    assert norm_a == norm_b

    fp_a = compute_payload_fingerprint(norm_a)
    fp_b = compute_payload_fingerprint(norm_b)
    assert fp_a == fp_b
    assert fp_a.startswith("v1:")


def test_alias_fields_and_casing_normalization():
    # method 与 cipher 别名互通，大小写自动规范化
    node_1 = {
        "name": "SS1",
        "type": "ss",
        "server": "1.1.1.1",
        "port": "8388",
        "method": "AES-128-GCM",
        "password": "sec",
    }
    node_2 = {
        "name": "SS2",
        "type": "ss",
        "server": "1.1.1.1",
        "port": 8388,
        "cipher": "aes-128-gcm",
        "password": "sec",
    }

    assert normalize_node_payload(node_1) == normalize_node_payload(node_2)
    assert compute_payload_fingerprint(normalize_node_payload(node_1)) == compute_payload_fingerprint(
        normalize_node_payload(node_2)
    )


def test_semantic_protocol_differences_produce_different_fingerprints():
    base_vless = {
        "name": "VLESS-Node",
        "type": "vless",
        "server": "us.example.com",
        "port": 443,
        "uuid": "11111111-2222-3333-4444-555555555555",
        "tls": True,
        "flow": "xtls-rprx-vision",
    }

    # 修改端口
    diff_port = dict(base_vless, port=8443)
    # 修改 uuid
    diff_uuid = dict(base_vless, uuid="22222222-2222-3333-4444-555555555555")
    # 修改 flow
    diff_flow = dict(base_vless, flow="none")

    fp_base = compute_payload_fingerprint(normalize_node_payload(base_vless))
    fp_port = compute_payload_fingerprint(normalize_node_payload(diff_port))
    fp_uuid = compute_payload_fingerprint(normalize_node_payload(diff_uuid))
    fp_flow = compute_payload_fingerprint(normalize_node_payload(diff_flow))

    assert len({fp_base, fp_port, fp_uuid, fp_flow}) == 4


def test_extract_node_identity_helper():
    raw = {
        "name": "Trojan-Tokyo",
        "type": "trojan",
        "server": "jp.trojan.net",
        "port": 443,
        "password": "trojan_pass",
        "sni": "jp.trojan.net",
    }
    name, protocol, server, port, normalized, fingerprint = extract_node_identity(raw)
    assert name == "Trojan-Tokyo"
    assert protocol == "trojan"
    assert server == "jp.trojan.net"
    assert port == 443
    assert "name" not in normalized
    assert fingerprint.startswith("v1:")
