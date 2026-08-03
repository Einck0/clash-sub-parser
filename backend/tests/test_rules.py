import pytest


@pytest.mark.asyncio
async def test_create_rule(client):
    response = await client.post("/api/rules", json={
        "type": "DOMAIN-SUFFIX",
        "value": "google.com",
        "proxy": "DIRECT",
        "category": "default",
    })
    assert response.status_code == 201
    data = response.json()
    assert data["type"] == "DOMAIN-SUFFIX"
    assert data["value"] == "google.com"


@pytest.mark.asyncio
async def test_list_rules(client):
    await client.post("/api/rules", json={
        "type": "DOMAIN",
        "value": "test.com",
        "proxy": "DIRECT",
    })
    response = await client.get("/api/rules")
    assert response.status_code == 200
    assert len(response.json()) >= 1


@pytest.mark.asyncio
async def test_reorder_rules_persists_order(client):
    first = await client.post(
        "/api/rules",
        json={"type": "DOMAIN", "value": "first.example", "proxy": "DIRECT", "category": "default"},
    )
    second = await client.post(
        "/api/rules",
        json={"type": "DOMAIN", "value": "second.example", "proxy": "DIRECT", "category": "default"},
    )
    assert first.status_code == 201
    assert second.status_code == 201

    reordered = await client.post(
        "/api/rules/reorder",
        json={
            "items": [
                {"id": second.json()["id"], "sort_order": 0},
                {"id": first.json()["id"], "sort_order": 1},
            ]
        },
    )
    assert reordered.status_code == 200
    selected = [item for item in reordered.json() if item["id"] in {first.json()["id"], second.json()["id"]}]
    assert [item["id"] for item in selected] == [second.json()["id"], first.json()["id"]]


@pytest.mark.asyncio
async def test_batch_rules(client):
    response = await client.post("/api/rules/batch", json={
        "delete": [],
        "create": [
            {"type": "DOMAIN-SUFFIX", "value": "a.com", "proxy": "DIRECT"},
            {"type": "DOMAIN-SUFFIX", "value": "b.com", "proxy": "REJECT"},
        ],
        "update": [],
        "reorder": [],
    })
    assert response.status_code == 200
    rules = response.json()
    assert len(rules) >= 2


@pytest.mark.asyncio
async def test_delete_rule(client):
    res = await client.post("/api/rules", json={
        "type": "DOMAIN",
        "value": "delete-me.com",
        "proxy": "REJECT",
    })
    rule_id = res.json()["id"]
    response = await client.delete(f"/api/rules/{rule_id}")
    assert response.status_code == 204


@pytest.mark.asyncio
async def test_delete_nonexistent_rule(client):
    """Idempotent delete - should return 204 even if rule doesn't exist."""
    response = await client.delete("/api/rules/99999")
    assert response.status_code == 204
