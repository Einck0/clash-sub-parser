from __future__ import annotations

import base64
import json
from typing import Any
import pytest
import yaml

from app.schemas.logical_config import (
    CompilationResult,
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
    render_compiled_shadowrocket_ruleset,
    render_compiled_singbox,
    render_compiled_stash,
    render_compiled_target,
    render_compiled_yaml,
)


@pytest.fixture
def sample_nodes() -> list[dict[str, Any]]:
    """Sample nodes covering mainstream protocols and reality options."""
    return [
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
            "private-key": "private-key-test-base64",
            "public-key": "public-key-test-base64",
            "ip": "10.0.0.2",
        },
    ]


@pytest.fixture
def sample_bundle() -> LogicalConfigurationBundle:
    """Logical configuration bundle with groups, rules, and DNS."""
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
            logical_id="r1",
            rule_type="DOMAIN-SUFFIX",
            payload="google.com",
            target_direct_or_reject="PROXY",
            sort_order=1,
            enabled=True,
        ),
        LogicalRuleConfig(
            logical_id="r2",
            rule_type="GEOSITE",
            payload="cn",
            target_direct_or_reject="DIRECT",
            sort_order=2,
            enabled=True,
        ),
        LogicalRuleConfig(
            logical_id="r3",
            rule_type="IP-CIDR",
            payload="192.168.0.0/16",
            target_direct_or_reject="DIRECT",
            sort_order=3,
            enabled=True,
        ),
        LogicalRuleConfig(
            logical_id="r4",
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

    return LogicalConfigurationBundle(
        logical_id="bundle_test",
        name="test_bundle",
        groups=[group_select, group_auto],
        rules=rules,
        dns=dns,
    )


def test_five_targets_golden_output(sample_bundle, sample_nodes):
    """Verify all five target adapters produce valid golden outputs."""
    resolver = CanonicalGraphResolver(bundle=sample_bundle, inventory_nodes=sample_nodes)
    comp_res: CompilationResult = resolver.compile()
    assert comp_res.success is True

    # 1. Clash Target
    clash_yaml = render_compiled_clash(comp_res, sample_bundle, sample_nodes)
    parsed_clash = yaml.safe_load(clash_yaml)
    assert isinstance(parsed_clash, dict)
    assert "proxies" in parsed_clash
    assert len(parsed_clash["proxies"]) == len(sample_nodes)
    assert parsed_clash["port"] == 7890
    assert parsed_clash["mode"] == "rule"
    assert "proxy-groups" in parsed_clash
    assert len(parsed_clash["proxy-groups"]) == 2
    assert "rules" in parsed_clash
    assert "DOMAIN-SUFFIX,google.com,PROXY" in parsed_clash["rules"]
    assert "MATCH,PROXY" in parsed_clash["rules"]
    assert "'815458e4'" in clash_yaml  # Reality short-id quoted

    # 2. Mihomo Target
    mihomo_yaml = render_compiled_mihomo(comp_res, sample_bundle, sample_nodes)
    parsed_mihomo = yaml.safe_load(mihomo_yaml)
    assert isinstance(parsed_mihomo, dict)
    assert parsed_mihomo.get("mixed-port") == 7890
    assert parsed_mihomo.get("ipv6") is True
    assert any(p.get("type") == "hysteria2" for p in parsed_mihomo["proxies"])
    assert any(p.get("type") == "tuic" for p in parsed_mihomo["proxies"])
    assert any(p.get("type") == "wireguard" for p in parsed_mihomo["proxies"])
    assert "GEOSITE,cn,DIRECT" in parsed_mihomo["rules"]

    # 3. Stash Target
    stash_yaml = render_compiled_stash(comp_res, sample_bundle, sample_nodes)
    parsed_stash = yaml.safe_load(stash_yaml)
    assert isinstance(parsed_stash, dict)
    assert "proxies" in parsed_stash
    assert "proxy-groups" in parsed_stash
    assert "rules" in parsed_stash

    # 4. Shadowrocket Target
    sr_b64 = render_compiled_shadowrocket(comp_res, sample_bundle, sample_nodes)
    assert isinstance(sr_b64, str)
    decoded_links = base64.b64decode(sr_b64).decode("utf-8")
    assert "ss://" in decoded_links
    assert "vmess://" in decoded_links
    assert "vless://" in decoded_links
    assert "trojan://" in decoded_links
    assert "hysteria2://" in decoded_links
    assert "tuic://" in decoded_links
    assert "wireguard://" in decoded_links

    sr_ruleset = render_compiled_shadowrocket_ruleset(comp_res, sample_bundle)
    assert "DOMAIN-SUFFIX,google.com,PROXY" in sr_ruleset
    assert "FINAL,PROXY" in sr_ruleset

    # 5. Sing-box Target
    sb_json = render_compiled_singbox(comp_res, sample_bundle, sample_nodes)
    parsed_sb = json.loads(sb_json)
    assert isinstance(parsed_sb, dict)
    assert "inbounds" in parsed_sb
    assert "outbounds" in parsed_sb
    assert "route" in parsed_sb
    assert "dns" in parsed_sb

    outbounds = parsed_sb["outbounds"]
    outbound_tags = [ob.get("tag") for ob in outbounds]
    assert "DIRECT" in outbound_tags
    assert "REJECT" in outbound_tags
    assert "HK-Shadowsocks" in outbound_tags
    assert "US-VMess" in outbound_tags
    assert "JP-VLESS-Reality" in outbound_tags
    assert "SG-Trojan" in outbound_tags
    assert "TW-Hysteria2" in outbound_tags
    assert "KR-TUIC" in outbound_tags
    assert "EU-WireGuard" in outbound_tags
    assert "PROXY" in outbound_tags
    assert "AUTO" in outbound_tags

    # Verify selector and urltest in Sing-box
    proxy_grp = next(ob for ob in outbounds if ob.get("tag") == "PROXY")
    assert proxy_grp.get("type") == "selector"
    auto_grp = next(ob for ob in outbounds if ob.get("tag") == "AUTO")
    assert auto_grp.get("type") == "urltest"
    assert auto_grp.get("tolerance") == 50

    # Verify route rules in Sing-box
    route_rules = parsed_sb["route"].get("rules", [])
    assert any("google.com" in r.get("domain_suffix", []) for r in route_rules)
    assert any("192.168.0.0/16" in r.get("ip_cidr", []) for r in route_rules)
    assert parsed_sb["route"].get("final") == "PROXY"


def test_secret_minimization(sample_bundle, sample_nodes):
    """Verify internal management tokens and source IDs are excluded from exports."""
    resolver = CanonicalGraphResolver(bundle=sample_bundle, inventory_nodes=sample_nodes)
    comp_res = resolver.compile()

    for target in ("clash", "mihomo", "stash", "sing-box"):
        rendered = render_compiled_target(target, comp_res, sample_bundle, sample_nodes)
        assert "internal-secret-source-123" not in rendered
        assert "leak-prevention-token-xyz" not in rendered
        assert "should-not-be-in-export" not in rendered

    sr_b64 = render_compiled_target("shadowrocket", comp_res, sample_bundle, sample_nodes)
    sr_text = base64.b64decode(sr_b64).decode("utf-8")
    assert "internal-secret-source-123" not in sr_text
    assert "leak-prevention-token-xyz" not in sr_text
    assert "should-not-be-in-export" not in sr_text


def test_script_format_strictly_rejected(sample_bundle, sample_nodes):
    """Verify script format is strictly rejected and returns error."""
    resolver = CanonicalGraphResolver(bundle=sample_bundle, inventory_nodes=sample_nodes)
    comp_res = resolver.compile()

    with pytest.raises(ValueError, match="deprecated"):
        render_compiled_target("script", comp_res, sample_bundle, sample_nodes)


def test_render_compiled_yaml_backward_compatibility(sample_bundle, sample_nodes):
    """Verify render_compiled_yaml works as backward-compatible alias."""
    resolver = CanonicalGraphResolver(bundle=sample_bundle, inventory_nodes=sample_nodes)
    comp_res = resolver.compile()

    out = render_compiled_yaml(comp_res, sample_bundle, sample_nodes)
    parsed = yaml.safe_load(out)
    assert "proxies" in parsed
    assert "rules" in parsed


@pytest.mark.asyncio
async def test_five_target_endpoints_and_headers(client):
    """Verify API routes for all five targets and HTTP header pass-through."""
    from tests.conftest import TestSession
    from app.models.subscription import Subscription

    # Create a primary subscription
    resp = await client.post(
        "/api/subscriptions",
        json={
            "name": "primary-sub",
            "url": "https://example.com/sub",
            "is_primary": True,
            "manual_nodes": [
                {
                    "name": "HK-Node",
                    "type": "ss",
                    "server": "hk.example.com",
                    "port": 8388,
                    "cipher": "aes-256-gcm",
                    "password": "p1",
                },
                {
                    "name": "US-Node",
                    "type": "vmess",
                    "server": "us.example.com",
                    "port": 443,
                    "uuid": "00000000-0000-4000-8000-000000000001",
                    "alterId": 0,
                    "cipher": "auto",
                },
            ],
        },
    )
    assert resp.status_code == 201
    sub_id = resp.json()["id"]

    # Set subscription userinfo headers
    async with TestSession() as session:
        sub = await session.get(Subscription, sub_id)
        assert sub is not None
        sub.subscription_userinfo = "upload=1000; download=2000; total=10000; expire=1800000000"
        sub.profile_update_interval = "24"
        sub.profile_web_page_url = "https://example.com/user"
        await session.commit()

    # 1. Clash
    clash_resp = await client.get("/api/generate/clash")
    assert clash_resp.status_code == 200
    assert "application/x-yaml" in clash_resp.headers["content-type"]
    assert 'filename="config.yaml"' in clash_resp.headers["content-disposition"]
    assert clash_resp.headers["subscription-userinfo"] == "upload=1000; download=2000; total=10000; expire=1800000000"
    assert clash_resp.headers["profile-update-interval"] == "24"
    assert clash_resp.headers["profile-web-page-url"] == "https://example.com/user"
    assert "HK-Node" in clash_resp.text

    # 2. Mihomo
    mihomo_resp = await client.get("/api/generate/mihomo")
    assert mihomo_resp.status_code == 200
    assert "application/x-yaml" in mihomo_resp.headers["content-type"]
    assert clash_resp.headers["subscription-userinfo"] == "upload=1000; download=2000; total=10000; expire=1800000000"

    # 3. Stash
    stash_resp = await client.get("/api/generate/stash")
    assert stash_resp.status_code == 200
    assert 'filename="stash.yaml"' in stash_resp.headers["content-disposition"]
    assert stash_resp.headers["subscription-userinfo"] == "upload=1000; download=2000; total=10000; expire=1800000000"

    # 4. Shadowrocket
    sr_resp = await client.get("/api/generate/shadowrocket")
    assert sr_resp.status_code == 200
    assert 'filename="sub.txt"' in sr_resp.headers["content-disposition"]
    assert sr_resp.headers["subscription-userinfo"] == "upload=1000; download=2000; total=10000; expire=1800000000"
    decoded_sr = base64.b64decode(sr_resp.text).decode("utf-8")
    assert "ss://" in decoded_sr

    # 5. Sing-box
    sb_resp = await client.get("/api/generate/sing-box")
    assert sb_resp.status_code == 200
    assert 'filename="config.json"' in sb_resp.headers["content-disposition"]
    assert sb_resp.headers["subscription-userinfo"] == "upload=1000; download=2000; total=10000; expire=1800000000"
    sb_data = json.loads(sb_resp.text)
    assert "inbounds" in sb_data
    assert "outbounds" in sb_data
    assert any(ob.get("tag") == "HK-Node" for ob in sb_data["outbounds"])

    # Attachment /current endpoint
    clash_attach = await client.get("/api/generate/clash/current")
    assert clash_attach.status_code == 200
    assert "attachment" in clash_attach.headers["content-disposition"]


@pytest.mark.asyncio
async def test_script_endpoints_return_404(client):
    """Verify all script endpoints strictly return 404."""
    for path in (
        "/script",
        "/api/generate/script",
        "/api/generate/script/current",
        "/api/generate/script/download",
        "/generate/script",
    ):
        res = await client.get(path)
        assert res.status_code == 404, f"Expected 404 for {path}, got {res.status_code}"


@pytest.mark.asyncio
async def test_quick_export_api(client):
    """Verify QuickExport returns URLs, schemes, and QR payloads for five targets."""
    # 1. Merged QuickExport
    res = await client.get("/api/generate/quick-export")
    assert res.status_code == 200
    data = res.json()
    assert data["scope"] == "merged"
    assert "targets" in data
    targets = data["targets"]

    for expected_target in ("clash", "mihomo", "stash", "shadowrocket", "sing-box"):
        assert expected_target in targets
        t_info = targets[expected_target]
        assert "url" in t_info
        assert "scheme_url" in t_info
        assert "qrcode_payload" in t_info

    assert "script" not in targets
    assert targets["clash"]["scheme_url"].startswith("clash://install-config")
    assert targets["stash"]["scheme_url"].startswith("stash://install-config")
    assert targets["shadowrocket"]["scheme_url"].startswith("sub://")
    assert targets["sing-box"]["scheme_url"].startswith("sing-box://import-remote-profile")

    # Verify via downloads router alias
    dl_res = await client.get("/api/downloads/quick-export")
    assert dl_res.status_code == 200
    assert dl_res.json()["scope"] == "merged"


@pytest.mark.asyncio
async def test_single_subscription_export(client):
    """Verify single subscription export for various targets."""
    resp = await client.post(
        "/api/subscriptions",
        json={
            "name": "single-sub-test",
            "url": "https://example.com/single",
            "manual_nodes": [
                {
                    "name": "Single-HK",
                    "type": "ss",
                    "server": "single.example.com",
                    "port": 8388,
                    "cipher": "aes-256-gcm",
                    "password": "pass",
                }
            ],
        },
    )
    assert resp.status_code == 201
    sub_id = resp.json()["id"]

    # GET /generate/subscription/{id}?target=sing-box
    sb_get = await client.get(f"/api/generate/subscription/{sub_id}?target=sing-box")
    assert sb_get.status_code == 200
    assert "Single-HK" in sb_get.text

    # GET /generate/subscription/{id}?target=shadowrocket
    sr_get = await client.get(f"/api/generate/subscription/{sub_id}?target=shadowrocket")
    assert sr_get.status_code == 200
    decoded = base64.b64decode(sr_get.text).decode("utf-8")
    assert "ss://" in decoded

    # POST /generate/subscription/{id}?target=clash
    clash_post = await client.post(f"/api/generate/subscription/{sub_id}?target=clash")
    assert clash_post.status_code == 200
    assert "Single-HK" in clash_post.json()["yaml"]

    # QuickExport for single subscription
    qe_resp = await client.get(f"/api/generate/quick-export?subscription_id={sub_id}")
    assert qe_resp.status_code == 200
    qe_data = qe_resp.json()
    assert qe_data["scope"] == "subscription"
    assert qe_data["subscription_id"] == sub_id
    assert qe_data["subscription_name"] == "single-sub-test"
    assert f"subscription/{sub_id}" in qe_data["targets"]["clash"]["url"]
