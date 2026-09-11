from __future__ import annotations

import pytest
import pytest_asyncio
import yaml
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

from app.database import Base
from app.models.node_group import NodeGroup
from app.models.node_probe_result import NodeProbeResult
from app.models.subscription import Subscription
from app.services.generate_service import generate_subscription_payload, generate_yaml
from app.services.node_group_service import preview_node_groups
from app.services.probe.service import get_all_db_probe_results
from app.services.subscription_service import collect_all_subscription_nodes
from app.utils.capability_filter import filter_nodes_by_capabilities
from app.utils.group_utils import resolve_group_members
from app.services.node_identity import canonical_node_key


@pytest_asyncio.fixture
async def consumer_db():
    engine = create_async_engine("sqlite+aiosqlite:///:memory:", future=True)
    async with engine.begin() as connection:
        await connection.run_sync(Base.metadata.create_all)
    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    async with session_factory() as session:
        yield session
    await engine.dispose()


def _duplicate_nodes() -> tuple[dict, dict]:
    return (
        {"name": "Shared", "type": "ss", "server": "192.0.2.1", "port": 443},
        {"name": "Shared", "type": "vmess", "server": "192.0.2.2", "port": 443},
    )


def _passing_probe(node: dict) -> dict:
    return {
        "node_key": canonical_node_key(node),
        "status": "ok",
        "speed_mbps": 100,
        "media": {"netflix": {"status": "full"}},
    }


def _failing_probe(node: dict) -> dict:
    return {
        "node_key": canonical_node_key(node),
        "status": "ok",
        "speed_mbps": 1,
        "media": {"netflix": {"status": "originals_only"}},
    }


def test_probe_map_has_no_display_name_alias_and_lookup_is_exact():
    first, second = _duplicate_nodes()
    probe_map = {
        canonical_node_key(first): _passing_probe(first),
        canonical_node_key(second): _failing_probe(second),
    }

    assert set(probe_map) == {canonical_node_key(first), canonical_node_key(second)}
    assert "Shared" not in probe_map


def test_capability_filter_keeps_duplicate_name_nodes_isolated():
    first, second = _duplicate_nodes()
    probe_map = {
        canonical_node_key(first): _passing_probe(first),
        canonical_node_key(second): _failing_probe(second),
    }

    filtered = filter_nodes_by_capabilities(
        [first, second], probe_map, min_speed_mbps=50, required_media=["netflix"]
    )

    assert filtered == [first]


def test_group_resolution_filters_keyed_nodes_before_name_reduction():
    first, second = _duplicate_nodes()
    group = NodeGroup(
        id=1,
        name="Fast",
        filter_min_speed_mbps=50,
        filter_media_unlock=["netflix"],
        include_entries=[{"type": "regex", "value": "Shared"}],
        exclude_nodes=[],
        exclude_group_ids=[],
    )
    probe_map = {
        canonical_node_key(first): _passing_probe(first),
        canonical_node_key(second): _failing_probe(second),
    }

    resolved = resolve_group_members([group], [first, second], probe_map=probe_map)

    assert resolved[1] == ["Shared"]


@pytest.mark.asyncio
async def test_subscription_export_uses_exact_probe_key_for_duplicate_names(consumer_db):
    first, second = _duplicate_nodes()
    consumer_db.add(
        Subscription(
            id=1,
            name="duplicate-sub",
            url="manual://nodes",
            enabled=True,
            raw_nodes=[first, second],
            filter_min_speed_mbps=50,
        )
    )
    consumer_db.add_all(
        [
            NodeProbeResult(
                node_key=canonical_node_key(first),
                name="Shared",
                type="ss",
                server="192.0.2.1",
                port=443,
                status="ok",
                speed_mbps=100,
                media={},
                checked_at=1,
            ),
            NodeProbeResult(
                node_key=canonical_node_key(second),
                name="Shared",
                type="vmess",
                server="192.0.2.2",
                port=443,
                status="ok",
                speed_mbps=1,
                media={},
                checked_at=1,
            ),
        ]
    )
    await consumer_db.commit()

    exported = await generate_subscription_payload(consumer_db, 1)
    payload = yaml.safe_load(exported["yaml"])

    assert payload["proxies"] == [first]


@pytest.mark.asyncio
async def test_aggregate_and_group_preview_do_not_borrow_duplicate_probe_results(consumer_db):
    first, second = _duplicate_nodes()
    consumer_db.add(
        Subscription(
            name="duplicate-sub",
            url="manual://nodes",
            enabled=True,
            raw_nodes=[first, second],
            filter_min_speed_mbps=50,
        )
    )
    consumer_db.add(
        NodeGroup(
            name="Fast",
            group_type="select",
            filter_min_speed_mbps=50,
            include_entries=[{"type": "regex", "value": "Shared"}],
        )
    )
    consumer_db.add_all(
        [
            NodeProbeResult(
                node_key=canonical_node_key(first),
                name="Shared",
                type="ss",
                server="192.0.2.1",
                port=443,
                status="ok",
                speed_mbps=100,
                media={},
                checked_at=1,
            ),
            NodeProbeResult(
                node_key=canonical_node_key(second),
                name="Shared",
                type="vmess",
                server="192.0.2.2",
                port=443,
                status="ok",
                speed_mbps=1,
                media={},
                checked_at=1,
            ),
        ]
    )
    await consumer_db.commit()

    aggregate = await generate_yaml(consumer_db, {"enabled": True, "rules": False, "dns": False})
    aggregate_payload = yaml.safe_load(aggregate["yaml"])
    preview = await preview_node_groups(consumer_db)
    fast = next(item for item in preview if item["name"] == "Fast")

    assert aggregate_payload["proxies"] == [first]
    assert fast["resolved_nodes"] == ["Shared"]


@pytest.mark.asyncio
async def test_subscription_collection_uses_exact_probe_key_for_duplicate_names(consumer_db):
    first, second = _duplicate_nodes()
    consumer_db.add(
        Subscription(
            name="duplicate-sub",
            url="manual://nodes",
            enabled=True,
            raw_nodes=[first, second],
            filter_min_speed_mbps=50,
        )
    )
    consumer_db.add_all(
        [
            NodeProbeResult(
                node_key=canonical_node_key(first),
                name="Shared",
                type="ss",
                server="192.0.2.1",
                port=443,
                status="ok",
                speed_mbps=100,
                media={},
                checked_at=1,
            ),
            NodeProbeResult(
                node_key=canonical_node_key(second),
                name="Shared",
                type="vmess",
                server="192.0.2.2",
                port=443,
                status="ok",
                speed_mbps=1,
                media={},
                checked_at=1,
            ),
        ]
    )
    await consumer_db.commit()

    collected = await collect_all_subscription_nodes(consumer_db)

    assert collected == [first]


@pytest.mark.asyncio
async def test_db_probe_projection_is_canonical_key_only(consumer_db):
    first, _ = _duplicate_nodes()
    consumer_db.add(
        NodeProbeResult(
            node_key=canonical_node_key(first),
            name="Shared",
            type="ss",
            server="192.0.2.1",
            port=443,
            status="ok",
            speed_mbps=100,
            media={},
            checked_at=1,
        )
    )
    await consumer_db.commit()

    result = await get_all_db_probe_results(consumer_db)

    assert set(result) == {canonical_node_key(first)}
    assert result[canonical_node_key(first)]["node_key"] == canonical_node_key(first)
