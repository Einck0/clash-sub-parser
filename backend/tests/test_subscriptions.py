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
async def test_failed_fetch_updates_last_fetched_at(monkeypatch):
    from datetime import datetime

    from app.database import AsyncSessionLocal
    from app.schemas.subscription import SubscriptionCreate
    from app.services import subscription_service as svc

    async def boom(_client, _url):
        raise RuntimeError("upstream down")

    monkeypatch.setattr(svc, "_fetch_subscription_text", boom)

    async with AsyncSessionLocal() as db:
        item = await svc.create_subscription(
            db,
            SubscriptionCreate(
                name="fail-once",
                url="https://example.com/fail",
                update_interval=60,
            ),
        )
        sub_id = item.id

    async with AsyncSessionLocal() as db:
        item = await svc.get_subscription(db, sub_id)
        assert item is not None
        with pytest.raises(RuntimeError):
            await svc.fetch_subscription_nodes(db, item)

    async with AsyncSessionLocal() as db:
        item = await svc.get_subscription(db, sub_id)
        assert item is not None
        assert item.last_fetch_error == "upstream down"
        assert item.fetch_failed_count == 1
        assert isinstance(item.last_fetched_at, datetime)


@pytest.mark.asyncio
async def test_delete_while_fetch_is_stale_is_idempotent(monkeypatch):
    from fastapi import HTTPException

    from app.database import AsyncSessionLocal
    from app.schemas.subscription import SubscriptionCreate
    from app.services import subscription_service as svc

    async def fetch_then_other_session_deletes(client, url):
        # Simulate another request deleting the subscription mid-fetch.
        async with AsyncSessionLocal() as delete_db:
            doomed = await svc.get_subscription(delete_db, sub_id)
            assert doomed is not None
            await svc.delete_subscription(delete_db, doomed)

        class Dummy:
            headers = {}

        return Dummy(), "proxies: []\n"

    async with AsyncSessionLocal() as db:
        item = await svc.create_subscription(
            db,
            SubscriptionCreate(
                name="race-delete",
                url="https://example.com/race",
                update_interval=60,
            ),
        )
        sub_id = item.id

    monkeypatch.setattr(svc, "_fetch_subscription_text", fetch_then_other_session_deletes)

    async with AsyncSessionLocal() as fetch_db:
        live = await svc.get_subscription(fetch_db, sub_id)
        assert live is not None
        with pytest.raises(HTTPException) as exc:
            await svc.fetch_subscription_nodes(fetch_db, live)
        assert exc.value.status_code == 404

    async with AsyncSessionLocal() as db:
        assert await svc.get_subscription(db, sub_id) is None


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
