import pytest


@pytest.mark.asyncio
async def test_create_node_group(client):
    response = await client.post("/api/node-groups", json={
        "name": "test-group",
        "group_type": "select",
        "include_entries": [{"type": "regex", "value": "香港"}],
    })
    assert response.status_code == 201
    data = response.json()
    assert data["name"] == "test-group"
    assert data["regex_rules"] == ["香港"]
    assert data["include_entries"] == [{"type": "regex", "value": "香港"}]


@pytest.mark.asyncio
async def test_list_node_groups(client):
    response = await client.get("/api/node-groups")
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_preview_node_groups(client):
    response = await client.get("/api/node-groups/_preview")
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_validate_node_groups(client):
    response = await client.post("/api/node-groups/validate")
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_update_rejects_empty_include_entries_wipe(client):
    created = await client.post(
        "/api/node-groups",
        json={
            "name": "keep-regex",
            "group_type": "select",
            "include_entries": [{"type": "regex", "value": "香港"}],
        },
    )
    assert created.status_code == 201
    group_id = created.json()["id"]

    wiped = await client.patch(
        f"/api/node-groups/{group_id}",
        json={"include_entries": []},
    )
    assert wiped.status_code == 400
    assert "cannot be empty" in wiped.json()["detail"]

    # Still has original matcher
    listed = await client.get("/api/node-groups")
    item = next(g for g in listed.json() if g["id"] == group_id)
    assert item["include_entries"] == [{"type": "regex", "value": "香港"}]
    assert item["regex_rules"] == ["香港"]
