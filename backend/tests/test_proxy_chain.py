import pytest
import yaml

from app.utils.proxy_chain import (
    apply_subscription_chains,
    dialer_hop_from_chain,
    effective_chain,
    normalize_chain,
    normalize_node_proxy_chains,
)


def test_normalize_chain():
    assert normalize_chain([" 香港 ", "", "香港", "日本"]) == ["香港", "日本"]
    assert normalize_chain(None) == []
    assert normalize_chain("x") == []


def test_effective_chain_priority():
    sub = ["入口A"]
    overrides = {"落地B": ["入口B"], "直连C": []}
    assert effective_chain("落地A", subscription_chain=sub, node_chains=overrides) == ["入口A"]
    assert effective_chain("落地B", subscription_chain=sub, node_chains=overrides) == ["入口B"]
    assert effective_chain("直连C", subscription_chain=sub, node_chains=overrides) == []


def test_dialer_hop_last_only():
    assert dialer_hop_from_chain(["A", "B"]) == "B"
    assert dialer_hop_from_chain([]) is None


def test_apply_subscription_chains_p0():
    nodes = [
        {"name": "入口A", "type": "ss", "server": "1.1.1.1", "port": 1},
        {"name": "落地1", "type": "vmess", "server": "2.2.2.2", "port": 2},
        {"name": "落地2", "type": "vmess", "server": "3.3.3.3", "port": 3},
        {"name": "直连观察", "type": "ss", "server": "4.4.4.4", "port": 4},
    ]
    out = apply_subscription_chains(
        nodes,
        subscription_chain=["入口A"],
        node_chains={"直连观察": [], "落地2": ["其它入口"]},
    )
    by_name = {n["name"]: n for n in out}
    assert "dialer-proxy" not in by_name["入口A"]  # self-ref stripped
    assert by_name["落地1"]["dialer-proxy"] == "入口A"
    assert by_name["落地2"]["dialer-proxy"] == "其它入口"
    assert "dialer-proxy" not in by_name["直连观察"]


def test_normalize_node_proxy_chains_omits_null():
    out = normalize_node_proxy_chains({"a": ["x"], "b": None, "": ["y"]})
    assert out == {"a": ["x"]}


@pytest.mark.asyncio
async def test_subscription_proxy_chain_roundtrip(client):
    created = await client.post(
        "/api/subscriptions",
        json={
            "name": "chain-sub",
            "url": "https://example.com/chain",
            "proxy_chain": [" 香港 ", "香港"],
            "node_proxy_chains": {"美区-1": ["日本"], "观察": []},
        },
    )
    assert created.status_code == 201
    data = created.json()
    assert data["proxy_chain"] == ["香港"]
    assert data["node_proxy_chains"] == {"美区-1": ["日本"], "观察": []}

    patched = await client.patch(
        f"/api/subscriptions/{data['id']}",
        json={"proxy_chain": ["入口组"], "node_proxy_chains": {}},
    )
    assert patched.status_code == 200
    assert patched.json()["proxy_chain"] == ["入口组"]
    assert patched.json()["node_proxy_chains"] == {}


@pytest.mark.asyncio
async def test_generate_yaml_applies_dialer_proxy(client, monkeypatch):
    from app.database import AsyncSessionLocal
    from app.models.subscription import Subscription
    from app.services import generate_service as gen

    async with AsyncSessionLocal() as db:
        entry = Subscription(
            name="entry-sub",
            url="https://example.com/entry",
            enabled=True,
            raw_nodes=[{"name": "香港入口", "type": "ss", "server": "1.1.1.1", "port": 443}],
            proxy_chain=[],
            node_proxy_chains={},
        )
        exit_sub = Subscription(
            name="exit-sub",
            url="https://example.com/exit",
            enabled=True,
            raw_nodes=[
                {"name": "美国落地", "type": "vmess", "server": "2.2.2.2", "port": 443},
                {"name": "无链节点", "type": "ss", "server": "3.3.3.3", "port": 80},
            ],
            proxy_chain=["香港入口"],
            node_proxy_chains={"无链节点": []},
        )
        db.add(entry)
        db.add(exit_sub)
        await db.commit()

    async with AsyncSessionLocal() as db:
        result = await gen.generate_yaml(db, {"enabled": True, "subscriptions": True, "node_groups": False, "rules": False, "dns": False})
    data = yaml.safe_load(result["yaml"])
    proxies = {p["name"]: p for p in data.get("proxies") or []}
    assert proxies["美国落地"]["dialer-proxy"] == "香港入口"
    assert "dialer-proxy" not in proxies["无链节点"]
    assert "dialer-proxy" not in proxies["香港入口"]
