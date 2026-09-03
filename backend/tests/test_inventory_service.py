from __future__ import annotations

import pytest
import pytest_asyncio
from sqlalchemy.ext.asyncio import AsyncSession

from tests.conftest import TestSession
from app.repositories.node_repository import NodeRepository
from app.repositories.source_repository import SourceRepository
from app.services.inventory_service import get_inventory_node_detail, list_inventory_nodes


@pytest_asyncio.fixture
async def async_session():
    async with TestSession() as session:
        yield session


@pytest.mark.asyncio
async def test_inventory_listing_and_filtering(async_session: AsyncSession):
    src = await SourceRepository.create_source(
        async_session, name="主要订阅源", kind="subscription", url="https://secret-link.com?key=supersecret"
    )
    rev = await SourceRepository.create_revision(async_session, src.id, "active", "hash1", 2)

    node1, _ = await NodeRepository.upsert_by_fingerprint(
        session=async_session,
        name="香港 01",
        protocol="ss",
        server="hk.node.net",
        port=443,
        normalized_payload={"cipher": "aes-128-gcm", "password": "mypassword123"},
        payload_fingerprint="fp1",
    )
    node2, _ = await NodeRepository.upsert_by_fingerprint(
        session=async_session,
        name="日本 01",
        protocol="vless",
        server="jp.node.net",
        port=8443,
        normalized_payload={"uuid": "1111-2222-3333", "flow": "xtls-rprx-vision"},
        payload_fingerprint="fp2",
    )

    await NodeRepository.link_nodes_to_revision(
        async_session, rev.id, [(node1.id, "HK 01", 0), (node2.id, "JP 01", 1)]
    )
    await async_session.commit()

    # 1 全量列表
    resp_all = await list_inventory_nodes(async_session, page=1, page_size=10)
    assert resp_all.total == 2
    assert len(resp_all.items) == 2

    # 验证安全来源追踪（不含 URL 密码）
    first_item = resp_all.items[0]
    assert len(first_item.sources) == 1
    assert first_item.sources[0].source_name == "主要订阅源"
    assert not hasattr(first_item.sources[0], "url")

    # 2 按协议过滤
    resp_ss = await list_inventory_nodes(async_session, protocol="ss")
    assert resp_ss.total == 1
    assert resp_ss.items[0].protocol == "ss"

    # 3 按关键字过滤
    resp_kw = await list_inventory_nodes(async_session, keyword="日本")
    assert resp_kw.total == 1
    assert resp_kw.items[0].name == "日本 01"


@pytest.mark.asyncio
async def test_inventory_node_detail_secret_sanitization(async_session: AsyncSession):
    node, _ = await NodeRepository.upsert_by_fingerprint(
        session=async_session,
        name="保密节点",
        protocol="trojan",
        server="secret.node.net",
        port=443,
        normalized_payload={
            "password": "plain_text_trojan_password",
            "sni": "secret.node.net",
            "token": "sensitive_auth_token",
        },
        payload_fingerprint="fp_secret",
    )
    await async_session.commit()

    detail = await get_inventory_node_detail(async_session, node.logical_id)
    assert detail is not None
    assert detail.name == "保密节点"
    # 验证敏感字段全部脱敏
    assert detail.sanitized_payload["password"] == "***"
    assert detail.sanitized_payload["token"] == "***"
    assert detail.sanitized_payload["sni"] == "secret.node.net"
