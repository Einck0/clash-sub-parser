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
async def test_proxy_chain_rejects_full_group_membership_cycle(client):
    """Reject only when every target is inside the dialer group."""
    sub, group = await _seed_nodes(client)
    # 美国组 only has 美国落地/日本落地. Binding that group to itself is full loop.
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
    assert "环" in bad2.json()["detail"]

    # Single node that is a member of 美国组 dialer to 美国组 -> full loop for that target
    bad_node = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "node",
            "target_name": "美国落地",
            "dialer_type": "node_group",
            "dialer_ref": group["name"],
            "enabled": True,
        },
    )
    assert bad_node.status_code == 400, bad_node.text


@pytest.mark.asyncio
async def test_proxy_chain_partial_overlap_allowed_and_skips_members(client):
    """Target set may partially overlap dialer group; only non-members get dialer."""
    sub = await client.post(
        "/api/subscriptions",
        json={
            "name": "mix-sub",
            "url": "https://example.com/mix",
            "is_primary": True,
            "node_prefix": "",
            "manual_nodes": [
                {"name": "入口A", "type": "ss", "server": "1.1.1.1", "port": 1},
                {"name": "落地B", "type": "vmess", "server": "2.2.2.2", "port": 2},
            ],
        },
    )
    assert sub.status_code == 201, sub.text
    sub_id = sub.json()["id"]

    entry_group = await client.post(
        "/api/node-groups",
        json={
            "name": "入口组",
            "kind": "manual",
            "group_type": "select",
            "include_entries": [{"type": "node", "value": "入口A"}],
        },
    )
    assert entry_group.status_code == 201, entry_group.text

    # Whole subscription -> 入口组: 入口A overlaps (skip), 落地B safe (chain)
    ok = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "subscription",
            "target_id": sub_id,
            "dialer_type": "node_group",
            "dialer_ref": "入口组",
            "enabled": True,
        },
    )
    assert ok.status_code == 201, ok.text

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
    assert "dialer-proxy" not in proxies["入口A"]
    assert proxies["落地B"].get("dialer-proxy") == "入口组"


@pytest.mark.asyncio
async def test_proxy_chain_preview_and_node_ledger(client):
    sub, group = await _seed_nodes(client)

    created = await client.post(
        "/api/proxy-chains",
        json={
            "target_type": "node",
            "target_name": "美国落地",
            "dialer_type": "node",
            "dialer_ref": "香港入口",
            "enabled": True,
        },
    )
    assert created.status_code == 201, created.text

    preview = await client.post(
        "/api/proxy-chains/meta/preview",
        json={
            "target_type": "subscription",
            "target_id": sub["id"],
            "dialer_type": "node",
            "dialer_ref": "香港入口",
        },
    )
    assert preview.status_code == 200, preview.text
    body = preview.json()
    assert body["target_count"] >= 3
    assert body["chain_count"] >= 2
    assert body["skip_count"] >= 1
    assert "香港入口" in body["skip_samples"]

    ledger = await client.get("/api/proxy-chains/meta/node-ledger")
    assert ledger.status_code == 200, ledger.text
    rows = {row["name"]: row for row in ledger.json()}
    assert "美国落地" in rows
    assert rows["美国落地"]["dialer_proxy"] == "香港入口"
    assert rows["美国落地"]["chain_source"] == "node"
    assert rows["美国落地"]["subscription_name"] == "chain-sub"
    assert rows["美国落地"].get("type") == "vmess"
    assert rows["美国落地"].get("server") == "2.2.2.2"
    assert rows["美国落地"].get("port") == 2
    assert "美国组" in (rows["美国落地"].get("group_names") or [])
    assert rows["香港入口"].get("dialer_proxy") in (None, "")

    finals = await client.get("/api/proxy-chains/meta/final-nodes")
    assert finals.status_code == 200, finals.text
    names = {row["name"] for row in finals.json()}
    assert {"香港入口", "美国落地", "日本落地"} <= names
    us = next(row for row in finals.json() if row["name"] == "美国落地")
    assert us.get("type") == "vmess"
    assert us.get("port") == 2
