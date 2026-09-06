import asyncio
from unittest.mock import AsyncMock, patch

import pytest
from sqlalchemy import select

from app.models.node_probe_result import NodeProbeResult
from app.models.subscription import Subscription
from app.services.probe_settings_service import get_probe_config
from app.services.scheduler import (
    _poll_node_probes,
    _probe_lock,
    collect_scheduler_inventory_nodes,
    get_probe_schedule_runtime,
)
from app.services.subscription_service import collect_all_subscription_nodes
from tests.conftest import TestSession


@pytest.mark.asyncio
async def test_scheduler_inventory_includes_nodes_excluded_from_export():
    """Verify scheduler collects raw_nodes without subscription export capability filtering."""
    async with TestSession() as session:
        # Create Subscription with strict capability filter
        sub = Subscription(
            name="Filtered Sub",
            url="http://example.com/sub",
            enabled=True,
            filter_min_speed_mbps=50.0,
            filter_media_unlock=["netflix"],
            raw_nodes=[
                {"name": "Slow-Node-1", "server": "1.1.1.1", "port": 8388, "type": "ss"},
                {"name": "Fast-Node-2", "server": "2.2.2.2", "port": 8388, "type": "ss"},
            ],
        )
        session.add(sub)

        # Create a second subscription with a duplicate node to verify deduplication
        sub2 = Subscription(
            name="Sub 2",
            url="http://example.com/sub2",
            enabled=True,
            raw_nodes=[
                {"name": "Slow-Node-1", "server": "1.1.1.1", "port": 8388, "type": "ss"},
                {"name": "Node-3", "server": "3.3.3.3", "port": 8388, "type": "ss"},
            ],
        )
        session.add(sub2)
        await session.commit()

        # Insert probe result for Fast-Node-2 that satisfies the filter
        res2 = NodeProbeResult(
            node_key="Fast-Node-2|ss|2.2.2.2:8388",
            name="Fast-Node-2",
            server="2.2.2.2",
            port=8388,
            type="ss",
            status="ok",
            speed_mbps=60.0,
            media={"netflix": {"status": "verified", "verdict": "full", "unlocked": True, "confidence": "verified"}},
            checked_at=1788657000,
        )
        session.add(res2)
        await session.commit()

        # 1. Verify export collector: Slow-Node-1 is excluded because it does not meet min_speed/netflix filter
        export_nodes = await collect_all_subscription_nodes(session)
        export_names = {n.get("name") for n in export_nodes}
        # Slow-Node-1 from sub 1 fails the filter, but sub2 has no filter so sub2 has Slow-Node-1
        # To strictly isolate: let's disable sub2 and check export_nodes
        sub2.enabled = False
        await session.commit()

        filtered_export_nodes = await collect_all_subscription_nodes(session)
        assert len(filtered_export_nodes) == 1
        assert filtered_export_nodes[0]["name"] == "Fast-Node-2"

        # 2. Verify scheduler inventory collector: Slow-Node-1 MUST BE INCLUDED regardless of filter!
        sub2.enabled = True
        await session.commit()

        scheduler_nodes = await collect_scheduler_inventory_nodes(session)
        scheduler_names = [n.get("name") for n in scheduler_nodes]

        # Must include all 3 unique nodes
        assert "Slow-Node-1" in scheduler_names
        assert "Fast-Node-2" in scheduler_names
        assert "Node-3" in scheduler_names
        # Deduplication check: Slow-Node-1 appears exactly once
        assert scheduler_names.count("Slow-Node-1") == 1
        assert len(scheduler_nodes) == 3


@pytest.mark.asyncio
async def test_scheduler_poll_probes_unfiltered_inventory_and_records_state():
    """Verify _poll_node_probes uses unfiltered inventory and updates runtime projection."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()

    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = True
        config.probe_cron_interval_minutes = 10
        await session.commit()

        sub = Subscription(
            name="Probed Sub",
            url="http://example.com/sub",
            enabled=True,
            filter_min_speed_mbps=100.0,
            raw_nodes=[
                {"name": "Raw-Node-A", "server": "10.0.0.1", "port": 443, "type": "vless"},
            ],
        )
        session.add(sub)
        await session.commit()

    # Step 1: Initial tick establishes baseline
    runtime._baseline_at = 0.0
    now = 1788657000.0
    with patch("time.time", return_value=now), \
         patch("app.services.scheduler.AsyncSessionLocal", return_value=TestSession()):
        await _poll_node_probes()

    assert runtime.state == "waiting"
    assert runtime._baseline_at == now

    # Step 2: Tick after 11 minutes triggers probe with Raw-Node-A
    batch_mock = AsyncMock(return_value={
        "results": [{"node_key": "Raw-Node-A", "status": "ok"}],
        "summary": {"total": 1, "ok": 1, "fail": 0, "timeout": 0, "skipped": 0},
    })

    with patch("time.time", return_value=now + 660), \
         patch("app.services.scheduler.AsyncSessionLocal", return_value=TestSession()), \
         patch("app.services.probe.service.probe_batch_nodes", side_effect=batch_mock):
        await _poll_node_probes()

    assert batch_mock.call_count == 1
    # Check that Raw-Node-A was passed to probe_batch_nodes
    called_nodes = batch_mock.call_args[0][0]
    assert len(called_nodes) == 1
    assert called_nodes[0]["name"] == "Raw-Node-A"

    # Runtime projection state updated
    assert runtime.state == "waiting"
    assert runtime.last_started_at == int(now + 660)
    assert runtime.last_summary == {"total": 1, "ok": 1, "fail": 0, "timeout": 0, "skipped": 0}


@pytest.mark.asyncio
async def test_scheduler_concurrent_tick_mutex():
    """Verify concurrent tick calls are mutually exclusive under _probe_lock."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()
    now = 1788657000.0
    runtime.record_baseline(now)
    import app.services.scheduler as sched_mod
    sched_mod._last_probe_run_at = now

    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = True
        config.probe_cron_interval_minutes = 5
        await session.commit()

        sub = Subscription(
            name="Mutex Sub",
            url="http://example.com/sub",
            enabled=True,
            raw_nodes=[{"name": "Node-M", "server": "10.0.0.1", "port": 443, "type": "ss"}],
        )
        session.add(sub)
        await session.commit()

    run_started_event = asyncio.Event()
    release_run_event = asyncio.Event()

    async def slow_probe_batch(*args, **kwargs):
        run_started_event.set()
        await release_run_event.wait()
        return {
            "results": [{"node_key": "Node-M", "status": "ok"}],
            "summary": {"total": 1, "ok": 1, "fail": 0, "timeout": 0, "skipped": 0},
        }

    with patch("time.time", return_value=now + 400), \
         patch("app.services.scheduler.AsyncSessionLocal", return_value=TestSession()), \
         patch("app.services.probe.service.probe_batch_nodes", side_effect=slow_probe_batch):

        # Launch first tick
        task1 = asyncio.create_task(_poll_node_probes())
        await run_started_event.wait()

        # While task1 is running, launch a second tick
        # Second tick should detect _probe_lock.locked() and return immediately without running
        task2 = asyncio.create_task(_poll_node_probes())
        await task2  # Must finish immediately

        # Release task1
        release_run_event.set()
        await task1

    assert runtime.state == "waiting"
    assert runtime.last_summary == {"total": 1, "ok": 1, "fail": 0, "timeout": 0, "skipped": 0}
