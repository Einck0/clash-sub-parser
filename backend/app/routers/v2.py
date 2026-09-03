from __future__ import annotations

import shutil
from typing import Any
from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.database import get_db
from app.models.node import Node
from app.models.source import Source
from app.repositories.node_repository import NodeRepository
from app.repositories.probe_repository import (
    ProbeJobRepository,
    ProbeObservationRepository,
    ProbeProfileRepository,
)
from app.schemas.bundle import (
    BundlePreflightResult,
    RevisionDTO,
)
from app.schemas.inventory import (
    InventoryListResponse,
    InventoryNodeItem,
    SafeSourceProvenance,
    SourceRead,
    SourceRevisionRead,
)
from app.schemas.logical_config import (
    CompilationResult,
    LogicalConfigurationBundle,
)
from app.schemas.probe_domain import (
    ProbeJobDTO,
    ProbeObservationDTO,
    ProbeProfileDTO,
)
from app.services.bundle_preflight_service import preflight_bundle_json
from app.services.canonical_compiler import CanonicalGraphResolver
from app.services.revision_service import (
    create_and_activate_revision,
    get_active_revision,
    list_revision_history,
    rollback_to_revision,
)
from app.services.schema_readiness import check_database_readiness

router = APIRouter(prefix="/api/v2", tags=["v2-domain"])


@router.get("/inventory/nodes", response_model=InventoryListResponse)
async def list_v2_nodes(
    page: int = Query(1, ge=1),
    page_size: int = Query(50, ge=1, le=500),
    lifecycle_state: str | None = None,
    protocol: str | None = None,
    db: AsyncSession = Depends(get_db),
) -> InventoryListResponse:
    """查询规范化节点库存列表，脱敏敏感认证参数"""
    offset = (page - 1) * page_size
    stmt = (
        select(Node)
        .options(selectinload(Node.source_links))
        .order_by(Node.id)
        .limit(page_size)
        .offset(offset)
    )
    if lifecycle_state:
        stmt = stmt.where(Node.lifecycle_state == lifecycle_state)
    if protocol:
        stmt = stmt.where(Node.protocol == protocol)

    result = await db.execute(stmt)
    nodes = list(result.scalars().all())
    total = await NodeRepository.count_nodes(db, lifecycle_state=lifecycle_state, protocol=protocol)

    items: list[InventoryNodeItem] = []
    for node in nodes:
        sources_list: list[SafeSourceProvenance] = []
        for link in node.source_links or []:
            sources_list.append(
                SafeSourceProvenance(
                    source_logical_id="src_link",
                    source_name="source",
                    source_kind="subscription",
                    revision_id=str(link.source_revision_id),
                    display_name=link.display_name,
                    original_order=link.original_order,
                )
            )
        items.append(
            InventoryNodeItem(
                logical_id=node.logical_id,
                name=node.name,
                protocol=node.protocol,
                server=node.server,
                port=node.port,
                lifecycle_state=node.lifecycle_state,
                sources=sources_list,
                created_at=node.created_at,
                updated_at=node.updated_at,
            )
        )
    return InventoryListResponse(total=total, page=page, page_size=page_size, items=items)


@router.get("/inventory/sources", response_model=list[SourceRead])
async def list_v2_sources(
    db: AsyncSession = Depends(get_db),
) -> list[SourceRead]:
    """查询规范化配置源列表"""
    stmt = (
        select(Source)
        .options(selectinload(Source.revisions))
        .order_by(Source.id.asc())
    )
    result = await db.execute(stmt)
    sources = list(result.scalars().all())

    read_list: list[SourceRead] = []
    for s in sources:
        rev_read = None
        if s.revisions:
            latest_rev = s.revisions[-1]
            rev_read = SourceRevisionRead(
                revision_id=latest_rev.revision_id,
                status=latest_rev.status,
                payload_hash=latest_rev.payload_hash,
                node_count=latest_rev.node_count,
                error_summary=latest_rev.error_summary,
                fetched_at=latest_rev.fetched_at,
                created_at=latest_rev.created_at,
            )
        read_list.append(
            SourceRead(
                logical_id=s.logical_id,
                name=s.name,
                kind="manual" if s.kind == "manual" else "subscription",
                url=s.url,
                update_interval=s.update_interval,
                enabled=s.enabled,
                created_at=s.created_at,
                updated_at=s.updated_at,
                active_revision=rev_read,
            )
        )
    return read_list


@router.post("/bundle/preflight", response_model=BundlePreflightResult)
async def preflight_bundle_endpoint(
    payload: dict[str, Any],
) -> BundlePreflightResult:
    """非破坏性静态预检配置 Bundle 完整性与安全边界"""
    return preflight_bundle_json(payload)


@router.post("/compile", response_model=CompilationResult)
async def compile_configuration_endpoint(
    bundle: LogicalConfigurationBundle | None = None,
    db: AsyncSession = Depends(get_db),
) -> CompilationResult:
    """执行纯内存规范化配置编译"""
    if bundle is None:
        active_rev = await get_active_revision(db)
        if not active_rev:
            raise HTTPException(status_code=400, detail="无激活配置版本，请先传入 Bundle 或激活版本")
        bundle = LogicalConfigurationBundle.model_validate(active_rev.bundle_json)

    # 加载当前规范化节点
    nodes = await NodeRepository.list_nodes(db, limit=1000, offset=0, lifecycle_state="active")

    node_dicts = [
        {
            "logical_id": n.logical_id,
            "name": n.name,
            "type": n.protocol,
            "server": n.server,
            "port": n.port,
            **n.normalized_payload,
        }
        for n in nodes
    ]

    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=node_dicts)
    return resolver.compile()


@router.get("/revisions", response_model=list[RevisionDTO])
async def list_revisions_endpoint(
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    db: AsyncSession = Depends(get_db),
) -> list[RevisionDTO]:
    """查询配置版本历史列表"""
    revisions = await list_revision_history(db, limit=limit, offset=offset)
    results: list[RevisionDTO] = []
    for r in revisions:
        bundle_data = r.bundle_json or {}
        groups = bundle_data.get("groups") or []
        rules = bundle_data.get("rules") or []
        results.append(
            RevisionDTO(
                logical_id=r.logical_id,
                schema_version=r.schema_version,
                checksum=r.checksum,
                is_active=r.is_active,
                created_at=r.created_at.isoformat() if r.created_at else "",
                created_by=r.created_by,
                group_count=len(groups),
                rule_count=len(rules),
            )
        )
    return results


@router.post("/revisions", response_model=RevisionDTO)
async def create_revision_endpoint(
    bundle: LogicalConfigurationBundle,
    created_by: str = Query("api_user"),
    db: AsyncSession = Depends(get_db),
) -> RevisionDTO:
    """创建并激活新配置版本"""
    try:
        rev = await create_and_activate_revision(db, bundle, created_by=created_by)
        await db.commit()
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc))

    return RevisionDTO(
        logical_id=rev.logical_id,
        schema_version=rev.schema_version,
        checksum=rev.checksum,
        is_active=rev.is_active,
        created_at=rev.created_at.isoformat() if rev.created_at else "",
        created_by=rev.created_by,
        group_count=len(bundle.groups),
        rule_count=len(bundle.rules),
    )


@router.post("/revisions/{logical_id}/rollback", response_model=RevisionDTO)
async def rollback_revision_endpoint(
    logical_id: str,
    db: AsyncSession = Depends(get_db),
) -> RevisionDTO:
    """回滚至指定历史配置版本"""
    try:
        rev = await rollback_to_revision(db, logical_id)
        await db.commit()
    except ValueError as exc:
        raise HTTPException(status_code=404, detail=str(exc))

    bundle_data = rev.bundle_json or {}
    return RevisionDTO(
        logical_id=rev.logical_id,
        schema_version=rev.schema_version,
        checksum=rev.checksum,
        is_active=rev.is_active,
        created_at=rev.created_at.isoformat() if rev.created_at else "",
        created_by=rev.created_by,
        group_count=len(bundle_data.get("groups") or []),
        rule_count=len(bundle_data.get("rules") or []),
    )


@router.get("/probe/profiles", response_model=list[ProbeProfileDTO])
async def list_probe_profiles_endpoint(
    db: AsyncSession = Depends(get_db),
) -> list[ProbeProfileDTO]:
    """查询探测 Profile 列表"""
    repo = ProbeProfileRepository(db)
    profiles = await repo.list_profiles()
    return [
        ProbeProfileDTO(
            id=p.id,
            logical_id=p.logical_id,
            name=p.name,
            platforms=p.platforms_json or [],
            timeout_s=p.timeout_s,
            concurrency=p.concurrency,
            max_bytes=p.max_bytes,
            min_speed_mbps=p.min_speed_mbps,
            created_at=p.created_at,
        )
        for p in profiles
    ]


@router.get("/probe/jobs", response_model=list[ProbeJobDTO])
async def list_probe_jobs_endpoint(
    limit: int = Query(50, ge=1, le=100),
    offset: int = Query(0, ge=0),
    db: AsyncSession = Depends(get_db),
) -> list[ProbeJobDTO]:
    """查询探测批任务记录列表"""
    repo = ProbeJobRepository(db)
    jobs = await repo.list_jobs(limit=limit, offset=offset)
    return [
        ProbeJobDTO(
            id=j.id,
            logical_id=j.logical_id,
            profile_id=j.profile_id,
            trigger_type=j.trigger_type,  # type: ignore[arg-type]
            status=j.status,  # type: ignore[arg-type]
            queued_at=j.queued_at,
            started_at=j.started_at,
            finished_at=j.finished_at,
            summary=j.summary_json,
        )
        for j in jobs
    ]


@router.get("/probe/observations", response_model=list[ProbeObservationDTO])
async def list_node_observations_endpoint(
    node_logical_id: str,
    limit: int = Query(20, ge=1, le=100),
    db: AsyncSession = Depends(get_db),
) -> list[ProbeObservationDTO]:
    """查询指定规范化节点的历史观测记录"""
    repo = ProbeObservationRepository(db)
    obs_list = await repo.list_observations_by_node_logical_id(node_logical_id, limit=limit)
    return [
        ProbeObservationDTO(
            id=o.id,
            job_id=o.job_id,
            node_id=o.node_id,
            status=o.status,  # type: ignore[arg-type]
            latency_ms=o.latency_ms,
            speed_mbps=o.speed_mbps,
            egress_ip=o.egress_ip,
            country=o.country,
            asn=o.asn,
            organization=o.organization,
            media_results=o.media_results_json or {},
            error=o.error,
            observed_at=o.observed_at,
        )
        for o in obs_list
    ]


@router.get("/readiness")
async def v2_readiness_endpoint(
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """全系统就绪状态检查，包含数据库迁移、激活配置与探测执行器"""
    db_ready = await check_database_readiness(db)
    active_rev = await get_active_revision(db)
    singbox_bin = shutil.which("sing-box")

    is_healthy = bool(db_ready.get("ready"))

    return {
        "healthy": is_healthy,
        "database": db_ready,
        "active_revision": {
            "present": active_rev is not None,
            "logical_id": active_rev.logical_id if active_rev else None,
            "checksum": active_rev.checksum if active_rev else None,
        },
        "probe_runner": {
            "available": singbox_bin is not None,
            "path": singbox_bin,
        },
    }
