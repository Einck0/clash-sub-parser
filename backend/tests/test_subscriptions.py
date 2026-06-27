import pytest


@pytest.mark.asyncio
async def test_create_subscription(client):
    response = await client.post("/api/subscriptions", json={
        "name": "test-sub",
        "url": "https://example.com/clash",
        "update_interval": 60,
        "is_primary": False,
    })
    assert response.status_code == 201
    data = response.json()
    assert data["name"] == "test-sub"
    assert data["id"] is not None


@pytest.mark.asyncio
async def test_list_subscriptions(client):
    await client.post("/api/subscriptions", json={
        "name": "sub1",
        "url": "https://example.com/1",
    })
    response = await client.get("/api/subscriptions")
    assert response.status_code == 200
    assert len(response.json()) >= 1


@pytest.mark.asyncio
async def test_update_subscription(client):
    res = await client.post("/api/subscriptions", json={
        "name": "original",
        "url": "https://example.com/orig",
    })
    sub_id = res.json()["id"]
    response = await client.patch(f"/api/subscriptions/{sub_id}", json={"name": "renamed"})
    assert response.status_code == 200
    assert response.json()["name"] == "renamed"


@pytest.mark.asyncio
async def test_delete_subscription(client):
    res = await client.post("/api/subscriptions", json={
        "name": "to-delete",
        "url": "https://example.com/del",
    })
    sub_id = res.json()["id"]
    response = await client.delete(f"/api/subscriptions/{sub_id}")
    assert response.status_code == 204


@pytest.mark.asyncio
async def test_get_nodes_empty(client):
    res = await client.post("/api/subscriptions", json={
        "name": "nodes-test",
        "url": "https://example.com/nodes",
    })
    sub_id = res.json()["id"]
    response = await client.get(f"/api/subscriptions/{sub_id}/nodes")
    assert response.status_code == 200
    assert response.json() == []
