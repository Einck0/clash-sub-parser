import pytest


@pytest.mark.asyncio
async def test_latency_tcp_fallback(client):
    response = await client.post(
        "/api/latency/check",
        json={"hosts": ["127.0.0.1:1"], "timeout_ms": 200},
    )
    assert response.status_code == 200
    data = response.json()
    assert isinstance(data, list)
    assert data
    assert data[0]["mode"] == "tcp"


@pytest.mark.asyncio
async def test_geoip_server_lookup_accepts_hosts(client):
    response = await client.post(
        "/api/geoip/lookup",
        json={"hosts": ["127.0.0.1"]},
    )
    assert response.status_code == 200
    data = response.json()
    assert isinstance(data, list)
    assert data
    assert data[0]["mode"] in {"server", "exit"}
