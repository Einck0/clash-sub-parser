from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock, patch
import pytest
import pytest_asyncio
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

from app.database import Base
from app.models.probe_config import ProbeConfig
from app.schemas.probe import ResolvedProbeConfig
from app.services.probe.providers import check_download_speed, check_geo_identity, check_media_unlock, check_transport
from app.services.probe.runner import spawn_node_runner
from app.services.probe.service import probe_batch_nodes, probe_single_node
from app.services.probe_settings_service import get_probe_config, resolve_probe_config
from app.services.scheduler import _poll_node_probes


@pytest_asyncio.fixture()
async def isolated_db():
    engine = create_async_engine("sqlite+aiosqlite:///:memory:", future=True)
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    maker = async_sessionmaker(engine, expire_on_commit=False)
    async with maker() as session:
        yield session
    await engine.dispose()


def test_resolve_probe_config_snapshot_rules():
    saved = ProbeConfig(
        id=1,
        probe_enabled=True,
        probe_concurrency=10,
        probe_service_timeout_ms=2000,
        probe_timeout_ms=3000,
        probe_cron_enabled=True,
        probe_cron_interval_minutes=60,
    )

    # 1. Default resolution
    resolved = resolve_probe_config(saved)
    assert resolved.concurrency == 10
    assert resolved.service_timeout_s == 2.0
    assert resolved.node_timeout_s == 3.0
    assert resolved.probe_cron_enabled is True
    assert resolved.probe_cron_interval_minutes == 60

    # 2. Client override lowers concurrency
    lower = resolve_probe_config(saved, client_concurrency=4)
    assert lower.concurrency == 4

    # 3. Client override cannot exceed persisted concurrency
    higher = resolve_probe_config(saved, client_concurrency=18)
    assert higher.concurrency == 10, "Cannot exceed persisted concurrency"

    # 4. Client override lower than 1 is clamped to 1
    zero = resolve_probe_config(saved, client_concurrency=0)
    assert zero.concurrency == 1

    # 5. Client timeout_ms override
    override_zero = resolve_probe_config(saved, client_timeout_ms=0)
    assert override_zero.node_timeout_s is None

    override_pos = resolve_probe_config(saved, client_timeout_ms=5000)
    assert override_pos.node_timeout_s == 5.0


@pytest.mark.asyncio
async def test_batch_concurrency_bounded_by_semaphore():
    # Test that batch runner enforces maximum active workers
    current_active = 0
    max_active_observed = 0
    lock = asyncio.Lock()

    async def mock_probe_single(*args, **kwargs):
        nonlocal current_active, max_active_observed
        async with lock:
            current_active += 1
            if current_active > max_active_observed:
                max_active_observed = current_active
        await asyncio.sleep(0.05)
        async with lock:
            current_active -= 1
        return {"name": "test", "status": "ok"}

    nodes = [{"name": f"n{i}", "server": f"1.1.1.{i}", "port": 443, "type": "ss"} for i in range(15)]
    resolved = ResolvedProbeConfig(concurrency=3)

    with patch("app.services.probe.service.probe_single_node", side_effect=mock_probe_single):
        result = await probe_batch_nodes(nodes, resolved_config=resolved)

    assert result["summary"]["total"] == 15
    assert max_active_observed == 3, f"Expected max 3 concurrent workers, observed {max_active_observed}"


@pytest.mark.asyncio
async def test_batch_snapshot_immune_to_mid_batch_settings_change(isolated_db):
    config = await get_probe_config(isolated_db)
    config.probe_concurrency = 8
    await isolated_db.commit()

    resolved_snapshot = resolve_probe_config(config)
    assert resolved_snapshot.concurrency == 8

    # Simulate administrator PATCHing concurrency to 2 mid-batch
    config.probe_concurrency = 2
    await isolated_db.commit()

    # In-flight batch retains frozen snapshot of 8
    assert resolved_snapshot.concurrency == 8

    # Next batch gets new setting of 2
    next_config = await get_probe_config(isolated_db)
    next_resolved = resolve_probe_config(next_config)
    assert next_resolved.concurrency == 2


@pytest.mark.asyncio
async def test_runner_startup_timeout_produces_timeout_status():
    node = {"name": "slow-node", "server": "127.0.0.1", "port": 1234, "type": "ss", "password": "pass", "cipher": "aes-128-gcm"}

    async def mock_wait_for_port_ready(port, max_wait_s=2.0, step_s=0.05):
        await asyncio.sleep(max_wait_s + 0.1)
        return False

    with patch("app.services.probe.runner.find_singbox_binary", return_value="/bin/true"), \
         patch("app.services.probe.runner._wait_for_port_ready", side_effect=mock_wait_for_port_ready), \
         patch("asyncio.create_subprocess_exec") as mock_exec:

        mock_proc = AsyncMock()
        mock_proc.returncode = None
        mock_proc.terminate = MagicMock()
        mock_exec.return_value = mock_proc

        resolved = ResolvedProbeConfig(service_timeout_s=0.1)
        res = await probe_single_node(node, resolved_config=resolved, use_cache=False)

        assert res["status"] == "timeout"
        assert "timed out" in (res.get("error") or "").lower() or "in time" in (res.get("error") or "").lower()


@pytest.mark.asyncio
async def test_independent_service_timeout_preserves_sibling_results():
    node = {"name": "test-node", "server": "1.1.1.1", "port": 443, "type": "ss", "password": "pass", "cipher": "aes-128-gcm"}

    # Transport ok
    mock_transport = {"status": "ok", "latency_ms": 42, "target": "https://cp.cloudflare.com/generate_204", "error": None}
    # Geo ok
    mock_geo = {"status": "ok", "ip": "1.2.3.4", "country": "JP", "asn": 13335, "organization": "Cloudflare"}
    # Netflix hangs / times out, YouTube succeeds
    async def mock_check_media(proxy_url, platforms, timeout_s=2.0):
        return {
            "youtube": {"status": "ok", "region": "JP"},
            "netflix": {"status": "timeout", "error": "netflix probe timed out"},
        }

    mock_runner_ctx = {"proxy_url": "http://127.0.0.1:21000", "port": 21000, "is_runner": True}

    from contextlib import asynccontextmanager
    @asynccontextmanager
    async def mock_spawn(n, **kwargs):
        yield mock_runner_ctx

    with patch("app.services.probe.service.spawn_node_runner", side_effect=mock_spawn), \
         patch("app.services.probe.service.check_transport", return_value=mock_transport), \
         patch("app.services.probe.service.check_geo_identity", return_value=mock_geo), \
         patch("app.services.probe.service.check_media_unlock", side_effect=mock_check_media):

        resolved = ResolvedProbeConfig(service_timeout_s=2.0, media_platforms=("youtube", "netflix"))
        res = await probe_single_node(node, resolved_config=resolved, use_cache=False)

        # Overall node must remain OK because transport succeeded!
        assert res["status"] == "ok"
        assert res["latency_ms"] == 42
        assert res["country"] == "JP"
        assert res["media"]["youtube"]["status"] == "ok"
        assert res["media"]["netflix"]["status"] == "timeout"


@pytest.mark.asyncio
async def test_speed_test_timeout_handling():
    # Test that speed test timeout produces structured outcome
    mock_runner_ctx = {"proxy_url": "http://127.0.0.1:21000", "port": 21000, "is_runner": True}

    from contextlib import asynccontextmanager
    @asynccontextmanager
    async def mock_spawn(n, **kwargs):
        yield mock_runner_ctx

    mock_transport = {"status": "ok", "latency_ms": 30}
    mock_geo = {"status": "ok", "ip": "1.1.1.1", "country": "US"}
    mock_speed = {"status": "timeout", "speed_mbps": 12.5, "bytes": 1048576, "duration_s": 0.67, "error": "speed test timed out"}

    with patch("app.services.probe.service.spawn_node_runner", side_effect=mock_spawn), \
         patch("app.services.probe.service.check_transport", return_value=mock_transport), \
         patch("app.services.probe.service.check_geo_identity", return_value=mock_geo), \
         patch("app.services.probe.service.check_download_speed", return_value=mock_speed):

        node = {"name": "speed-node", "server": "1.1.1.1", "port": 443, "type": "ss", "password": "p", "cipher": "aes-128-gcm"}
        resolved = ResolvedProbeConfig(speedtest_enabled=True, service_timeout_s=1.0)
        res = await probe_single_node(node, resolved_config=resolved, use_cache=False)

        assert res["status"] == "ok"
        assert res["speed_mbps"] == 12.5


@pytest.mark.asyncio
async def test_node_wide_budget_timeout():
    node = {"name": "slow-node", "server": "1.1.1.1", "port": 443, "type": "ss", "password": "p", "cipher": "aes-128-gcm"}

    mock_runner_ctx = {"proxy_url": "http://127.0.0.1:21000", "port": 21000, "is_runner": True}
    from contextlib import asynccontextmanager
    @asynccontextmanager
    async def mock_spawn(n, **kwargs):
        yield mock_runner_ctx

    async def mock_slow_transport(*args, **kwargs):
        await asyncio.sleep(0.5)
        return {"status": "ok", "latency_ms": 500}

    with patch("app.services.probe.service.spawn_node_runner", side_effect=mock_spawn), \
         patch("app.services.probe.service.check_transport", side_effect=mock_slow_transport):

        # Node budget of 0.1s expires during the 0.5s transport
        resolved = ResolvedProbeConfig(node_timeout_s=0.1)
        res = await probe_single_node(node, resolved_config=resolved, use_cache=False)

        assert res["status"] == "timeout"
        assert "total budget" in (res.get("error") or "").lower() or "timed out" in (res.get("error") or "").lower()


@pytest.mark.asyncio
async def test_scheduler_dynamic_cron_tick_and_no_overlap(isolated_db):
    import app.services.scheduler as sched_mod

    # Reset scheduler module state
    sched_mod._last_probe_run_at = 0.0
    if sched_mod._probe_lock.locked():
        sched_mod._probe_lock.release()

    config = await get_probe_config(isolated_db)
    config.probe_enabled = True
    config.probe_cron_enabled = True
    config.probe_cron_interval_minutes = 30
    await isolated_db.commit()

    run_mock = AsyncMock()

    with patch("app.services.scheduler.AsyncSessionLocal") as mock_session_ctx, \
         patch("app.services.scheduler.collect_all_subscription_nodes", return_value=[{"name": "n1"}]), \
         patch("app.services.probe.service.probe_batch_nodes", side_effect=run_mock):

        mock_session_ctx.return_value.__aenter__.return_value = isolated_db

        # 1. First tick after startup sets baseline, no immediate catch-up run
        await _poll_node_probes()
        assert run_mock.call_count == 0
        assert sched_mod._last_probe_run_at > 0

        # 2. Advance time by 10 minutes: interval 30 has not elapsed -> no run
        baseline = sched_mod._last_probe_run_at
        with patch("app.services.scheduler.time.time", return_value=baseline + 600):
            await _poll_node_probes()
            assert run_mock.call_count == 0

        # 3. Advance time by 31 minutes: interval 30 elapsed -> triggers run!
        with patch("app.services.scheduler.time.time", return_value=baseline + 1860):
            await _poll_node_probes()
            assert run_mock.call_count == 1

        # 4. Turn off probe_cron_enabled mid-flight -> next tick starts nothing
        config.probe_cron_enabled = False
        await isolated_db.commit()

        with patch("app.services.scheduler.time.time", return_value=baseline + 5000):
            await _poll_node_probes()
            assert run_mock.call_count == 1, "Must not trigger when probe_cron_enabled is False"

        # 5. Turn back on, test lock prevents overlapping runs
        config.probe_cron_enabled = True
        config.probe_cron_interval_minutes = 5
        await isolated_db.commit()

        sched_mod._last_probe_run_at = 1000.0
        async with sched_mod._probe_lock:
            with patch("app.services.scheduler.time.time", return_value=5000.0):
                await _poll_node_probes()
                # Lock was held by another coroutine -> skipped, no duplicate run
                assert run_mock.call_count == 1
