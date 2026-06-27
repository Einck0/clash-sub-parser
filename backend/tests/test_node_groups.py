import pytest


@pytest.mark.asyncio
async def test_create_node_group(client):
    response = await client.post("/api/node-groups", json={
        "name": "test-group",
        "group_type": "select",
        "regex_rules": ["香港"],
    })
    assert response.status_code == 201
    data = response.json()
    assert data["name"] == "test-group"


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
