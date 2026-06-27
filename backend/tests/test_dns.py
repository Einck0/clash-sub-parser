import pytest


@pytest.mark.asyncio
async def test_get_dns(client):
    response = await client.get("/api/dns")
    assert response.status_code == 200
    data = response.json()
    assert "raw_yaml" in data


@pytest.mark.asyncio
async def test_update_dns(client):
    response = await client.patch("/api/dns", json={
        "raw_yaml": "dns:\n  enable: true\n  nameserver:\n    - 8.8.8.8",
        "enabled": True,
    })
    assert response.status_code == 200
    assert response.json()["enabled"] is True
