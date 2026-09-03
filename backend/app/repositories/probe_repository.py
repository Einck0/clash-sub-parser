from __future__ import annotations

import uuid
from datetime import datetime, timezone
from sqlalchemy import desc, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.node import Node
from app.models.probe_domain import ProbeJob, ProbeObservation, ProbeProfile


class ProbeProfileRepository:
    """探测配置文件仓储"""

    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_by_id(self, profile_id: int) -> ProbeProfile | None:
        return await self.session.get(ProbeProfile, profile_id)

    async def get_by_logical_id(self, logical_id: str) -> ProbeProfile | None:
        stmt = select(ProbeProfile).where(ProbeProfile.logical_id == logical_id)
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def get_by_name(self, name: str) -> ProbeProfile | None:
        stmt = select(ProbeProfile).where(ProbeProfile.name == name)
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def list_profiles(self) -> list[ProbeProfile]:
        stmt = select(ProbeProfile).order_by(ProbeProfile.id.asc())
        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def create_profile(
        self,
        name: str,
        platforms: list[str] | None = None,
        timeout_s: int = 10,
        concurrency: int = 5,
        max_bytes: int = 50000000,
        min_speed_mbps: float | None = None,
        logical_id: str | None = None,
    ) -> ProbeProfile:
        profile = ProbeProfile(
            logical_id=logical_id or str(uuid.uuid4()),
            name=name,
            platforms_json=platforms or ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"],
            timeout_s=timeout_s,
            concurrency=concurrency,
            max_bytes=max_bytes,
            min_speed_mbps=min_speed_mbps,
        )
        self.session.add(profile)
        await self.session.flush()
        return profile

    async def ensure_default_profile(self) -> ProbeProfile:
        existing = await self.get_by_name("default")
        if existing:
            return existing
        return await self.create_profile(
            name="default",
            platforms=["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"],
            timeout_s=10,
            concurrency=5,
            logical_id="profile-default-001",
        )


class ProbeJobRepository:
    """探测批任务仓储"""

    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def create_job(
        self,
        profile_id: int,
        trigger_type: str = "manual",
        logical_id: str | None = None,
    ) -> ProbeJob:
        job = ProbeJob(
            logical_id=logical_id or str(uuid.uuid4()),
            profile_id=profile_id,
            trigger_type=trigger_type,
            status="queued",
            queued_at=datetime.now(timezone.utc),
        )
        self.session.add(job)
        await self.session.flush()
        return job

    async def get_by_id(self, job_id: int) -> ProbeJob | None:
        return await self.session.get(ProbeJob, job_id)

    async def get_by_logical_id(self, logical_id: str) -> ProbeJob | None:
        stmt = select(ProbeJob).where(ProbeJob.logical_id == logical_id)
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def list_jobs(self, limit: int = 50, offset: int = 0) -> list[ProbeJob]:
        stmt = select(ProbeJob).order_by(desc(ProbeJob.queued_at)).limit(limit).offset(offset)
        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def update_job_status(
        self,
        job_id: int,
        status: str,
        started_at: datetime | None = None,
        finished_at: datetime | None = None,
        summary: dict | None = None,
    ) -> ProbeJob | None:
        job = await self.get_by_id(job_id)
        if not job:
            return None
        job.status = status
        if started_at:
            job.started_at = started_at
        if finished_at:
            job.finished_at = finished_at
        if summary is not None:
            job.summary_json = summary
        await self.session.flush()
        return job


class ProbeObservationRepository:
    """不可变探测观测记录仓储"""

    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def create_observation(
        self,
        job_id: int,
        node_id: int,
        status: str,
        latency_ms: int | None = None,
        speed_mbps: float | None = None,
        egress_ip: str | None = None,
        country: str | None = None,
        asn: int | None = None,
        organization: str | None = None,
        media_results: dict | None = None,
        error: str | None = None,
        observed_at: datetime | None = None,
    ) -> ProbeObservation:
        obs = ProbeObservation(
            job_id=job_id,
            node_id=node_id,
            status=status,
            latency_ms=latency_ms,
            speed_mbps=speed_mbps,
            egress_ip=egress_ip,
            country=country,
            asn=asn,
            organization=organization,
            media_results_json=media_results or {},
            error=error,
            observed_at=observed_at or datetime.now(timezone.utc),
        )
        self.session.add(obs)
        await self.session.flush()
        return obs

    async def get_latest_observation_for_node(
        self,
        node_id: int,
    ) -> ProbeObservation | None:
        stmt = (
            select(ProbeObservation)
            .where(ProbeObservation.node_id == node_id)
            .order_by(desc(ProbeObservation.observed_at), desc(ProbeObservation.id))
            .limit(1)
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def get_latest_observations_for_nodes(
        self,
        node_ids: list[int],
    ) -> dict[int, ProbeObservation]:
        if not node_ids:
            return {}
        stmt = (
            select(ProbeObservation)
            .where(ProbeObservation.node_id.in_(node_ids))
            .order_by(desc(ProbeObservation.observed_at), desc(ProbeObservation.id))
        )
        result = await self.session.execute(stmt)
        obs_list = result.scalars().all()
        latest_map: dict[int, ProbeObservation] = {}
        for obs in obs_list:
            if obs.node_id not in latest_map:
                latest_map[obs.node_id] = obs
        return latest_map

    async def list_observations_by_node_logical_id(
        self,
        node_logical_id: str,
        limit: int = 20,
    ) -> list[ProbeObservation]:
        stmt = (
            select(ProbeObservation)
            .join(Node, ProbeObservation.node_id == Node.id)
            .where(Node.logical_id == node_logical_id)
            .order_by(desc(ProbeObservation.observed_at))
            .limit(limit)
        )
        result = await self.session.execute(stmt)
        return list(result.scalars().all())
