from __future__ import annotations

import pytest
from httpx import AsyncClient

from tests.conftest import TestSession
from app.models.node import Node
from app.models.source import Source
from app.repositories.probe_repository import ProbeProfileRepository


@pytest.mark.asyncio
async def test_v2_inventory_endpoints(client: AsyncClient):
    async with TestSession() as session:
        src = Source(
            logical_id="src-v2-001",
            name="测试主订阅",
            kind="subscription",
            url="https://sub.example.com/test",
            enabled=True,
        )
        session.add(src)

        node = Node(
            logical_id="node-v2-001",
            name="日本 01",
            protocol="shadowsocks",
            server="jp.example.com",
            port=8388,
            normalized_payload={"type": "ss", "server": "jp.example.com", "port": 8388, "password": "secret_pass"},
            payload_fingerprint="fp-v2-001",
            lifecycle_state="active",
        )
        session.add(node)
        await session.commit()

    # 测试 v2 节点库存接口
    res_nodes = await client.get("/api/v2/inventory/nodes")
    assert res_nodes.status_code == 200
    data = res_nodes.json()
    assert data["total"] >= 1
    assert any(n["logical_id"] == "node-v2-001" for n in data["items"])

    # 测试 v2 源列表接口
    res_src = await client.get("/api/v2/inventory/sources")
    assert res_src.status_code == 200
    src_data = res_src.json()
    assert any(s["logical_id"] == "src-v2-001" for s in src_data)


@pytest.mark.asyncio
async def test_v2_bundle_preflight_and_compile(client: AsyncClient):
    bundle_payload = {
        "schema_version": 1,
        "logical_id": "b-preflight-001",
        "name": "preflight_test",
        "groups": [
            {
                "logical_id": "g1",
                "name": "香港优选",
                "group_type": "select",
                "include_entries": [{"entry_type": "regex", "value": "香港"}],
            }
        ],
    }

    # 测试预检接口
    res_pf = await client.post("/api/v2/bundle/preflight", json=bundle_payload)
    assert res_pf.status_code == 200
    pf_data = res_pf.json()
    assert pf_data["valid"] is True
    assert pf_data["blockers"] == []

    # 测试编译接口
    res_comp = await client.post("/api/v2/compile", json=bundle_payload)
    assert res_comp.status_code == 200
    comp_data = res_comp.json()
    assert comp_data["success"] is True
    assert "semantic_fingerprint" in comp_data


@pytest.mark.asyncio
async def test_v2_revisions_lifecycle_endpoints(client: AsyncClient):
    bundle_payload = {
        "schema_version": 1,
        "logical_id": "b-rev-api-001",
        "name": "rev_api_test",
        "groups": [{"logical_id": "g1", "name": "默认组"}],
    }

    # 创建版本
    res_create = await client.post("/api/v2/revisions", json=bundle_payload)
    assert res_create.status_code == 200
    rev_data = res_create.json()
    assert rev_data["is_active"] is True
    rev_id = rev_data["logical_id"]

    # 查询版本历史
    res_list = await client.get("/api/v2/revisions")
    assert res_list.status_code == 200
    hist = res_list.json()
    assert any(r["logical_id"] == rev_id for r in hist)

    # 回滚版本
    res_rb = await client.post(f"/api/v2/revisions/{rev_id}/rollback")
    assert res_rb.status_code == 200
    assert res_rb.json()["is_active"] is True


@pytest.mark.asyncio
async def test_v2_probe_and_readiness_endpoints(client: AsyncClient):
    async with TestSession() as session:
        repo = ProbeProfileRepository(session)
        await repo.ensure_default_profile()
        await session.commit()

    # 测试 v2 探测配置列表接口
    res_prof = await client.get("/api/v2/probe/profiles")
    assert res_prof.status_code == 200
    assert len(res_prof.json()) >= 1

    # 测试 v2 探测任务列表接口
    res_jobs = await client.get("/api/v2/probe/jobs")
    assert res_jobs.status_code == 200

    # 测试 v2 系统就绪接口
    res_ready = await client.get("/api/v2/readiness")
    assert res_ready.status_code == 200
    ready_data = res_ready.json()
    assert "healthy" in ready_data
    assert "database" in ready_data
    assert "probe_runner" in ready_data
