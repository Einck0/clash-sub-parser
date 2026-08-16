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


@pytest.mark.asyncio
async def test_login_session_uses_hash_cookie_and_enforces_csrf(client):
    configured = await client.patch(
        "/api/settings/security",
        json={"auth_enabled": True, "token": "secret-token"},
    )
    assert configured.status_code == 200, configured.text

    login = await client.post(
        "/api/settings/auth/login",
        json={"token": "secret-token"},
    )
    assert login.status_code == 200, login.text
    assert "clash_auth_hash=" in login.headers.get("set-cookie", "")

    session = await client.post("/api/settings/auth/check")
    assert session.status_code == 200, session.text

    without_csrf = await client.post(
        "/api/subscriptions",
        json={"name": "csrf-blocked", "url": "https://example.com/sub"},
    )
    assert without_csrf.status_code == 403, without_csrf.text

    with_csrf = await client.post(
        "/api/subscriptions",
        headers={"X-Clash-CSRF": "1"},
        json={"name": "csrf-accepted", "url": "https://example.com/sub"},
    )
    assert with_csrf.status_code == 201, with_csrf.text
