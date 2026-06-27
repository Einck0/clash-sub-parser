import asyncio
import socket
import time
from fastapi import APIRouter
from pydantic import BaseModel

router = APIRouter(prefix="/latency", tags=["latency"])

class LatencyRequest(BaseModel):
    hosts: list[str]  # list of "host:port" strings
    timeout_ms: int = 3000

class LatencyResult(BaseModel):
    host: str
    port: int
    latency_ms: float | None = None
    error: str | None = None

async def _check_tcp(host: str, port: int, timeout: float) -> LatencyResult:
    try:
        start = time.monotonic()
        _, writer = await asyncio.wait_for(
            asyncio.open_connection(host, port),
            timeout=timeout,
        )
        elapsed = (time.monotonic() - start) * 1000
        writer.close()
        await writer.wait_closed()
        return LatencyResult(host=host, port=port, latency_ms=round(elapsed, 1))
    except asyncio.TimeoutError:
        return LatencyResult(host=host, port=port, error="timeout")
    except (OSError, socket.gaierror) as e:
        return LatencyResult(host=host, port=port, error=str(e)[:100])


async def _error_result(host: str, port: int, msg: str) -> LatencyResult:
    return LatencyResult(host=host, port=port, error=msg)


@router.post("/check", response_model=list[LatencyResult])
async def check_latency(payload: LatencyRequest):
    timeout = payload.timeout_ms / 1000
    tasks = []
    for item in payload.hosts[:50]:  # max 50 per batch
        parts = item.rsplit(":", 1)
        if len(parts) != 2:
            tasks.append(_error_result(item, 0, "invalid format"))
            continue
        host, port_str = parts
        try:
            port = int(port_str)
        except ValueError:
            tasks.append(_error_result(host, 0, "invalid port"))
            continue
        tasks.append(_check_tcp(host, port, timeout))

    results = await asyncio.gather(*tasks, return_exceptions=True)
    output = []
    for r in results:
        if isinstance(r, LatencyResult):
            output.append(r)
        elif isinstance(r, Exception):
            output.append(LatencyResult(host="unknown", port=0, error=str(r)[:100]))
    return output
