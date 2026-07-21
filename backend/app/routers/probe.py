"""TCP reachability probe endpoints (not proxy latency)."""

from __future__ import annotations

from typing import Any

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field

from app.services.tcp_probe_service import (
    DEFAULT_CONCURRENCY,
    DEFAULT_TIMEOUT_MS,
    MAX_NODES,
    probe_nodes,
)

router = APIRouter(prefix="/probe", tags=["probe"])


class ProbeRequest(BaseModel):
    nodes: list[dict[str, Any]] = Field(default_factory=list)
    timeout_ms: int = DEFAULT_TIMEOUT_MS
    concurrency: int = DEFAULT_CONCURRENCY


@router.post("/tcp")
async def probe_tcp(payload: ProbeRequest) -> dict[str, Any]:
    """Probe server:port TCP connectivity for a node list.

    This does not speak SS/VMess/VLESS/Hysteria. Open port != usable proxy.
    """
    if not payload.nodes:
        raise HTTPException(status_code=400, detail="nodes is required")
    if len(payload.nodes) > MAX_NODES:
        raise HTTPException(
            status_code=400,
            detail=f"Too many nodes (max {MAX_NODES})",
        )
    return await probe_nodes(
        payload.nodes,
        timeout_ms=payload.timeout_ms,
        concurrency=payload.concurrency,
    )
