import pytest


@pytest.mark.asyncio
async def test_generate_settings_get_and_patch(client):
    got = await client.get("/api/generate/settings")
    assert got.status_code == 200, got.text
    data = got.json()
    assert data["enabled"] is True
    assert data["subscriptions"] is True

    patched = await client.patch(
        "/api/generate/settings",
        json={"rules": False, "dns": False},
    )
    assert patched.status_code == 200, patched.text
    body = patched.json()
    assert body["rules"] is False
    assert body["dns"] is False
    assert body["subscriptions"] is True

    again = await client.get("/api/generate/settings")
    assert again.status_code == 200
    assert again.json()["rules"] is False


@pytest.mark.asyncio
async def test_generate_yaml_stats_shape(client):
    created = await client.post(
        "/api/subscriptions",
        json={
            "name": "gen-stats",
            "url": "https://example.com/gen",
            "is_primary": True,
            "manual_nodes": [
                {"name": "节点A", "type": "ss", "server": "1.1.1.1", "port": 1},
            ],
        },
    )
    assert created.status_code == 201, created.text

    gen = await client.post(
        "/api/generate/yaml",
        json={
            "enabled": True,
            "subscriptions": True,
            "node_groups": True,
            "rules": True,
            "dns": False,
        },
    )
    assert gen.status_code == 200, gen.text
    payload = gen.json()
    assert "yaml" in payload
    stats = payload.get("stats") or {}
    assert stats.get("proxies", 0) >= 1
    assert "proxy_groups" in stats
    assert "rules" in stats
    assert "dialer_proxy" in stats
