"""Tests for canonical node identity contract, ledger node_key, identity-bearing summary,
and current-set convergence / read fence.
"""

from __future__ import annotations

from httpx import ASGITransport, AsyncClient
import pytest
from sqlalchemy import delete, select

from app.database import get_db
from app.models.node_probe_result import NodeProbeResult
from app.models.subscription import Subscription
from app.services.node_identity import canonical_node_key
from app.services.probe.service import (
    _PROBE_CACHE,
    converge_probe_results_to_current_nodes,
    get_paged_db_probe_summary,
    set_cached_result,
)
from app.main import app


ALLOWED_SUMMARY_KEYS = {
    "node_key",
    "name",
    "status",
    "latency_ms",
    "speed_mbps",
    "country",
    "ip",
    "media",
}

FORBIDDEN_SUMMARY_KEYS = {
    "server",
    "port",
    "type",
    "asn",
    "organization",
    "error",
    "checked_at",
    "identity_evidence",
    "identity_confidence",
    "diagnostics",
    "evidence",
    "password",
    "uuid",
}


def test_canonical_node_key_format_and_trimming():
    """Verify canonical_node_key produces exact trimmed name|type|server:port format."""
    node_standard = {
        "name": "  HK-Premium-01 ",
        "type": " vmess ",
        "server": " 192.0.2.10 ",
        "port": 443,
    }
    assert canonical_node_key(node_standard) == "HK-Premium-01|vmess|192.0.2.10:443"

    node_string_port = {
        "name": "US-Node",
        "type": "ss",
        "server": "198.51.100.1",
        "port": " 8388 ",
    }
    assert canonical_node_key(node_string_port) == "US-Node|ss|198.51.100.1:8388"

    node_missing_port = {
        "name": "NoPort-Node",
        "type": "trojan",
        "server": "203.0.113.5",
        "port": None,
    }
    assert canonical_node_key(node_missing_port) == "NoPort-Node|trojan|203.0.113.5:"

    node_zero_port = {
        "name": "ZeroPort-Node",
        "type": "shadowsocks",
        "server": "203.0.113.6",
        "port": 0,
    }
    assert canonical_node_key(node_zero_port) == "ZeroPort-Node|shadowsocks|203.0.113.6:0"


def test_canonical_node_key_duplicate_name_isolation():
    """Two nodes with identical display names but different endpoints produce distinct keys."""
    n1 = {"name": "SameName", "type": "ss", "server": "1.1.1.1", "port": 8388}
    n2 = {"name": "SameName", "type": "vmess", "server": "2.2.2.2", "port": 443}
    k1 = canonical_node_key(n1)
    k2 = canonical_node_key(n2)
    assert k1 != k2
    assert k1 == "SameName|ss|1.1.1.1:8388"
    assert k2 == "SameName|vmess|2.2.2.2:443"


@pytest.mark.asyncio
async def test_node_ledger_item_requires_node_key_and_preserves_duplicate_names():
    """Verify GET /api/proxy-chains/meta/node-ledger supplies required node_key and carries distinct duplicate-name nodes."""
    async for db in get_db():
        await db.execute(delete(Subscription))
        await db.commit()

        # Insert subscription with duplicate display name nodes but different endpoints
        sub = Subscription(
            name="sub-dup-test",
            url="https://example.com/sub",
            enabled=True,
            raw_nodes=[
                {"name": "Shared-Name", "type": "ss", "server": "10.0.0.1", "port": 1001},
                {"name": "Shared-Name", "type": "vmess", "server": "10.0.0.2", "port": 1002},
                {"name": "Unique-Node", "type": "trojan", "server": "10.0.0.3", "port": 1003},
            ],
        )
        db.add(sub)
        await db.commit()

    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        res = await ac.get("/api/proxy-chains/meta/node-ledger")
        assert res.status_code == 200, res.text
        data = res.json()
        assert len(data) == 3, f"Expected 3 ledger rows including duplicate names, got {len(data)}"

        keys = [item["node_key"] for item in data]
        assert len(set(keys)) == 3, f"All ledger rows must have unique canonical keys: {keys}"
        assert all(isinstance(k, str) and len(k) > 0 for k in keys)

        k1 = canonical_node_key({"name": "Shared-Name", "type": "ss", "server": "10.0.0.1", "port": 1001})
        k2 = canonical_node_key({"name": "Shared-Name", "type": "vmess", "server": "10.0.0.2", "port": 1002})
        assert k1 in keys
        assert k2 in keys


@pytest.mark.asyncio
async def test_probe_summary_identity_bearing_privacy_safe_and_status_preservation():
    """Verify GET /api/probe/results returns node_key and name in summary items, preserves timeout/skipped, and strictly redacts sensitive fields."""
    async for db in get_db():
        await db.execute(delete(Subscription))
        await db.execute(delete(NodeProbeResult))
        await db.commit()

        # Seed enabled subscription to satisfy read fence
        sub = Subscription(
            name="sub-probe-test",
            url="https://example.com/sub",
            enabled=True,
            raw_nodes=[
                {"name": "Node-OK", "type": "vmess", "server": "1.1.1.1", "port": 443},
                {"name": "Node-Timeout", "type": "ss", "server": "2.2.2.2", "port": 8388},
                {"name": "Node-Skipped", "type": "trojan", "server": "3.3.3.3", "port": 443},
            ],
        )
        db.add(sub)
        await db.commit()

    key_ok = canonical_node_key({"name": "Node-OK", "type": "vmess", "server": "1.1.1.1", "port": 443})
    key_timeout = canonical_node_key({"name": "Node-Timeout", "type": "ss", "server": "2.2.2.2", "port": 8388})
    key_skipped = canonical_node_key({"name": "Node-Skipped", "type": "trojan", "server": "3.3.3.3", "port": 443})

    async for db in get_db():
        db_records = [
            NodeProbeResult(
                node_key=key_ok,
                name="Node-OK",
                server="1.1.1.1",
                port=443,
                type="vmess",
                status="ok",
                latency_ms=35,
                speed_mbps=12.5,
                ip="1.1.1.1",
                country="JP",
                asn=13335,
                organization="Cloudflare Inc",
                media={"youtube": {"status": "ok"}, "netflix": True},
                error=None,
                checked_at=1788600000,
            ),
            NodeProbeResult(
                node_key=key_timeout,
                name="Node-Timeout",
                server="2.2.2.2",
                port=8388,
                type="ss",
                status="timeout",
                latency_ms=None,
                speed_mbps=None,
                ip=None,
                country="US",
                asn=None,
                organization=None,
                media={},
                error="Connection timed out after 5000ms",
                checked_at=1788600001,
            ),
            NodeProbeResult(
                node_key=key_skipped,
                name="Node-Skipped",
                server="3.3.3.3",
                port=443,
                type="trojan",
                status="skipped",
                latency_ms=None,
                speed_mbps=None,
                ip=None,
                country=None,
                asn=None,
                organization=None,
                media={},
                error=None,
                checked_at=1788600002,
            ),
        ]
        db.add_all(db_records)
        await db.commit()

    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        res = await ac.get("/api/probe/results")
        assert res.status_code == 200, res.text
        assert len(res.content) <= 51200, f"Payload exceeded 51,200 bytes: {len(res.content)}"

        body = res.json()
        results = body.get("results", {})
        assert len(results) == 3

        # Key equality: mapKey == item.node_key
        for map_key, item in results.items():
            assert item.get("node_key") == map_key, f"Expected item.node_key == map_key, got {item.get('node_key')} vs {map_key}"
            assert set(item.keys()) == ALLOWED_SUMMARY_KEYS, f"Summary item keys mismatch: {item.keys()}"
            for fk in FORBIDDEN_SUMMARY_KEYS:
                assert fk not in item, f"Forbidden key '{fk}' leaked in summary: {item}"

        assert results[key_ok]["name"] == "Node-OK"
        assert results[key_ok]["status"] == "ok"
        assert results[key_timeout]["name"] == "Node-Timeout"
        assert results[key_timeout]["status"] == "timeout"
        assert results[key_skipped]["name"] == "Node-Skipped"
        assert results[key_skipped]["status"] == "skipped"


@pytest.mark.asyncio
async def test_convergence_cleans_orphans_and_cache_and_is_idempotent():
    """Verify converge_probe_results_to_current_nodes removes orphans from DB and cache, retains current, returns aggregate counts, and is idempotent."""
    async for db in get_db():
        await db.execute(delete(Subscription))
        await db.execute(delete(NodeProbeResult))
        await db.commit()

        # Enabled subscription with 2 current nodes
        sub = Subscription(
            name="sub-active",
            url="https://example.com/sub",
            enabled=True,
            raw_nodes=[
                {"name": "Current-1", "type": "vmess", "server": "1.1.1.1", "port": 443},
                {"name": "Current-2", "type": "ss", "server": "2.2.2.2", "port": 8388},
            ],
        )
        db.add(sub)
        await db.commit()

        k_current1 = canonical_node_key(sub.raw_nodes[0])
        k_current2 = canonical_node_key(sub.raw_nodes[1])
        k_orphan1 = "Orphan-Old|ss|9.9.9.9:8388"
        k_orphan2 = "Orphan-Renamed|vmess|8.8.8.8:443"

        db.add_all([
            NodeProbeResult(node_key=k_current1, name="Current-1", server="1.1.1.1", port=443, type="vmess", status="ok", checked_at=1788600000),
            NodeProbeResult(node_key=k_current2, name="Current-2", server="2.2.2.2", port=8388, type="ss", status="ok", checked_at=1788600000),
            NodeProbeResult(node_key=k_orphan1, name="Orphan-Old", server="9.9.9.9", port=8388, type="ss", status="fail", checked_at=1788500000),
            NodeProbeResult(node_key=k_orphan2, name="Orphan-Renamed", server="8.8.8.8", port=443, type="vmess", status="ok", checked_at=1788500000),
        ])
        await db.commit()

        # Put entries in in-memory cache
        set_cached_result({"name": "Current-1", "type": "vmess", "server": "1.1.1.1", "port": 443}, {"status": "ok"})
        set_cached_result({"name": "Orphan-Old", "type": "ss", "server": "9.9.9.9", "port": 8388}, {"status": "fail"})
        set_cached_result({"name": "Cache-Only-Orphan", "type": "ss", "server": "7.7.7.7", "port": 8388}, {"status": "fail"})
        assert k_orphan1 in _PROBE_CACHE
        assert k_current1 in _PROBE_CACHE
        assert canonical_node_key({"name": "Cache-Only-Orphan", "type": "ss", "server": "7.7.7.7", "port": 8388}) in _PROBE_CACHE

        # First convergence run: should delete 2 orphans and retain 2
        stats = await converge_probe_results_to_current_nodes(db)
        assert stats == {"retained": 2, "deleted": 2}

        # Cache check: orphan evicted, current retained
        assert k_orphan1 not in _PROBE_CACHE
        assert k_current1 in _PROBE_CACHE
        cache_only_orphan_key = canonical_node_key({"name": "Cache-Only-Orphan", "type": "ss", "server": "7.7.7.7", "port": 8388})
        assert cache_only_orphan_key not in _PROBE_CACHE

        # DB check
        remaining = list((await db.execute(select(NodeProbeResult.node_key))).scalars().all())
        assert set(remaining) == {k_current1, k_current2}

        # Second convergence run: idempotent
        stats2 = await converge_probe_results_to_current_nodes(db)
        assert stats2 == {"retained": 2, "deleted": 0}


@pytest.mark.asyncio
async def test_summary_read_fence_excludes_late_writes():
    """Verify get_paged_db_probe_summary read fence ignores late-written removed nodes."""
    async for db in get_db():
        await db.execute(delete(Subscription))
        await db.execute(delete(NodeProbeResult))
        await db.commit()

        # Subscription has only 1 node
        sub = Subscription(
            name="sub-fence",
            url="https://example.com/sub",
            enabled=True,
            raw_nodes=[
                {"name": "Active-Node", "type": "vmess", "server": "1.1.1.1", "port": 443},
            ],
        )
        db.add(sub)
        await db.commit()

        k_active = canonical_node_key(sub.raw_nodes[0])
        k_late_removed = "Removed-Late-Writer|ss|9.9.9.9:8388"

        db.add_all([
            NodeProbeResult(node_key=k_active, name="Active-Node", server="1.1.1.1", port=443, type="vmess", status="ok", checked_at=1788600000),
            NodeProbeResult(node_key=k_late_removed, name="Removed-Late-Writer", server="9.9.9.9", port=8388, type="ss", status="ok", checked_at=1788600001),
        ])
        await db.commit()

        summary_page = await get_paged_db_probe_summary(db, limit=100)
        assert k_active in summary_page["results"]
        assert k_late_removed not in summary_page["results"], "Read fence must exclude late-written removed node"
        assert len(summary_page["results"]) == 1


@pytest.mark.asyncio
async def test_create_subscription_converges_existing_probe_orphans():
    """Verify publishing a newly created enabled node set prunes stale probe records."""
    from app.schemas.subscription import SubscriptionCreate
    from app.services.subscription_service import create_subscription

    orphan_key = "Orphan-Before-Create|ss|9.9.9.9:8388"
    node = {"name": "Created-Node", "type": "ss", "server": "1.1.1.1", "port": 8388}

    async for db in get_db():
        await db.execute(delete(Subscription))
        await db.execute(delete(NodeProbeResult))
        await db.commit()
        db.add(NodeProbeResult(
            node_key=orphan_key,
            name="Orphan-Before-Create",
            server="9.9.9.9",
            port=8388,
            type="ss",
            status="ok",
            checked_at=1788600000,
        ))
        await db.commit()

        await create_subscription(
            db,
            SubscriptionCreate(
                name="created-subscription",
                url="manual://nodes",
                manual_nodes=[node],
            ),
        )

        remaining = list((await db.execute(select(NodeProbeResult.node_key))).scalars().all())
        assert remaining == []
        break


@pytest.mark.asyncio
async def test_create_manual_subscription_converges_existing_probe_orphans():
    """Verify publishing a manually created enabled node set prunes stale probe records."""
    from app.schemas.subscription import ManualNodeCreate
    from app.services.subscription_service import create_manual_node_subscription

    orphan_key = "Orphan-Before-Manual|ss|9.9.9.9:8388"
    node_link = (
        "vless://00000000-0000-4000-8000-000000000002@example.net:8443"
        "?type=tcp&security=reality&pbk=key&fp=chrome&sni=www.example.net"
        "&sid=beef&flow=xtls-rprx-vision#Manual-Node"
    )

    async for db in get_db():
        await db.execute(delete(Subscription))
        await db.execute(delete(NodeProbeResult))
        await db.commit()
        db.add(NodeProbeResult(
            node_key=orphan_key,
            name="Orphan-Before-Manual",
            server="9.9.9.9",
            port=8388,
            type="ss",
            status="ok",
            checked_at=1788600000,
        ))
        await db.commit()

        await create_manual_node_subscription(
            db,
            ManualNodeCreate(name="manual-subscription", node_links=node_link),
        )

        remaining = list((await db.execute(select(NodeProbeResult.node_key))).scalars().all())
        assert remaining == []
        break


@pytest.mark.asyncio
async def test_subscription_lifecycle_convergence_on_fetch_update_delete():
    """Verify convergence is triggered on successful fetch, selection update, and delete, but skipped on failed fetch."""
    from unittest.mock import patch
    from app.schemas.subscription import SubscriptionUpdate
    from app.services.subscription_service import (
        delete_subscription,
        fetch_subscription_nodes,
        update_subscription,
    )

    async for db in get_db():
        await db.execute(delete(Subscription))
        await db.execute(delete(NodeProbeResult))
        await db.commit()

        # Seed initial subscription with 2 nodes
        sub = Subscription(
            name="lifecycle-sub",
            url="https://example.com/sub",
            enabled=True,
            is_primary=True,
            node_prefix="",
            raw_nodes=[
                {"name": "Node-1", "type": "vmess", "server": "1.1.1.1", "port": 443},
                {"name": "Node-2", "type": "ss", "server": "2.2.2.2", "port": 8388},
            ],
        )
        db.add(sub)
        await db.commit()
        await db.refresh(sub)

        k1 = canonical_node_key(sub.raw_nodes[0])
        k2 = canonical_node_key(sub.raw_nodes[1])

        # Add probe results for both nodes
        db.add_all([
            NodeProbeResult(node_key=k1, name="Node-1", server="1.1.1.1", port=443, type="vmess", status="ok", checked_at=1788600000),
            NodeProbeResult(node_key=k2, name="Node-2", server="2.2.2.2", port=8388, type="ss", status="ok", checked_at=1788600000),
        ])
        await db.commit()

        # 1. Failed refresh: mock network failure in _fetch_subscription_text
        with patch("app.services.subscription_service._fetch_subscription_text", side_effect=RuntimeError("Network error")):
            with pytest.raises(RuntimeError):
                await fetch_subscription_nodes(db, sub)

        # Confirm probe results are completely preserved after failed refresh!
        recs = list((await db.execute(select(NodeProbeResult.node_key))).scalars().all())
        assert set(recs) == {k1, k2}, "Failed refresh must preserve all existing probe records"

        # 2. Successful refresh returning updated nodes (Node-1 remains, Node-2 removed, Node-3 added)
        new_yaml = """
proxies:
  - { name: "Node-1", type: "vmess", server: "1.1.1.1", port: 443, uuid: "abc" }
  - { name: "Node-3", type: "trojan", server: "3.3.3.3", port: 443, password: "xyz" }
"""
        class MockResp:
            headers = {}

        with patch("app.services.subscription_service._fetch_subscription_text", return_value=(MockResp(), new_yaml)):
            await fetch_subscription_nodes(db, sub)

        # After successful refresh: Node-2 was removed from raw_nodes, so its probe result should be converged away!
        recs_after_fetch = list((await db.execute(select(NodeProbeResult.node_key))).scalars().all())
        assert recs_after_fetch == [k1], f"Node-2 orphan probe should be converged, remaining: {recs_after_fetch}"

        # 3. Disable subscription via update_subscription -> all nodes become disabled -> orphan Node-1 deleted
        await update_subscription(db, sub, SubscriptionUpdate(enabled=False))
        recs_after_disable = list((await db.execute(select(NodeProbeResult.node_key))).scalars().all())
        assert recs_after_disable == [], f"Disabling subscription should converge away all orphan probe records: {recs_after_disable}"

        # 4. Delete subscription -> converges
        await delete_subscription(db, sub)
        recs_after_delete = list((await db.execute(select(NodeProbeResult.node_key))).scalars().all())
        assert recs_after_delete == []

