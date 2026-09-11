import base64
import json
import os
import sys

# Ensure backend is in path
sys.path.insert(0, os.path.abspath("backend"))

from app.schemas.logical_config import (
    LogicalConfigurationBundle,
    LogicalDnsConfig,
    LogicalGroupConfig,
    LogicalRuleConfig,
    MembershipEntry,
)
from app.services.canonical_compiler import CanonicalGraphResolver
from app.services.compiler_target_adapters import (
    render_compiled_clash,
    render_compiled_mihomo,
    render_compiled_shadowrocket,
    render_compiled_singbox,
    render_compiled_stash,
)

def get_sample_data():
    sample_nodes = [
        {
            "name": "HK-Shadowsocks",
            "type": "ss",
            "server": "hk.example.com",
            "port": 8388,
            "cipher": "aes-256-gcm",
            "password": "secret-password-1",
            "source_id": "internal-secret-source-123",
            "admin_token": "leak-prevention-token-xyz",
        },
        {
            "name": "US-VMess",
            "type": "vmess",
            "server": "us.example.com",
            "port": 443,
            "uuid": "00000000-0000-4000-8000-000000000001",
            "alterId": 0,
            "cipher": "auto",
            "tls": True,
            "network": "ws",
            "ws-opts": {"path": "/ws-path"},
            "management_token": "should-not-be-in-export",
        },
        {
            "name": "JP-VLESS-Reality",
            "type": "vless",
            "server": "jp.example.com",
            "port": 443,
            "uuid": "00000000-0000-4000-8000-000000000002",
            "tls": True,
            "reality-opts": {
                "public-key": "test-pubkey-reality",
                "short-id": "815458e4",
            },
            "client-fingerprint": "chrome",
        },
        {
            "name": "SG-Trojan",
            "type": "trojan",
            "server": "sg.example.com",
            "port": 443,
            "password": "trojan-pass-123",
            "sni": "sg.example.com",
        },
        {
            "name": "TW-Hysteria2",
            "type": "hysteria2",
            "server": "tw.example.com",
            "port": 8443,
            "password": "hy2-password-abc",
            "sni": "tw.example.com",
            "obfs": "salamander",
            "obfs-password": "obfs-pass-xyz",
        },
        {
            "name": "KR-TUIC",
            "type": "tuic",
            "server": "kr.example.com",
            "port": 8443,
            "uuid": "00000000-0000-4000-8000-000000000003",
            "password": "tuic-pass-456",
            "congestion-controller": "bbr",
        },
        {
            "name": "EU-WireGuard",
            "type": "wireguard",
            "server": "eu.example.com",
            "port": 51820,
            "ip": "10.0.0.2",
            "public-key": "public-key-test-base64",
            "private-key": "private-key-test-base64",
        },
    ]

    group_select = LogicalGroupConfig(
        logical_id="g_proxy",
        name="PROXY",
        group_type="select",
        sort_order=1,
        include_entries=[
            MembershipEntry(entry_type="node_name", value="HK-Shadowsocks", order_idx=0),
            MembershipEntry(entry_type="node_name", value="US-VMess", order_idx=1),
            MembershipEntry(entry_type="node_name", value="JP-VLESS-Reality", order_idx=2),
            MembershipEntry(entry_type="node_name", value="SG-Trojan", order_idx=3),
            MembershipEntry(entry_type="node_name", value="TW-Hysteria2", order_idx=4),
            MembershipEntry(entry_type="node_name", value="KR-TUIC", order_idx=5),
            MembershipEntry(entry_type="node_name", value="EU-WireGuard", order_idx=6),
        ],
    )

    group_auto = LogicalGroupConfig(
        logical_id="g_auto",
        name="AUTO",
        group_type="url-test",
        sort_order=2,
        include_entries=[
            MembershipEntry(entry_type="node_name", value="HK-Shadowsocks", order_idx=0),
            MembershipEntry(entry_type="node_name", value="US-VMess", order_idx=1),
        ],
        url_test_config={
            "url": "https://cp.cloudflare.com/generate_204",
            "interval": 300,
            "tolerance": 50,
        },
    )

    rules = [
        LogicalRuleConfig(
            logical_id="rule_1",
            rule_type="DOMAIN-SUFFIX",
            payload="google.com",
            target_group="PROXY",
            sort_order=1,
            enabled=True,
        ),
        LogicalRuleConfig(
            logical_id="rule_2",
            rule_type="GEOSITE",
            payload="cn",
            target_direct_or_reject="DIRECT",
            sort_order=2,
            enabled=True,
        ),
        LogicalRuleConfig(
            logical_id="rule_3",
            rule_type="IP-CIDR",
            payload="192.168.0.0/16",
            target_direct_or_reject="DIRECT",
            options=["no-resolve"],
            sort_order=3,
            enabled=True,
        ),
        LogicalRuleConfig(
            logical_id="rule_4",
            rule_type="MATCH",
            payload="",
            target_direct_or_reject="PROXY",
            sort_order=4,
            enabled=True,
        ),
    ]

    dns = LogicalDnsConfig(
        enabled=True,
        raw_yaml="dns:\n  enable: true\n  nameserver:\n    - 8.8.8.8\n    - 1.1.1.1\n",
    )

    bundle = LogicalConfigurationBundle(
        logical_id="bundle_test",
        name="test_bundle",
        groups=[group_select, group_auto],
        rules=rules,
        dns=dns,
    )

    return bundle, sample_nodes

def main():
    bundle, sample_nodes = get_sample_data()
    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=sample_nodes)
    comp_res = resolver.compile()
    assert comp_res.success

    out_dir = os.path.abspath("tmp/python_golden")
    os.makedirs(out_dir, exist_ok=True)

    clash_out = render_compiled_clash(comp_res, bundle, sample_nodes)
    with open(os.path.join(out_dir, "clash.yaml"), "w", encoding="utf-8") as f:
        f.write(clash_out)

    mihomo_out = render_compiled_mihomo(comp_res, bundle, sample_nodes)
    with open(os.path.join(out_dir, "mihomo.yaml"), "w", encoding="utf-8") as f:
        f.write(mihomo_out)

    singbox_out = render_compiled_singbox(comp_res, bundle, sample_nodes)
    with open(os.path.join(out_dir, "singbox.json"), "w", encoding="utf-8") as f:
        f.write(singbox_out)

    sr_b64 = render_compiled_shadowrocket(comp_res, bundle, sample_nodes)
    with open(os.path.join(out_dir, "shadowrocket.txt"), "w", encoding="utf-8") as f:
        f.write(sr_b64)

    stash_out = render_compiled_stash(comp_res, bundle, sample_nodes)
    with open(os.path.join(out_dir, "stash.yaml"), "w", encoding="utf-8") as f:
        f.write(stash_out)

    print(f"Generated python golden files in {out_dir}")
    print(f"Clash: {len(clash_out)} chars")
    print(f"Mihomo: {len(mihomo_out)} chars")
    print(f"Singbox: {len(singbox_out)} chars")
    print(f"Shadowrocket: {len(sr_b64)} chars")
    print(f"Stash: {len(stash_out)} chars")

if __name__ == "__main__":
    main()
