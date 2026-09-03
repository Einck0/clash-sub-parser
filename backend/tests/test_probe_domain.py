from __future__ import annotations

from datetime import datetime, timedelta, timezone
import pytest
from sqlalchemy import select

from tests.conftest import TestSession
from app.models.node import Node
from app.models.node_probe_result import NodeProbeResult
from app.models.probe_domain import ProbeObservation
from app.models.quarantine import QuarantineRecord
from app.repositories.probe_repository import (
    ProbeJobRepository,
    ProbeObservationRepository,
    ProbeProfileRepository,
)
from app.schemas.probe_domain import ProbeObservationDTO
from app.services.probe.eligibility_evaluator import evaluate_observation_eligibility
from app.services.probe.legacy_probe_migrator import migrate_legacy_probe_results
from app.services.probe.redactor import redact_node_data, sanitize_diagnostic_message


@pytest.mark.asyncio
async def test_immutable_observations_and_history():
    async with TestSession() as session:
        profile_repo = ProbeProfileRepository(session)
        job_repo = ProbeJobRepository(session)
        obs_repo = ProbeObservationRepository(session)

        profile = await profile_repo.ensure_default_profile()
        job1 = await job_repo.create_job(profile_id=profile.id, trigger_type="manual")
        job2 = await job_repo.create_job(profile_id=profile.id, trigger_type="scheduled")

        node = Node(
            logical_id="node-imm-001",
            name="香港高速 01",
            protocol="shadowsocks",
            server="1.2.3.4",
            port=8388,
            normalized_payload={"type": "ss", "server": "1.2.3.4", "port": 8388},
            payload_fingerprint="fp-imm-001",
            lifecycle_state="active",
        )
        session.add(node)
        await session.flush()

        # 第一次观测：测速 10Mbps
        t1 = datetime.now(timezone.utc) - timedelta(minutes=10)
        obs1 = await obs_repo.create_observation(
            job_id=job1.id,
            node_id=node.id,
            status="ok",
            latency_ms=50,
            speed_mbps=10.0,
            observed_at=t1,
        )

        # 第二次观测：测速 80Mbps
        t2 = datetime.now(timezone.utc)
        obs2 = await obs_repo.create_observation(
            job_id=job2.id,
            node_id=node.id,
            status="ok",
            latency_ms=45,
            speed_mbps=80.0,
            observed_at=t2,
        )
        await session.commit()

        # 验证两条观测记录均被不可变持久化且未相互覆盖
        assert obs1.id != obs2.id
        history = await obs_repo.list_observations_by_node_logical_id(node.logical_id)
        assert len(history) == 2
        assert history[0].speed_mbps == 80.0
        assert history[1].speed_mbps == 10.0

        # 最新观测查询返回最新一条
        latest = await obs_repo.get_latest_observation_for_node(node.id)
        assert latest is not None
        assert latest.id == obs2.id
        assert latest.speed_mbps == 80.0


def test_eligibility_evaluator_all_categories():
    now = datetime.now(timezone.utc)

    # 1 缺少观测记录 (missing)
    res_missing = evaluate_observation_eligibility(None)
    assert res_missing.eligible is False
    assert res_missing.status == "missing"

    # 2 观测失败 (failed)
    failed_obs = ProbeObservationDTO(
        job_id=1,
        node_id=1,
        status="fail",
        error="握手超时",
        observed_at=now,
    )
    res_failed = evaluate_observation_eligibility(failed_obs)
    assert res_failed.eligible is False
    assert res_failed.status == "failed"

    # 3 观测过期 (stale)
    stale_obs = ProbeObservationDTO(
        job_id=1,
        node_id=1,
        status="ok",
        speed_mbps=50.0,
        observed_at=now - timedelta(seconds=7200),
    )
    res_stale = evaluate_observation_eligibility(stale_obs, max_age_seconds=3600)
    assert res_stale.eligible is False
    assert res_stale.status == "stale"

    # 4 缺少流媒体平台探测项 (incompatible)
    incomplete_obs = ProbeObservationDTO(
        job_id=1,
        node_id=1,
        status="ok",
        speed_mbps=50.0,
        media_results={"youtube": {"status": "ok", "unlocked": True}},
        observed_at=now,
    )
    res_incompatible = evaluate_observation_eligibility(incomplete_obs, required_media=["netflix"])
    assert res_incompatible.eligible is False
    assert res_incompatible.status == "incompatible"

    # 5 测速或流媒体未达标 (insufficient)
    slow_obs = ProbeObservationDTO(
        job_id=1,
        node_id=1,
        status="ok",
        speed_mbps=5.0,
        observed_at=now,
    )
    res_slow = evaluate_observation_eligibility(slow_obs, min_speed_mbps=20.0)
    assert res_slow.eligible is False
    assert res_slow.status == "insufficient"

    # 6 满足所有要求 (valid)
    valid_obs = ProbeObservationDTO(
        job_id=1,
        node_id=1,
        status="ok",
        speed_mbps=50.0,
        media_results={"netflix": {"status": "full", "unlocked": True}},
        observed_at=now,
    )
    res_valid = evaluate_observation_eligibility(
        valid_obs,
        min_speed_mbps=20.0,
        required_media=["netflix"],
        max_age_seconds=3600,
    )
    assert res_valid.eligible is True
    assert res_valid.status == "valid"


def test_redactor_sanitizes_credentials():
    node = {
        "name": "HK-Node",
        "type": "vmess",
        "server": "1.2.3.4",
        "port": 443,
        "uuid": "e9b5f3a1-1234-4567-89ab-cdef01234567",
        "password": "supersecretpassword",
        "token": "mysecrettoken",
    }
    redacted = redact_node_data(node)
    assert redacted["uuid"] == "[REDACTED]"
    assert redacted["password"] == "[REDACTED]"
    assert redacted["token"] == "[REDACTED]"
    assert redacted["name"] == "HK-Node"
    assert redacted["server"] == "1.2.3.4"

    raw_msg = "Error connecting to https://user:secretpass123@example.com/api?token=abc12345 with uuid e9b5f3a1-1234-4567-89ab-cdef01234567"
    sanitized = sanitize_diagnostic_message(raw_msg)
    assert "secretpass123" not in sanitized
    assert "abc12345" not in sanitized
    assert "e9b5f3a1-1234-4567-89ab-cdef01234567" not in sanitized
    assert "[REDACTED]" in sanitized
    assert "[REDACTED_UUID]" in sanitized


@pytest.mark.asyncio
async def test_legacy_probe_migrator_deterministic():
    async with TestSession() as session:
        # 准备规范化节点
        node1 = Node(
            logical_id="node-leg-001",
            name="日本 01",
            protocol="trojan",
            server="jp.example.com",
            port=443,
            normalized_payload={"type": "trojan", "server": "jp.example.com", "port": 443},
            payload_fingerprint="fp-leg-001",
            lifecycle_state="active",
        )
        session.add(node1)

        # 准备旧版探测记录：1 条可关联，1 条孤儿记录
        legacy1 = NodeProbeResult(
            node_key="key1",
            name="日本 01",
            server="jp.example.com",
            port=443,
            type="trojan",
            status="ok",
            latency_ms=80,
            speed_mbps=35.0,
            ip="100.1.2.3",
            country="JP",
            asn=12345,
            checked_at=1700000000,
        )
        legacy_orphan = NodeProbeResult(
            node_key="key_orphan",
            name="未知孤儿节点",
            server="99.99.99.99",
            port=8080,
            type="ss",
            status="fail",
            error="Connection refused",
            checked_at=1700000000,
        )
        session.add_all([legacy1, legacy_orphan])
        await session.commit()

        # 执行迁移
        summary = await migrate_legacy_probe_results(session)
        assert summary["total"] == 2
        assert summary["migrated"] == 1
        assert summary["quarantined"] == 1

        # 验证迁移后的观测记录
        obs_res = await session.execute(
            select(ProbeObservation).where(ProbeObservation.node_id == node1.id)
        )
        obs_list = obs_res.scalars().all()
        assert len(obs_list) == 1
        assert obs_list[0].speed_mbps == 35.0
        assert obs_list[0].country == "JP"

        # 验证隔离区记录
        q_res = await session.execute(
            select(QuarantineRecord).where(QuarantineRecord.category == "probe_result")
        )
        q_list = q_res.scalars().all()
        assert len(q_list) == 1
        assert "99.99.99.99" in q_list[0].reason or "99.99.99.99" in str(q_list[0].payload_json)
