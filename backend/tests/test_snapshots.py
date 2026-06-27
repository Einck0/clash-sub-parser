import pytest


@pytest.mark.asyncio
async def test_create_snapshot(client):
    response = await client.post("/api/snapshots", json={
        "label": "test-snapshot",
        "description": "test",
    })
    assert response.status_code == 200
    data = response.json()
    assert data["label"] == "test-snapshot"


@pytest.mark.asyncio
async def test_list_snapshots(client):
    await client.post("/api/snapshots", json={"label": "snap1"})
    response = await client.get("/api/snapshots")
    assert response.status_code == 200
    assert len(response.json()) >= 1
