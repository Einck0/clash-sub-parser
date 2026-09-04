"""Unit and integration tests for probe credential hydration and converter robustness.

Tests verify:
1. Detection of missing or redacted credentials across proxy protocols.
2. Robust conversion in sing-box outbound converter (Reality, TUIC, Hysteria2).
3. Automatic credential hydration from database (Subscription and NodeRepository).
4. End-to-end hydration integration in probe_single_node and probe_batch_nodes.
"""

from __future__ import annotations

from contextlib import asynccontextmanager
from typing import Any
import pytest
import pytest_asyncio
from sqlalchemy.ext.asyncio import AsyncSession

from tests.conftest import TestSession
from app.models.subscription import Subscription
from app.repositories.node_repository import NodeRepository
from app.services.probe.converter import clash_to_singbox_outbound
from app.services.probe.service import (
    is_node_credential_missing,
    hydrate_nodes_batch,
    probe_single_node,
    probe_batch_nodes,
)


@pytest_asyncio.fixture
async def async_session():
    """Yield an isolated test database session."""
    async with TestSession() as session:
        yield session


def test_is_node_credential_missing():
    """Verify that is_node_credential_missing accurately identifies missing or redacted credentials."""
    # 1. Complete nodes should NOT be considered missing credentials
    complete_vless = {
        "name": "vless-node",
        "type": "vless",
        "server": "1.1.1.1",
        "port": 443,
        "uuid": "11111111-2222-3333-4444-555555555555",
        "reality-opts": {"public-key": "pubkey123"},
    }
    assert not is_node_credential_missing(complete_vless)

    complete_ss = {
        "name": "ss-node",
        "type": "ss",
        "server": "1.1.1.1",
        "port": 8388,
        "cipher": "aes-128-gcm",
        "password": "secret-pass",
    }
    assert not is_node_credential_missing(complete_ss)

    complete_hy2 = {
        "name": "hy2-node",
        "type": "hysteria2",
        "server": "1.1.1.1",
        "port": 443,
        "password": "auth-password",
    }
    assert not is_node_credential_missing(complete_hy2)

    # 2. Stripped nodes (e.g. from list_final_nodes)
    stripped_vless = {
        "name": "vless-node",
        "type": "vless",
        "server": "1.1.1.1",
        "port": 443,
    }
    assert is_node_credential_missing(stripped_vless)

    stripped_ss = {
        "name": "ss-node",
        "type": "ss",
        "server": "1.1.1.1",
        "port": 8388,
        "cipher": "aes-128-gcm",
    }
    assert is_node_credential_missing(stripped_ss)

    stripped_hy2 = {
        "name": "hy2-node",
        "type": "hysteria2",
        "server": "1.1.1.1",
        "port": 443,
    }
    assert is_node_credential_missing(stripped_hy2)

    # 3. Redacted credentials
    redacted_vmess = {
        "name": "vmess-node",
        "type": "vmess",
        "server": "1.1.1.1",
        "port": 443,
        "uuid": "[REDACTED]",
    }
    assert is_node_credential_missing(redacted_vmess)

    redacted_reality = {
        "name": "vless-reality",
        "type": "vless",
        "server": "1.1.1.1",
        "port": 443,
        "uuid": "11111111-2222-3333-4444-555555555555",
        "reality-opts": {"public-key": "[REDACTED]"},
    }
    assert is_node_credential_missing(redacted_reality)


def test_converter_reality_and_protocols_robustness():
    """Verify converter handles parameter variations for Reality, Hysteria2, and TUIC."""
    # 1. Reality with snake_case and top-level public_key
    vless_snake = {
        "name": "vless-reality-snake",
        "type": "vless",
        "server": "198.51.100.1",
        "port": 443,
        "uuid": "00000000-0000-0000-0000-000000000000",
        "flow": "xtls-rprx-vision",
        "reality_opts": {
            "public_key": "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
            "short_id": "abcd12",
        },
        "alpn": "h2,http/1.1",
    }
    outbound = clash_to_singbox_outbound(vless_snake)
    assert outbound is not None
    assert outbound["type"] == "vless"
    assert outbound["tls"]["reality"]["enabled"] is True
    assert outbound["tls"]["reality"]["public_key"] == "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo"
    assert outbound["tls"]["reality"]["short_id"] == "abcd12"
    assert outbound["tls"]["alpn"] == ["h2", "http/1.1"]

    # 2. Hysteria2 with auth alias, obfs dict, and string ports
    hy2_node = {
        "name": "hy2-advanced",
        "type": "hysteria2",
        "server": "hy2.example.com",
        "port": 0,
        "ports": "443,10000-20000",
        "auth": "my-secret-token",
        "obfs": {"type": "salamander", "password": "obfs-password"},
        "alpn": "h3",
        "up": "100 Mbps",
        "down": "300 Mbps",
    }
    hy2_ob = clash_to_singbox_outbound(hy2_node)
    assert hy2_ob is not None
    assert hy2_ob["type"] == "hysteria2"
    assert hy2_ob["server_port"] == 443
    assert hy2_ob["password"] == "my-secret-token"
    assert hy2_ob["obfs"]["type"] == "salamander"
    assert hy2_ob["obfs"]["password"] == "obfs-password"
    assert hy2_ob["tls"]["alpn"] == ["h3"]
    assert hy2_ob.get("up_mbps") == 100
    assert hy2_ob.get("down_mbps") == 300

    # 3. TUIC with token and string alpn
    tuic_node = {
        "name": "tuic-node",
        "type": "tuic",
        "server": "tuic.example.com",
        "port": 8443,
        "uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
        "token": "tuic-pass",
        "congestion_controller": "bbr",
        "udp_relay_mode": "native",
        "alpn": "h3",
    }
    tuic_ob = clash_to_singbox_outbound(tuic_node)
    assert tuic_ob is not None
    assert tuic_ob["type"] == "tuic"
    assert tuic_ob["uuid"] == "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    assert tuic_ob["password"] == "tuic-pass"
    assert tuic_ob["congestion_control"] == "bbr"
    assert tuic_ob["udp_relay_mode"] == "native"
    assert tuic_ob["tls"]["alpn"] == ["h3"]


@pytest.mark.asyncio
async def test_credential_hydration_from_subscription(async_session: AsyncSession):
    """Test hydrating missing credentials from subscription raw_nodes."""
    full_vless = {
        "name": "US-VLESS",
        "type": "vless",
        "server": "us.vless.com",
        "port": 443,
        "uuid": "12345678-1234-1234-1234-123456789abc",
        "flow": "xtls-rprx-vision",
        "tls": True,
        "servername": "yahoo.com",
        "reality-opts": {
            "public-key": "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
            "short-id": "1234",
        },
        "client-fingerprint": "chrome",
    }
    full_ss = {
        "name": "HK-SS",
        "type": "ss",
        "server": "hk.ss.com",
        "port": 8388,
        "cipher": "aes-256-gcm",
        "password": "super-secret-ss-pass",
    }

    sub = Subscription(
        name="MainSub",
        url="https://example.com/sub",
        enabled=True,
        raw_nodes=[full_vless, full_ss],
    )
    async_session.add(sub)
    await async_session.commit()
    await async_session.refresh(sub)

    # Simulating what frontend or list_final_nodes provides (stripped of secrets)
    stripped_vless = {
        "name": "US-VLESS",
        "subscription_id": sub.id,
        "subscription_name": "MainSub",
        "type": "vless",
        "server": "us.vless.com",
        "port": 443,
        "tls": True,
        "sni": "yahoo.com",
    }
    stripped_ss = {
        "name": "HK-SS",
        "subscription_id": sub.id,
        "subscription_name": "MainSub",
        "type": "ss",
        "server": "hk.ss.com",
        "port": 8388,
        "cipher": "aes-256-gcm",
    }

    # Before hydration, conversion should fail due to missing credentials
    assert clash_to_singbox_outbound(stripped_vless) is None
    assert clash_to_singbox_outbound(stripped_ss) is None

    # Perform batch hydration
    hydrated_list = await hydrate_nodes_batch([stripped_vless, stripped_ss], db=async_session)
    assert len(hydrated_list) == 2

    # Verify VLESS is hydrated and converts successfully
    hydrated_vless = hydrated_list[0]
    assert hydrated_vless["uuid"] == "12345678-1234-1234-1234-123456789abc"
    assert "reality-opts" in hydrated_vless
    vless_ob = clash_to_singbox_outbound(hydrated_vless)
    assert vless_ob is not None
    assert vless_ob["uuid"] == "12345678-1234-1234-1234-123456789abc"
    assert vless_ob["tls"]["reality"]["enabled"] is True

    # Verify SS is hydrated and converts successfully
    hydrated_ss = hydrated_list[1]
    assert hydrated_ss["password"] == "super-secret-ss-pass"
    ss_ob = clash_to_singbox_outbound(hydrated_ss)
    assert ss_ob is not None
    assert ss_ob["password"] == "super-secret-ss-pass"


@pytest.mark.asyncio
async def test_credential_hydration_from_node_repository(async_session: AsyncSession):
    """Test hydrating missing credentials from Node table via NodeRepository."""
    full_payload = {
        "name": "JP-VMess",
        "type": "vmess",
        "server": "jp.vmess.com",
        "port": 443,
        "uuid": "87654321-4321-4321-4321-cba987654321",
        "cipher": "auto",
        "alterId": 0,
        "network": "ws",
        "ws-opts": {"path": "/vmess-path"},
    }

    node, created = await NodeRepository.upsert_by_fingerprint(
        session=async_session,
        name="JP-VMess",
        protocol="vmess",
        server="jp.vmess.com",
        port=443,
        normalized_payload=full_payload,
        payload_fingerprint="fingerprint-jp-vmess-001",
        lifecycle_state="active",
        logical_id="logical-id-jp-001",
    )
    await async_session.commit()

    # Stripped node
    stripped_vmess = {
        "name": "JP-VMess",
        "type": "vmess",
        "server": "jp.vmess.com",
        "port": 443,
        "logical_id": "logical-id-jp-001",
    }

    hydrated = await hydrate_nodes_batch([stripped_vmess], db=async_session)
    assert len(hydrated) == 1
    assert hydrated[0]["uuid"] == "87654321-4321-4321-4321-cba987654321"

    ob = clash_to_singbox_outbound(hydrated[0])
    assert ob is not None
    assert ob["uuid"] == "87654321-4321-4321-4321-cba987654321"
    assert ob["transport"]["path"] == "/vmess-path"


@pytest.mark.asyncio
async def test_probe_single_and_batch_hydration_integration(async_session: AsyncSession, monkeypatch):
    """Verify that probe_single_node and probe_batch_nodes seamlessly hydrate stripped nodes."""
    full_vless = {
        "name": "Test-VLESS",
        "type": "vless",
        "server": "127.0.0.1",
        "port": 8443,
        "uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
        "reality-opts": {
            "public-key": "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
            "short-id": "1234",
        },
    }
    sub = Subscription(
        name="IntegrationSub",
        url="https://example.com/sub",
        enabled=True,
        raw_nodes=[full_vless],
    )
    async_session.add(sub)
    await async_session.commit()

    received_nodes: list[dict[str, Any]] = []

    @asynccontextmanager
    async def fake_spawn_node_runner(node: dict[str, Any], runner_bin=None):
        received_nodes.append(dict(node))
        yield {"proxy_url": "http://127.0.0.1:21000", "port": 21000, "is_runner": True}

    async def fake_check_transport(proxy_url, timeout_s=3.0):
        return {"status": "ok", "latency_ms": 25.0}

    async def fake_check_geo(proxy_url, timeout_s=3.0):
        return {"ip": "1.2.3.4", "country": "US", "asn": "AS123", "organization": "Test"}

    monkeypatch.setattr("app.services.probe.service.spawn_node_runner", fake_spawn_node_runner)
    monkeypatch.setattr("app.services.probe.service.check_transport", fake_check_transport)
    monkeypatch.setattr("app.services.probe.service.check_geo_identity", fake_check_geo)

    stripped_vless = {
        "name": "Test-VLESS",
        "type": "vless",
        "server": "127.0.0.1",
        "port": 8443,
    }

    # 1. Test probe_single_node
    res_single = await probe_single_node(
        stripped_vless,
        media_check_enabled=False,
        speedtest_enabled=False,
        use_cache=False,
        db=async_session,
    )
    assert res_single["status"] == "ok"
    assert len(received_nodes) == 1
    assert received_nodes[0]["uuid"] == "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    assert "reality-opts" in received_nodes[0]

    # 2. Test probe_batch_nodes
    received_nodes.clear()
    res_batch = await probe_batch_nodes(
        [stripped_vless],
        media_check_enabled=False,
        speedtest_enabled=False,
        use_cache=False,
        db=async_session,
    )
    assert res_batch["summary"]["ok"] == 1
    assert len(received_nodes) == 1
    assert received_nodes[0]["uuid"] == "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"


@pytest.mark.asyncio
async def test_probe_batch_route_credential_hydration(client, async_session: AsyncSession, monkeypatch):
    """Verify that POST /api/probe/batch route hydrates stripped nodes from database."""
    full_hy2 = {
        "name": "Route-HY2",
        "type": "hysteria2",
        "server": "127.0.0.1",
        "port": 8443,
        "password": "hy2-secret-auth-key",
    }
    sub = Subscription(
        name="RouteSub",
        url="https://example.com/route-sub",
        enabled=True,
        raw_nodes=[full_hy2],
    )
    async_session.add(sub)
    await async_session.commit()

    received_nodes: list[dict[str, Any]] = []

    @asynccontextmanager
    async def fake_spawn_node_runner(node: dict[str, Any], runner_bin=None):
        received_nodes.append(dict(node))
        yield {"proxy_url": "http://127.0.0.1:21000", "port": 21000, "is_runner": True}

    async def fake_check_transport(proxy_url, timeout_s=3.0):
        return {"status": "ok", "latency_ms": 15.0}

    async def fake_check_geo(proxy_url, timeout_s=3.0):
        return {"ip": "1.2.3.4", "country": "JP", "asn": "AS456", "organization": "RouteTest"}

    monkeypatch.setattr("app.services.probe.service.spawn_node_runner", fake_spawn_node_runner)
    monkeypatch.setattr("app.services.probe.service.check_transport", fake_check_transport)
    monkeypatch.setattr("app.services.probe.service.check_geo_identity", fake_check_geo)

    stripped_hy2 = {
        "name": "Route-HY2",
        "type": "hysteria2",
        "server": "127.0.0.1",
        "port": 8443,
    }

    res = await client.post(
        "/api/probe/batch",
        json={
            "nodes": [stripped_hy2],
            "include_speed": False,
            "include_media": False,
            "use_cache": False,
        },
    )
    assert res.status_code == 200
    data = res.json()
    assert data["summary"]["ok"] == 1
    assert len(received_nodes) == 1
    assert received_nodes[0]["password"] == "hy2-secret-auth-key"
