import time
from unittest.mock import patch

import pytest

from app.models.probe_config import ProbeConfig
from app.services.probe_settings_service import get_probe_config
from app.services.scheduler import ProbeScheduleRuntime, get_probe_schedule_runtime, _probe_lock
from tests.conftest import TestSession


@pytest.mark.asyncio
async def test_probe_status_api_initializing_state(client):
    """Scenario: Process start before baseline has been established."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()

    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = True
        config.probe_cron_interval_minutes = 60
        await session.commit()

    res = await client.get("/api/probe/status")
    assert res.status_code == 200
    data = res.json()

    assert data["state"] == "initializing"
    assert abs(data["server_now"] - int(time.time())) <= 2
    assert data["interval_minutes"] == 60
    assert data["next_expected_at"] is None
    assert data["last_started_at"] is None
    assert data["last_finished_at"] is None
    assert data["last_summary"] is None
    assert data["last_error_code"] is None


@pytest.mark.asyncio
async def test_probe_status_api_waiting_state(client):
    """Scenario: Baseline established, scheduler waiting for next interval."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()

    now = 1788657000.0
    runtime.record_baseline(now)

    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = True
        config.probe_cron_interval_minutes = 30
        await session.commit()

    with patch("time.time", return_value=now + 300):
        res = await client.get("/api/probe/status")
        assert res.status_code == 200
        data = res.json()

        assert data["state"] == "waiting"
        assert data["server_now"] == int(now + 300)
        assert data["interval_minutes"] == 30
        assert data["next_expected_at"] == int(now + 1800)
        assert data["last_error_code"] is None


@pytest.mark.asyncio
async def test_probe_status_api_running_state(client):
    """Scenario: Probe execution in flight; next_expected_at must be null."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()

    now = 1788657000.0
    runtime.record_baseline(now)
    runtime.record_start(now + 1800)

    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = True
        config.probe_cron_interval_minutes = 30
        await session.commit()

    res = await client.get("/api/probe/status")
    assert res.status_code == 200
    data = res.json()

    assert data["state"] == "running"
    assert data["last_started_at"] == int(now + 1800)
    assert data["next_expected_at"] is None
    assert data["last_error_code"] is None


@pytest.mark.asyncio
async def test_probe_status_api_lock_held_infers_running(client):
    """Scenario: Even if runtime object has not flipped, held _probe_lock indicates running."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()
    runtime.record_baseline(1788657000.0)

    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = True
        await session.commit()

    async with _probe_lock:
        res = await client.get("/api/probe/status")
        assert res.status_code == 200
        data = res.json()
        assert data["state"] == "running"
        assert data["next_expected_at"] is None


@pytest.mark.asyncio
async def test_probe_status_api_disabled_state(client):
    """Scenario: Master switch or cron switch disabled -> state=disabled, next_expected_at=null."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()
    runtime.record_baseline(1788657000.0)

    async with TestSession() as session:
        config = await get_probe_config(session)
        # 1. probe_enabled = False
        config.probe_enabled = False
        config.probe_cron_enabled = True
        await session.commit()

    res = await client.get("/api/probe/status")
    assert res.status_code == 200
    data = res.json()
    assert data["state"] == "disabled"
    assert data["next_expected_at"] is None

    # 2. probe_cron_enabled = False
    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = False
        await session.commit()

    res = await client.get("/api/probe/status")
    assert res.status_code == 200
    data = res.json()
    assert data["state"] == "disabled"
    assert data["next_expected_at"] is None


@pytest.mark.asyncio
async def test_probe_status_api_failed_state_with_sanitized_error(client):
    """Scenario: Background job fails -> stable categorized error code, no raw traceback or secrets."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()

    now = 1788657000.0
    runtime.record_baseline(now)
    runtime.record_start(now + 600)
    runtime.record_failure(now + 610, error_code="probe_run_failed")

    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = True
        config.probe_cron_interval_minutes = 60
        await session.commit()

    res = await client.get("/api/probe/status")
    assert res.status_code == 200
    data = res.json()

    assert data["state"] == "failed"
    assert data["last_finished_at"] == int(now + 610)
    assert data["last_error_code"] == "probe_run_failed"
    # Must NOT contain traceback, exception class, or host credentials
    assert "Traceback" not in str(data)
    assert "password" not in str(data)
    assert data["next_expected_at"] == int(now + 610 + 3600)


@pytest.mark.asyncio
async def test_probe_status_api_success_summary_recorded(client):
    """Scenario: Successful background run updates summary and resets error."""
    runtime = get_probe_schedule_runtime()
    runtime.reset_for_restart()

    now = 1788657000.0
    runtime.record_baseline(now)
    runtime.record_start(now + 3600)
    runtime.record_success(now + 3620, summary={"total": 12, "ok": 10, "fail": 1, "timeout": 1, "skipped": 0})

    async with TestSession() as session:
        config = await get_probe_config(session)
        config.probe_enabled = True
        config.probe_cron_enabled = True
        config.probe_cron_interval_minutes = 60
        await session.commit()

    res = await client.get("/api/probe/status")
    assert res.status_code == 200
    data = res.json()

    assert data["state"] == "waiting"
    assert data["last_started_at"] == int(now + 3600)
    assert data["last_finished_at"] == int(now + 3620)
    assert data["last_summary"] == {"total": 12, "ok": 10, "fail": 1, "timeout": 1, "skipped": 0}
    assert data["last_error_code"] is None
    assert data["next_expected_at"] == int(now + 3620 + 3600)


@pytest.mark.asyncio
async def test_probe_status_api_protected_by_auth(client):
    """Scenario: Protected endpoint requires token when API authentication is enabled."""
    from app.services.security_settings_service import get_security_settings, hash_token

    async with TestSession() as session:
        sec = await get_security_settings(session)
        sec.auth_enabled = True
        sec.protect_api = True
        sec.token_hash = hash_token("secret-token-123456")
        await session.commit()

    # 1. Without token -> 401
    res_no_auth = await client.get("/api/probe/status")
    assert res_no_auth.status_code == 401

    # 2. With valid header token -> 200
    res_auth = await client.get("/api/probe/status", headers={"x-clash-token": "secret-token-123456"})
    assert res_auth.status_code == 200

    # Reset security settings
    async with TestSession() as session:
        sec = await get_security_settings(session)
        sec.auth_enabled = False
        sec.protect_api = False
        await session.commit()
