from __future__ import annotations

from datetime import datetime, timezone
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.node import Node
from app.models.node_probe_result import NodeProbeResult
from app.models.quarantine import QuarantineRecord
from app.repositories.probe_repository import (
    ProbeJobRepository,
    ProbeObservationRepository,
    ProbeProfileRepository,
)


async def migrate_legacy_probe_results(
    session: AsyncSession,
) -> dict[str, int]:
    """从传统 node_probe_results 迁移至规范化不可变观测记录"""
    profile_repo = ProbeProfileRepository(session)
    job_repo = ProbeJobRepository(session)
    obs_repo = ProbeObservationRepository(session)

    # 1 确保存在迁移专用 Profile
    legacy_profile = await profile_repo.get_by_name("legacy-migrated")
    if not legacy_profile:
        legacy_profile = await profile_repo.create_profile(
            name="legacy-migrated",
            platforms=["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"],
            timeout_s=5,
            concurrency=5,
            logical_id="profile-legacy-migrated-001",
        )

    # 2 创建迁移专用批任务记录
    migration_job = await job_repo.create_job(
        profile_id=legacy_profile.id,
        trigger_type="system",
        logical_id="job-legacy-probe-migration-001",
    )

    # 3 查询所有旧版探测记录
    stmt = select(NodeProbeResult).order_by(NodeProbeResult.id.asc())
    res = await session.execute(stmt)
    legacy_records = res.scalars().all()

    migrated_count = 0
    quarantined_count = 0
    skipped_count = 0

    for item in legacy_records:
        # 按 server 与 port 以及 name 查找对应规范化节点
        node_stmt = select(Node).where(
            Node.server == item.server,
            Node.port == (item.port or 0),
            Node.name == item.name,
        )
        node_res = await session.execute(node_stmt)
        node = node_res.scalar_one_or_none()

        if not node:
            # 二次兜底：按 server 与 port 匹配
            fallback_stmt = select(Node).where(
                Node.server == item.server,
                Node.port == (item.port or 0),
            )
            fallback_res = await session.execute(fallback_stmt)
            node = fallback_res.scalars().first()

        if not node:
            # 隔离无法关联到规范化节点的孤儿探测记录
            quarantine = QuarantineRecord(
                category="probe_result",
                source_table="node_probe_results",
                source_record_id=str(item.id),
                reason=f"无法匹配到规范化节点 (server={item.server}, port={item.port})",
                payload_json={
                    "name": item.name,
                    "server": item.server,
                    "port": item.port,
                    "node_key": item.node_key,
                },
            )
            session.add(quarantine)
            quarantined_count += 1
            continue

        obs_time = datetime.fromtimestamp(item.checked_at, tz=timezone.utc)
        await obs_repo.create_observation(
            job_id=migration_job.id,
            node_id=node.id,
            status=item.status,
            latency_ms=item.latency_ms,
            speed_mbps=item.speed_mbps,
            egress_ip=item.ip,
            country=item.country,
            asn=item.asn,
            organization=item.organization,
            media_results=item.media or {},
            error=item.error,
            observed_at=obs_time,
        )
        migrated_count += 1

    await job_repo.update_job_status(
        job_id=migration_job.id,
        status="completed",
        finished_at=datetime.now(timezone.utc),
        summary={
            "total_legacy_records": len(legacy_records),
            "migrated": migrated_count,
            "quarantined": quarantined_count,
            "skipped": skipped_count,
        },
    )
    await session.flush()

    return {
        "total": len(legacy_records),
        "migrated": migrated_count,
        "quarantined": quarantined_count,
        "skipped": skipped_count,
    }
