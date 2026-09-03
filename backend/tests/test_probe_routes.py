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


@pytest.mark.asyncio
async def test_probe_results_endpoint():
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        res = await ac.get("/api/probe/results")
        assert res.status_code == 200
        assert isinstance(res.json(), dict)
