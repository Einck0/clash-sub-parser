from __future__ import annotations

import pytest
import pytest_asyncio
from httpx import ASGITransport, AsyncClient
from pydantic import ValidationError
from sqlalchemy import select
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

from app.database import Base
from app.main import app
from app.models.probe_config import ProbeConfig
from app.schemas.probe import ProbeSettingsRead, ProbeSettingsUpdate
from app.services.config_transfer_service import export_data, import_data, reset_data
from app.services.probe_settings_service import get_probe_config, to_read


@pytest_asyncio.fixture()
async def isolated_db_session():
    engine = create_async_engine("sqlite+aiosqlite:///:memory:", future=True)
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    maker = async_sessionmaker(engine, expire_on_commit=False)
    async with maker() as session:
        yield session
    await engine.dispose()


@pytest.mark.asyncio
async def test_probe_config_model_defaults():
    config = ProbeConfig()
    assert config.probe_concurrency == 10
    assert config.probe_service_timeout_ms == 2000
    assert config.probe_cron_enabled is True
    assert config.probe_cron_interval_minutes == 60
    assert config.probe_timeout_ms == 3000


@pytest.mark.asyncio
async def test_probe_settings_read_defaults():
    read_schema = ProbeSettingsRead()
    assert read_schema.probe_concurrency == 10
    assert read_schema.probe_service_timeout_ms == 2000
    assert read_schema.probe_cron_enabled is True
    assert read_schema.probe_cron_interval_minutes == 60


def test_probe_settings_update_validation():
    # Valid update
    update = ProbeSettingsUpdate(
        probe_concurrency=10,
        probe_service_timeout_ms=2500,
        probe_cron_enabled=False,
        probe_cron_interval_minutes=30,
        probe_timeout_ms=0,
    )
    assert update.probe_concurrency == 10
    assert update.probe_service_timeout_ms == 2500
    assert update.probe_cron_enabled is False
    assert update.probe_cron_interval_minutes == 30
    assert update.probe_timeout_ms == 0

    # Bounds: probe_concurrency (1..20)
    with pytest.raises(ValidationError):
        ProbeSettingsUpdate(probe_concurrency=0)
    with pytest.raises(ValidationError):
        ProbeSettingsUpdate(probe_concurrency=21)

    # Bounds: probe_service_timeout_ms (500..30000)
    with pytest.raises(ValidationError):
        ProbeSettingsUpdate(probe_service_timeout_ms=499)
    with pytest.raises(ValidationError):
        ProbeSettingsUpdate(probe_service_timeout_ms=30001)

    # Bounds: probe_cron_interval_minutes (1..1440)
    with pytest.raises(ValidationError):
        ProbeSettingsUpdate(probe_cron_interval_minutes=0)
    with pytest.raises(ValidationError):
        ProbeSettingsUpdate(probe_cron_interval_minutes=1441)

    # Bounds: probe_timeout_ms (0..30000)
    with pytest.raises(ValidationError):
        ProbeSettingsUpdate(probe_timeout_ms=-1)
    with pytest.raises(ValidationError):
        ProbeSettingsUpdate(probe_timeout_ms=30001)


@pytest.mark.asyncio
async def test_get_probe_config_lazy_backfill(isolated_db_session):
    # Insert a legacy row where new columns might be None or old concurrency 5
    legacy_row = ProbeConfig(
        id=1,
        probe_enabled=True,
        probe_interval_minutes=10,
        speedtest_enabled=False,
        media_check_enabled=True,
        probe_concurrency=5,  # Explicitly saved old concurrency
    )
    # Manually set new fields to None to simulate legacy database row
    legacy_row.probe_service_timeout_ms = None
    legacy_row.probe_cron_enabled = None
    legacy_row.probe_cron_interval_minutes = None

    isolated_db_session.add(legacy_row)
    await isolated_db_session.commit()

    # get_probe_config should lazily backfill unset new fields without modifying old concurrency
    config = await get_probe_config(isolated_db_session)
    assert config.probe_concurrency == 5, "Explicit old concurrency must NOT be overwritten"
    assert config.probe_service_timeout_ms == 2000
    assert config.probe_cron_enabled is True
    assert config.probe_cron_interval_minutes == 60


@pytest.mark.asyncio
async def test_settings_api_crud_and_validation():
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        # 1. Read default settings
        res = await ac.get("/api/probe/settings")
        assert res.status_code == 200
        data = res.json()
        assert "probe_service_timeout_ms" in data
        assert "probe_cron_enabled" in data
        assert "probe_cron_interval_minutes" in data

        # 2. Patch valid settings
        patch_res = await ac.patch(
            "/api/probe/settings",
            json={
                "probe_service_timeout_ms": 3500,
                "probe_cron_enabled": False,
                "probe_cron_interval_minutes": 45,
                "probe_concurrency": 8,
                "probe_timeout_ms": 0,
            },
        )
        assert patch_res.status_code == 200
        patch_data = patch_res.json()
        assert patch_data["probe_service_timeout_ms"] == 3500
        assert patch_data["probe_cron_enabled"] is False
        assert patch_data["probe_cron_interval_minutes"] == 45
        assert patch_data["probe_concurrency"] == 8
        assert patch_data["probe_timeout_ms"] == 0

        # 3. Reject invalid settings
        bad_res = await ac.patch(
            "/api/probe/settings",
            json={"probe_service_timeout_ms": 100},
        )
        assert bad_res.status_code == 422

        # 4. Restore defaults
        await ac.patch(
            "/api/probe/settings",
            json={
                "probe_service_timeout_ms": 2000,
                "probe_cron_enabled": True,
                "probe_cron_interval_minutes": 60,
                "probe_concurrency": 10,
                "probe_timeout_ms": 3000,
            },
        )


@pytest.mark.asyncio
async def test_config_transfer_roundtrip_with_new_fields(isolated_db_session):
    # Configure probe_config
    config = await get_probe_config(isolated_db_session)
    config.probe_service_timeout_ms = 4000
    config.probe_cron_enabled = False
    config.probe_cron_interval_minutes = 90
    config.probe_concurrency = 12
    await isolated_db_session.commit()

    # Export
    exported = await export_data(isolated_db_session)
    probe_rows = exported["tables"]["probe_config"]
    assert len(probe_rows) == 1
    row = probe_rows[0]
    assert row["probe_service_timeout_ms"] == 4000
    assert row["probe_cron_enabled"] is False
    assert row["probe_cron_interval_minutes"] == 90
    assert row["probe_concurrency"] == 12

    # Reset
    await reset_data(isolated_db_session)
    reset_row = (await isolated_db_session.execute(select(ProbeConfig))).scalar_one()
    assert reset_row.probe_concurrency == 10
    assert reset_row.probe_service_timeout_ms == 2000
    assert reset_row.probe_cron_enabled is True
    assert reset_row.probe_cron_interval_minutes == 60

    # Import
    await import_data(isolated_db_session, {"tables": exported["tables"]})
    imported_row = (await isolated_db_session.execute(select(ProbeConfig))).scalar_one()
    assert imported_row.probe_service_timeout_ms == 4000
    assert imported_row.probe_cron_enabled is False
    assert imported_row.probe_cron_interval_minutes == 90
    assert imported_row.probe_concurrency == 12
