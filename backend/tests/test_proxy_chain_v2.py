import pytest
import yaml


async def _seed_nodes(client):
    created = await client.post(
        "/api/subscriptions",
        json={
            "name": "chain-sub",
            "url": "https://example.com/sub",
            "is_primary": True,
            "node_prefix": "",
            "manual_nodes": [
                {"name": "香港入口", "type": "ss", "server": "1.1.1.1", "port": 1},
                {"name": "美国落地", "type": "vmess", "server": "2.2.2.2", "port": 2},
                {"name": "日本落地", "type": "ss", "server": "3.3.3.3", "port": 3},
            ],
        },
    )
    assert created.status_code == 201, created.text
    sub = created.json()
    names = [n.get("name") for n in (sub.get("raw_nodes") or [])]
    assert "美国落地" in names, sub

    group = await client.post(
        "/api/node-groups",
        json={
            "name": "美国组",
            "kind": "manual",
            "group_type": "select",
            "include_entries": [
                {"type": "node", "value": "美国落地"},
                {"type": "node", "value": "日本落地"},
            ],
        },
    )
    assert group.status_code == 201, group.text
    return sub, group.json()


@pytest.mark.asyncio
async def test_proxy_chain_node_binding_and_priority(client):
    sub, group = await _seed_nodes(client)
    entry = "香港入口"
    exit_us = "美国落地"
    exit_jp = "日本落地"

    created = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "node",
            "target_name": exit_us,
            "dialer_type": "node",
            "dialer_ref": entry,
            "enabled": True,
        },
    )
    assert created.status_code == 201, created.text

    sub_bind = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "subscription",
            "target_id": sub["id"],
            "dialer_type": "node",
            "dialer_ref": entry,
            "enabled": True,
        },
    )
    assert sub_bind.status_code == 201, sub_bind.text

    # Japan uses a different entry node (still a node dialer); group dialer that
    # contains the target itself is a cycle and is covered by a separate test.
    jp = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "node",
            "target_name": exit_jp,
            "dialer_type": "node",
            "dialer_ref": entry,
            "enabled": True,
        },
    )
    assert jp.status_code == 201, jp.text

    gen = await client.post(
        "/api/generate/yaml",
        json={
            "enabled": True,
            "subscriptions": True,
            "node_groups": True,
            "rules": False,
            "dns": False,
        },
    )
    assert gen.status_code == 200, gen.text
    data = yaml.safe_load(gen.json()["yaml"])
    proxies = {p["name"]: p for p in data.get("proxies") or []}
    assert proxies[exit_us].get("dialer-proxy") == entry
    assert proxies[exit_jp].get("dialer-proxy") == entry
    # entry may be in subscription target set, but must not self-dialer
    assert proxies.get(entry, {}).get("dialer-proxy") != entry


@pytest.mark.asyncio
async def test_proxy_chain_group_target_expands_leaves(client):
    sub, group = await _seed_nodes(client)
    entry = "香港入口"
    created = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "node_group",
            "target_id": group["id"],
            "dialer_type": "node",
            "dialer_ref": entry,
            "enabled": True,
        },
    )
    assert created.status_code == 201, created.text

    gen = await client.post(
        "/api/generate/yaml",
        json={
            "enabled": True,
            "subscriptions": True,
            "node_groups": False,
            "rules": False,
            "dns": False,
        },
    )
    assert gen.status_code == 200, gen.text
    data = yaml.safe_load(gen.json()["yaml"])
    proxies = {p["name"]: p for p in data.get("proxies") or []}
    assert proxies["美国落地"].get("dialer-proxy") == entry
    assert proxies["日本落地"].get("dialer-proxy") == entry
    assert "dialer-proxy" not in proxies["香港入口"]


@pytest.mark.asyncio
async def test_proxy_chain_rejects_bad_dialer(client):
    await _seed_nodes(client)
    bad = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "node",
            "target_name": "美国落地",
            "dialer_type": "node",
            "dialer_ref": "不存在的节点",
            "enabled": True,
        },
    )
    assert bad.status_code == 400

    self_ref = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "node",
            "target_name": "香港入口",
            "dialer_type": "node",
            "dialer_ref": "香港入口",
            "enabled": True,
        },
    )
    assert self_ref.status_code == 400


@pytest.mark.asyncio
async def test_final_nodes_endpoint(client):
    await _seed_nodes(client)
    res = await client.get("/api/proxy-chains/meta/final-nodes")
    assert res.status_code == 200
    names = {x["name"] for x in res.json()}
    assert "香港入口" in names
    assert "美国落地" in names


@pytest.mark.asyncio
async def test_proxy_chain_crud_list_delete(client):
    sub, _ = await _seed_nodes(client)
    created = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "subscription",
            "target_id": sub["id"],
            "dialer_type": "node",
            "dialer_ref": "香港入口",
        },
    )
    assert created.status_code == 201
    bid = created.json()["id"]
    listed = await client.get("/api/proxy-chains")
    assert listed.status_code == 200
    assert any(x["id"] == bid for x in listed.json())
    patched = await client.patch(f"/api/proxy-chains/{bid}", json={"enabled": False})
    assert patched.status_code == 200
    assert patched.json()["enabled"] is False
    deleted = await client.delete(f"/api/proxy-chains/{bid}")
    assert deleted.status_code == 204


@pytest.mark.asyncio
async def test_proxy_chain_rejects_group_membership_cycle(client):
    """subscription/group targets cannot dialer a group that contains them."""
    sub, group = await _seed_nodes(client)
    # group "美国组" contains 美国落地/日本落地
    # binding whole subscription to dialer group 美国组 => membership cycle
    bad = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "subscription",
            "target_id": sub["id"],
            "dialer_type": "node_group",
            "dialer_ref": group["name"],
            "enabled": True,
        },
    )
    assert bad.status_code == 400, bad.text
    assert "环" in bad.json()["detail"]

    # same group dialer to itself
    bad2 = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "node_group",
            "target_id": group["id"],
            "dialer_type": "node_group",
            "dialer_ref": group["name"],
            "enabled": True,
        },
    )
    assert bad2.status_code == 400, bad2.text
