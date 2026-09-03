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
async def test_reorder_node_groups_persists_order(client):
    first = await client.post(
        "/api/node-groups",
        json={"name": "first", "group_type": "select", "include_entries": [{"type": "node", "value": "a"}]},
    )
    second = await client.post(
        "/api/node-groups",
        json={"name": "second", "group_type": "select", "include_entries": [{"type": "node", "value": "b"}]},
    )
    assert first.status_code == 201
    assert second.status_code == 201

    reordered = await client.post(
        "/api/node-groups/reorder",
        json={
            "items": [
                {"id": second.json()["id"], "sort_order": 0},
                {"id": first.json()["id"], "sort_order": 1},
            ]
        },
    )
    assert reordered.status_code == 200
    assert [item["name"] for item in reordered.json() if item["name"] in {"first", "second"}] == ["second", "first"]

    listed = await client.get("/api/node-groups")
    assert [item["name"] for item in listed.json() if item["name"] in {"first", "second"}] == ["second", "first"]


@pytest.mark.asyncio
async def test_reorder_node_groups_rejects_unknown_id(client):
    created = await client.post(
        "/api/node-groups",
        json={"name": "known", "group_type": "select", "include_entries": [{"type": "node", "value": "a"}]},
    )
    assert created.status_code == 201

    response = await client.post(
        "/api/node-groups/reorder",
        json={"items": [{"id": created.json()["id"], "sort_order": 0}, {"id": 99999, "sort_order": 1}]},
    )
    assert response.status_code == 400


@pytest.mark.asyncio
async def test_reorder_node_groups_rejects_duplicate_order(client):
    first = await client.post(
        "/api/node-groups",
        json={"name": "first-order", "group_type": "select", "include_entries": [{"type": "node", "value": "a"}]},
    )
    second = await client.post(
        "/api/node-groups",
        json={"name": "second-order", "group_type": "select", "include_entries": [{"type": "node", "value": "b"}]},
    )
    assert first.status_code == 201
    assert second.status_code == 201

    response = await client.post(
        "/api/node-groups/reorder",
        json={
            "items": [
                {"id": first.json()["id"], "sort_order": 0},
                {"id": second.json()["id"], "sort_order": 0},
            ]
        },
    )
    assert response.status_code == 400


@pytest.mark.asyncio
async def test_reorder_node_groups_rejects_duplicate_id(client):
    created = await client.post(
        "/api/node-groups",
        json={"name": "duplicate-id", "group_type": "select", "include_entries": [{"type": "node", "value": "a"}]},
    )
    assert created.status_code == 201
    group_id = created.json()["id"]

    response = await client.post(
        "/api/node-groups/reorder",
        json={"items": [{"id": group_id, "sort_order": 0}, {"id": group_id, "sort_order": 1}]},
    )
    assert response.status_code == 400


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


@pytest.mark.asyncio
async def test_exclude_group_cycle_rejected(client):
    a = await client.post(
        "/api/node-groups",
        json={
            "name": "group-a",
            "group_type": "select",
            "include_entries": [{"type": "node", "value": "n1"}],
        },
    )
    b = await client.post(
        "/api/node-groups",
        json={
            "name": "group-b",
            "group_type": "select",
            "include_entries": [{"type": "node", "value": "n2"}],
        },
    )
    assert a.status_code == 201
    assert b.status_code == 201
    a_id = a.json()["id"]
    b_id = b.json()["id"]

    # A 包含 B 且 B 排除 A 构成循环引用
    ok = await client.patch(
        f"/api/node-groups/{a_id}",
        json={
            "include_entries": [{"type": "group_nodes", "value": b_id}],
        },
    )
    assert ok.status_code == 200

    cycle = await client.patch(
        f"/api/node-groups/{b_id}",
        json={
            "include_entries": [
                {"type": "node", "value": "n2"},
                {"type": "exclude_group_nodes", "value": a_id},
            ],
        },
    )
    assert cycle.status_code == 400
    detail = str(cycle.json().get("detail") or "")
    assert "循环" in detail or "Circular" in detail


@pytest.mark.asyncio
async def test_exclude_group_nodes_entry_resolves(client):
    base = await client.post(
        "/api/node-groups",
        json={
            "name": "cheap",
            "group_type": "select",
            "include_entries": [{"type": "node", "value": "cheap-1"}],
        },
    )
    region = await client.post(
        "/api/node-groups",
        json={
            "name": "usa",
            "group_type": "select",
            "include_entries": [
                {"type": "node", "value": "us-1"},
                {"type": "node", "value": "cheap-1"},
                {"type": "exclude_group_nodes", "value": base.json()["id"]},
            ],
        },
    )
    assert base.status_code == 201
    assert region.status_code == 201
    data = region.json()
    assert data["exclude_group_ids"] == [base.json()["id"]]
    assert any(
        e.get("type") == "exclude_group_nodes" and e.get("value") == base.json()["id"]
        for e in data["include_entries"]
    )

    preview = await client.get("/api/node-groups/_preview")
    assert preview.status_code == 200
    item = next(g for g in preview.json() if g["id"] == data["id"])
    assert "us-1" in item["resolved_nodes"]
    assert "cheap-1" not in item["resolved_nodes"]

