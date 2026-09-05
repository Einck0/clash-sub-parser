import pytest
from httpx import ASGITransport, AsyncClient

from app.main import app


@pytest.mark.asyncio
async def test_probe_settings_crud():
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        # 1. Get default settings
        res = await ac.get("/api/probe/settings")
        assert res.status_code == 200
        data = res.json()
        assert data["probe_enabled"] is True
        assert data["speedtest_enabled"] is False
        assert "youtube" in data["media_platforms"]

        # 2. Update settings
        patch_res = await ac.patch(
            "/api/probe/settings",
            json={
                "speedtest_enabled": True,
                "speedtest_timeout_s": 8,
                "probe_concurrency": 10,
            },
        )
        assert patch_res.status_code == 200
        patch_data = patch_res.json()
        assert patch_data["speedtest_enabled"] is True
        assert patch_data["speedtest_timeout_s"] == 8
        assert patch_data["probe_concurrency"] == 10

        # Restore default
        await ac.patch("/api/probe/settings", json={"speedtest_enabled": False, "speedtest_timeout_s": 5, "probe_concurrency": 5})


@pytest.mark.asyncio
async def test_probe_tcp_endpoint():
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        res = await ac.post(
            "/api/probe/tcp",
            json={
                "nodes": [
                    {"name": "test-1", "server": "127.0.0.1", "port": 99999},  # 无效端口跳过或失败
                ]
            },
        )
        assert res.status_code == 200
        data = res.json()
        assert "summary" in data
        assert "results" in data


@pytest.mark.asyncio
async def test_probe_cache_endpoints():
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        res = await ac.get("/api/probe/cache")
        assert res.status_code == 200
        assert "cache" in res.json()

        del_res = await ac.delete("/api/probe/cache")
        assert del_res.status_code == 200
        assert del_res.json()["ok"] is True


import urllib.parse
from app.database import get_db
from app.models.node_probe_result import NodeProbeResult
from sqlalchemy import select, delete


@pytest.mark.asyncio
async def test_probe_results_envelope_and_summary_projection():
    """1.1 Verify GET /api/probe/results returns frozen lightweight summary envelope."""
    async for db in get_db():
        # Clean up existing rows
        await db.execute(delete(NodeProbeResult))
        await db.commit()

        # Insert test records
        nodes = [
            NodeProbeResult(
                node_key="node:001:jp",
                name="Node-Japan-01",
                server="192.0.2.1",
                port=443,
                type="vmess",
                status="ok",
                latency_ms=45,
                speed_mbps=88.5,
                ip="203.0.113.1",
                country="JP",
                asn=12345,
                organization="Test AS Org",
                media={
                    "youtube": {"status": "ok", "region": "JP"},
                    "netflix": {"status": "verified", "verdict": "full", "unlocked": True},
                    "disney": {"status": "fail"},
                    "_identity": {
                        "identity_evidence": {"ip": "203.0.113.1", "confidence": "high"},
                        "confidence": "verified",
                    },
                },
                error=None,
                checked_at=1788500000,
            ),
            NodeProbeResult(
                node_key="node:002:us",
                name="Node-US-02",
                server="192.0.2.2",
                port=8388,
                type="ss",
                status="fail",
                latency_ms=None,
                speed_mbps=None,
                ip=None,
                country="US",
                asn=None,
                organization=None,
                media={"youtube": False},
                error="Timeout connecting",
                checked_at=1788500001,
            ),
        ]
        db.add_all(nodes)
        await db.commit()

    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        res = await ac.get("/api/probe/results")
        assert res.status_code == 200
        raw_body = res.content
        assert len(raw_body) <= 51200, f"Payload exceeded 51,200 bytes: {len(raw_body)}"

        data = res.json()
        assert "results" in data
        assert "next_cursor" in data
        assert "has_more" in data
        assert isinstance(data["results"], dict)
        assert data["has_more"] is False
        assert data["next_cursor"] is None

        # Verify keyed ONLY by node_key, no name aliases
        assert "node:001:jp" in data["results"]
        assert "node:002:us" in data["results"]
        assert "Node-Japan-01" not in data["results"]
        assert "Node-US-02" not in data["results"]

        allowed_summary_keys = {"status", "latency_ms", "speed_mbps", "country", "ip", "media"}
        forbidden_keys = {
            "name", "server", "port", "type", "asn", "organization",
            "error", "checked_at", "identity_evidence", "identity_confidence",
            "diagnostics", "evidence", "password", "uuid"
        }

        for node_k, summary in data["results"].items():
            assert set(summary.keys()) == allowed_summary_keys, f"Summary keys mismatch: {summary.keys()}"
            for fk in forbidden_keys:
                assert fk not in summary, f"Forbidden key '{fk}' leaked in summary"

            # Media must be a dict of booleans
            assert isinstance(summary["media"], dict)
            for m_k, m_v in summary["media"].items():
                assert isinstance(m_v, bool), f"Media value for {m_k} must be bool, got {type(m_v)}"

        # Verify media unlock predicates
        jp_media = data["results"]["node:001:jp"]["media"]
        assert jp_media.get("youtube") is True
        assert jp_media.get("netflix") is True
        assert jp_media.get("disney") is False
        # Platforms not tested must be false
        assert jp_media.get("chatgpt") is False


@pytest.mark.asyncio
async def test_probe_results_pagination_limits_and_budget():
    """1.1 Verify pagination parameters, limit validation (422), cursor continuation, and 50KB ceiling."""
    async for db in get_db():
        await db.execute(delete(NodeProbeResult))
        await db.commit()

        # Insert 10 ordered records
        nodes = [
            NodeProbeResult(
                node_key=f"node:{i:03d}",
                name=f"Node-{i:03d}",
                server=f"192.0.2.{i}",
                port=443,
                type="ss",
                status="ok",
                latency_ms=50 + i,
                speed_mbps=10.0 + i,
                ip=f"203.0.113.{i}",
                country="HK",
                media={"youtube": True},
                checked_at=1788500000 + i,
            )
            for i in range(10)
        ]
        db.add_all(nodes)
        await db.commit()

    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        # Invalid limits should return 422
        res_0 = await ac.get("/api/probe/results?limit=0")
        assert res_0.status_code == 422

        res_neg = await ac.get("/api/probe/results?limit=-1")
        assert res_neg.status_code == 422

        res_over = await ac.get("/api/probe/results?limit=101")
        assert res_over.status_code == 422

        # Page 1 with limit=4
        res_p1 = await ac.get("/api/probe/results?limit=4")
        assert res_p1.status_code == 200
        p1 = res_p1.json()
        assert len(p1["results"]) == 4
        assert p1["has_more"] is True
        assert p1["next_cursor"] == "node:003"
        assert list(p1["results"].keys()) == [f"node:{i:03d}" for i in range(4)]

        # Page 2 with cursor
        res_p2 = await ac.get(f"/api/probe/results?limit=4&cursor={p1['next_cursor']}")
        assert res_p2.status_code == 200
        p2 = res_p2.json()
        assert len(p2["results"]) == 4
        assert p2["has_more"] is True
        assert p2["next_cursor"] == "node:007"
        assert list(p2["results"].keys()) == [f"node:{i:03d}" for i in range(4, 8)]

        # Page 3 (final)
        res_p3 = await ac.get(f"/api/probe/results?limit=4&cursor={p2['next_cursor']}")
        assert res_p3.status_code == 200
        p3 = res_p3.json()
        assert len(p3["results"]) == 2
        assert p3["has_more"] is False
        assert p3["next_cursor"] is None
        assert list(p3["results"].keys()) == ["node:008", "node:009"]


@pytest.mark.asyncio
async def test_probe_results_byte_budget_enforcement():
    """1.1 Verify serialized UTF-8 payload never exceeds 51,200 bytes even with many nodes."""
    async for db in get_db():
        await db.execute(delete(NodeProbeResult))
        await db.commit()

        # Insert 300 nodes to test byte capping
        nodes = [
            NodeProbeResult(
                node_key=f"node:deep:budget:key:testing:prefix:length:{i:04d}",
                name=f"Node-Budget-{i:04d}",
                server="203.0.113.100",
                port=8080,
                type="vmess",
                status="ok",
                latency_ms=100,
                speed_mbps=50.0,
                ip="198.51.100.5",
                country="US",
                media={
                    "youtube": {"status": "ok"},
                    "netflix": {"status": "ok"},
                    "disney": {"status": "ok"},
                    "chatgpt": {"status": "ok"},
                    "bilibili": {"status": "ok"},
                    "meta_ai": {"status": "ok"},
                    "gemini": {"status": "ok"},
                },
                checked_at=1788500000 + i,
            )
            for i in range(300)
        ]
        db.add_all(nodes)
        await db.commit()

    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        # Default limit=100
        res = await ac.get("/api/probe/results")
        assert res.status_code == 200
        raw_body = res.content
        assert len(raw_body) <= 51200, f"Payload size {len(raw_body)} exceeded 51,200 bytes"
        data = res.json()
        assert data["has_more"] is True
        assert data["next_cursor"] is not None
        assert len(data["results"]) > 0


@pytest.mark.asyncio
async def test_probe_results_detail_endpoint():
    """1.2 Verify GET /api/probe/results/detail exact-key, 422 on blank, 404 on missing, sanitization."""
    test_node_key = "node:exact:test:key#01"
    async for db in get_db():
        await db.execute(delete(NodeProbeResult))
        await db.commit()

        node = NodeProbeResult(
            node_key=test_node_key,
            name="Node-Exact-Detail-Test",
            server="192.0.2.99",
            port=443,
            type="ss",
            status="ok",
            latency_ms=62,
            speed_mbps=120.4,
            ip="203.0.113.88",
            country="SG",
            asn=54321,
            organization="Singapore Tel",
            media={
                "youtube": {
                    "status": "verified",
                    "verdict": "full",
                    "unlocked": True,
                    "evidence": {
                        "http_status": 200,
                        "final_host": "www.youtube.com",
                        "signals": ["playback_verified"],
                    },
                },
                "_identity": {
                    "identity_evidence": {
                        "ip": "203.0.113.88",
                        "asn": 54321,
                    },
                    "confidence": "verified",
                },
            },
            error=None,
            checked_at=1788500000,
        )
        db.add(node)
        await db.commit()

    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        # 1. Missing node_key -> 422
        res_missing = await ac.get("/api/probe/results/detail")
        assert res_missing.status_code == 422

        # 2. Blank node_key -> 422
        res_blank = await ac.get("/api/probe/results/detail?node_key=")
        assert res_blank.status_code == 422
        res_spaces = await ac.get("/api/probe/results/detail?node_key=%20%20%20")
        assert res_spaces.status_code == 422

        # 3. Unknown node_key -> 404
        res_unknown = await ac.get("/api/probe/results/detail?node_key=nonexistent-node-key")
        assert res_unknown.status_code == 404

        # 4. Lookup by name must NOT match (exact node_key match only) -> 404
        res_by_name = await ac.get("/api/probe/results/detail?node_key=Node-Exact-Detail-Test")
        assert res_by_name.status_code == 404

        # 5. Exact URL-encoded node_key -> 200
        encoded_key = urllib.parse.quote(test_node_key)
        res_ok = await ac.get(f"/api/probe/results/detail?node_key={encoded_key}")
        assert res_ok.status_code == 200
        detail = res_ok.json()

        assert detail["node_key"] == test_node_key
        assert detail["name"] == "Node-Exact-Detail-Test"
        assert detail["server"] == "192.0.2.99"
        assert detail["port"] == 443
        assert detail["type"] == "ss"
        assert detail["status"] == "ok"
        assert detail["latency_ms"] == 62
        assert detail["speed_mbps"] == 120.4
        assert detail["ip"] == "203.0.113.88"
        assert detail["country"] == "SG"
        assert detail["asn"] == 54321
        assert detail["organization"] == "Singapore Tel"
        assert "youtube" in detail["media"]
        assert detail["media"]["youtube"]["evidence"]["http_status"] == 200

        # Identity evidence and confidence preserved
        assert "identity_evidence" in detail
        assert detail["identity_evidence"]["ip"] == "203.0.113.88"
        assert detail["identity_confidence"] == "verified"

        # Forbidden secret check
        for forbidden in ("password", "token", "authorization", "cookie", "raw_body"):
            assert forbidden not in detail
