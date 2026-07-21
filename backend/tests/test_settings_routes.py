import pytest


@pytest.mark.asyncio
async def test_security_settings_get(client):
    res = await client.get("/api/settings/security")
    assert res.status_code == 200, res.text
    data = res.json()
    assert "auth_enabled" in data
    assert "protect_api" in data


@pytest.mark.asyncio
async def test_export_and_reset_config(client):
    created = await client.post(
        "/api/subscriptions",
        json={
            "name": "reset-me",
            "url": "https://example.com/reset",
            "manual_nodes": [
                {"name": "节点R", "type": "ss", "server": "9.9.9.9", "port": 9},
            ],
        },
    )
    assert created.status_code == 201, created.text

    exported = await client.get("/api/settings/export?include_subscriptions=true")
    assert exported.status_code == 200, exported.text
    payload = exported.json()
    assert "exported_at" in payload
    tables = payload.get("tables") or {}
    assert "subscriptions" in tables
    assert any(item.get("name") == "reset-me" for item in tables["subscriptions"])

    reset = await client.post("/api/settings/reset")
    assert reset.status_code == 200, reset.text

    subs = await client.get("/api/subscriptions")
    assert subs.status_code == 200
    assert subs.json() == []
