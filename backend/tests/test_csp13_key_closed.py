from __future__ import annotations

from types import SimpleNamespace

import pytest
from sqlalchemy import delete

from app.database import get_db
from app.models.node_probe_result import NodeProbeResult
from app.models.subscription import Subscription
from app.services.generate_service import generate_subscription_payload
from app.services.probe.service import get_all_db_probe_results
from app.services.subscription_service import _apply_selection
from app.utils.capability_filter import get_probe_result_for_node
from app.utils.group_utils import resolve_group_members


NODES = [
    {"name": "Shared", "type": "ss", "server": "192.0.2.10", "port": 443},
    {"name": "Shared", "type": "vmess", "server": "192.0.2.11", "port": 443},
]


def _group(*, filter_min_speed_mbps=None, filter_media_unlock=None):
    return SimpleNamespace(
        id=1,
        name="qualified",
        include_entries=[
            {"type": "node", "value": NODES[0]["name"]},
            {"type": "node", "value": NODES[1]["name"]},
        ],
        include_nodes=[],
        include_group_ids=[],
        include_group_nodes_ids=[],
        exclude_nodes=[],
        exclude_group_ids=[],
        filter_min_speed_mbps=filter_min_speed_mbps,
        filter_media_unlock=filter_media_unlock or [],
    )


@pytest.mark.asyncio
async def test_probe_map_is_canonical_key_only_and_lookup_is_exact():
    async for db in get_db():
        await db.execute(delete(NodeProbeResult))
        await db.commit()
        db.add_all(
            [
                NodeProbeResult(
                    node_key="Shared|ss|192.0.2.10:443",
                    name="Shared",
                    server="192.0.2.10",
                    port=443,
                    type="ss",
                    status="ok",
                    checked_at=1,
                ),
                NodeProbeResult(
                    node_key="Shared|vmess|192.0.2.11:443",
                    name="Shared",
                    server="192.0.2.11",
                    port=443,
                    type="vmess",
                    status="fail",
                    checked_at=1,
                ),
            ]
        )
        await db.commit()

        probe_map = await get_all_db_probe_results(db)
        assert set(probe_map) == {
            "Shared|ss|192.0.2.10:443",
            "Shared|vmess|192.0.2.11:443",
        }
        assert get_probe_result_for_node(probe_map, NODES[0])["status"] == "ok"
        assert get_probe_result_for_node(probe_map, NODES[1])["status"] == "fail"


def test_capability_filter_never_uses_duplicate_display_name():
    probe_map = {
        "Shared|ss|192.0.2.10:443": {"status": "ok", "speed_mbps": 20},
        "Shared|vmess|192.0.2.11:443": {"status": "ok", "speed_mbps": 2},
    }
    from app.utils.capability_filter import filter_nodes_by_capabilities

    assert filter_nodes_by_capabilities(
        NODES, probe_map, min_speed_mbps=10
    ) == [NODES[0]]


def test_subscription_selection_preserves_distinct_nodes_with_same_name():
    selected = _apply_selection(NODES, [], [], [])
    assert selected == NODES


def test_group_capability_filter_keeps_distinct_keyed_nodes_separate():
    probe_map = {
        "Shared|ss|192.0.2.10:443": {"status": "ok", "speed_mbps": 20},
        "Shared|vmess|192.0.2.11:443": {"status": "ok", "speed_mbps": 2},
    }
    resolved = resolve_group_members(
        [_group(filter_min_speed_mbps=10)], NODES, probe_map=probe_map
    )
    assert resolved[1] == ["Shared"]


@pytest.mark.asyncio
async def test_subscription_export_deduplicates_by_key_not_name():
    async for db in get_db():
        await db.execute(delete(Subscription))
        await db.execute(delete(NodeProbeResult))
        await db.commit()
        subscription = Subscription(
            name="same-name-export",
            url="manual://nodes",
            enabled=True,
            raw_nodes=NODES,
            filter_min_speed_mbps=None,
            filter_media_unlock=[],
        )
        db.add(subscription)
        await db.commit()
        result = await generate_subscription_payload(db, subscription.id, target="clash")
        assert result["yaml"].count("name: Shared") == 2
