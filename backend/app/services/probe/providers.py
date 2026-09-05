"""模块化探测器：连通性、出口地理位置、流媒体与 AI 解锁与受控带宽测速

所有探测请求显式通过指定的本地回环代理（trust_env=False）
严禁继承或受到宿主机环境代理污染
"""
from __future__ import annotations

import asyncio
import inspect
import time
from typing import Any

import httpx

from app.services.probe.catalogue import (
    EVIDENCE_VERSION,
    eval_aistudio,
    eval_bilibili,
    eval_chatgpt,
    eval_disney,
    eval_gemini,
    eval_geo_identity_consensus,
    eval_meta_ai,
    eval_netflix,
    eval_youtube,
    eval_youtube_cdn,
)

TRANSPORT_204_URLS = [
    "http://cp.cloudflare.com/generate_204",
    "http://www.gstatic.com/generate_204",
    "https://cp.cloudflare.com/generate_204",
    "https://www.google.com/generate_204",
]

DEFAULT_UA = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)


async def check_transport(
    proxy_url: str,
    timeout_s: float = 2.0,
) -> dict[str, Any]:
    """检测节点代理握手与基础连通性，测量往返延迟 (ms)"""
    started = time.monotonic()
    timed_out = False
    for target_url in TRANSPORT_204_URLS:
        remaining = timeout_s - (time.monotonic() - started)
        if remaining <= 0:
            timed_out = True
            break
        try:
            async with httpx.AsyncClient(
                proxy=proxy_url,
                trust_env=False,
                timeout=min(remaining, 5.0),
                follow_redirects=False,
            ) as client:
                resp = await client.get(target_url)
                if resp.status_code in (200, 204):
                    elapsed_ms = int((time.monotonic() - started) * 1000)
                    return {
                        "status": "ok",
                        "latency_ms": elapsed_ms,
                        "target": target_url,
                        "error": None,
                    }
        except (asyncio.TimeoutError, httpx.TimeoutException):
            if (time.monotonic() - started) >= timeout_s:
                timed_out = True
                break
            continue
        except Exception:
            continue

    elapsed_ms = int((time.monotonic() - started) * 1000)
    is_timeout = timed_out or (time.monotonic() - started >= timeout_s)
    return {
        "status": "timeout" if is_timeout else "fail",
        "latency_ms": None if is_timeout else elapsed_ms,
        "target": TRANSPORT_204_URLS[0],
        "error": "handshake or transport timeout" if is_timeout else "transport handshake failed",
    }


async def check_geo_identity(
    proxy_url: str,
    timeout_s: float = 2.0,
) -> dict[str, Any]:
    """检测经节点出站的真实出口 IP、国家代码与 ASN（基于多 Provider 共识）"""
    obs = await eval_geo_identity_consensus(proxy_url, timeout_s=timeout_s)
    return obs.to_dict()


async def _call_eval(fn: Any, client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> Any:
    sig = inspect.signature(fn)
    kwargs: dict[str, Any] = {}
    if "timeout_s" in sig.parameters or any(p.kind == inspect.Parameter.VAR_KEYWORD for p in sig.parameters.values()):
        kwargs["timeout_s"] = timeout_s
    if deadline_monotonic is not None and (
        "deadline_monotonic" in sig.parameters or any(p.kind == inspect.Parameter.VAR_KEYWORD for p in sig.parameters.values())
    ):
        kwargs["deadline_monotonic"] = deadline_monotonic
    try:
        res = await fn(client, **kwargs)
    except TypeError:
        try:
            res = await fn(client, timeout_s=timeout_s)
        except TypeError:
            res = await fn(client)
    return res.to_dict() if hasattr(res, "to_dict") else res


async def check_youtube(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 YouTube Premium 解锁状态与地区"""
    return await _call_eval(eval_youtube, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


async def check_netflix(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 Netflix 解锁状态（Full 完整片库 或 Originals 仅自制剧 或 Blocked 被阻断）"""
    return await _call_eval(eval_netflix, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


async def check_disney(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 Disney+ 解锁状态"""
    return await _call_eval(eval_disney, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


async def check_chatgpt(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 OpenAI 或 ChatGPT 出口访问权限"""
    return await _call_eval(eval_chatgpt, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


async def check_bilibili(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 Bilibili 港澳台与大陆限定区域解锁"""
    return await _call_eval(eval_bilibili, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


async def check_meta_ai(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 Meta AI (meta.ai 或 Imagine) 地区访问权限"""
    return await _call_eval(eval_meta_ai, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


async def check_gemini(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 Google Gemini (gemini.google.com) 出口访问权限"""
    return await _call_eval(eval_gemini, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


async def check_aistudio(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 Google AI Studio (generativelanguage.googleapis.com) 出口访问权限"""
    return await _call_eval(eval_aistudio, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


async def check_youtube_cdn(client: httpx.AsyncClient, timeout_s: float = 2.0, deadline_monotonic: float | None = None) -> dict[str, Any]:
    """检测 YouTube CDN mapping 路由提示（独立于 exit identity）"""
    return await _call_eval(eval_youtube_cdn, client, timeout_s=timeout_s, deadline_monotonic=deadline_monotonic)


PROBE_MEDIA_DISPATCH = {
    "youtube": check_youtube,
    "netflix": check_netflix,
    "disney": check_disney,
    "chatgpt": check_chatgpt,
    "bilibili": check_bilibili,
    "meta_ai": check_meta_ai,
    "gemini": check_gemini,
    "aistudio": check_aistudio,
    "youtube_cdn": check_youtube_cdn,
}


async def check_media_unlock(
    proxy_url: str,
    platforms: list[str],
    timeout_s: float = 2.0,
) -> dict[str, Any]:
    """针对指定流媒体与 AI 平台执行出站能力检测（单节点共享单个 HTTP 连接池，各平台并发发起）"""
    results: dict[str, Any] = {}
    valid_platforms = [p for p in platforms if p.lower().strip() in PROBE_MEDIA_DISPATCH]
    if not valid_platforms:
        return results

    if timeout_s <= 0:
        for p in valid_platforms:
            results[p] = {
                "status": "timeout",
                "verdict": "unknown",
                "unlocked": False,
                "confidence": "unavailable",
                "evidence": {
                    "http_status": None,
                    "signals": [],
                    "elapsed_ms": 0,
                    "error_code": "timeout",
                },
                "error": f"{p} probe timed out",
            }
        return results

    stage_deadline = time.monotonic() + timeout_s

    async def _check_one(p_name: str, client: httpx.AsyncClient) -> tuple[str, dict[str, Any]]:
        remaining = stage_deadline - time.monotonic()
        if remaining <= 0:
            return p_name, {
                "status": "timeout",
                "verdict": "unknown",
                "unlocked": False,
                "confidence": "unavailable",
                "evidence": {
                    "http_status": None,
                    "signals": [],
                    "elapsed_ms": int(timeout_s * 1000),
                    "error_code": "timeout",
                },
                "error": f"{p_name} probe timed out",
            }

        fn = PROBE_MEDIA_DISPATCH[p_name.lower().strip()]
        try:
            sig = inspect.signature(fn)
            kwargs: dict[str, Any] = {}
            if "timeout_s" in sig.parameters:
                kwargs["timeout_s"] = remaining
            if "deadline_monotonic" in sig.parameters:
                kwargs["deadline_monotonic"] = stage_deadline
            coro = fn(client, **kwargs)
            res = await asyncio.wait_for(coro, timeout=max(0.01, remaining))
            return p_name, res
        except (asyncio.TimeoutError, httpx.TimeoutException):
            return p_name, {
                "status": "timeout",
                "verdict": "unknown",
                "unlocked": False,
                "confidence": "unavailable",
                "evidence": {
                    "http_status": None,
                    "signals": [],
                    "elapsed_ms": int(timeout_s * 1000),
                    "error_code": "timeout",
                },
                "error": f"{p_name} probe timed out",
            }
        except Exception as exc:
            return p_name, {
                "status": "transport_error",
                "verdict": "unknown",
                "unlocked": False,
                "confidence": "unavailable",
                "evidence": {
                    "http_status": None,
                    "signals": [],
                    "elapsed_ms": 0,
                    "error_code": "transport_error",
                },
                "error": str(exc),
            }

    async with httpx.AsyncClient(
        proxy=proxy_url,
        trust_env=False,
        follow_redirects=True,
        headers={"User-Agent": DEFAULT_UA},
    ) as client:
        gathered = await asyncio.gather(
            *[_check_one(p, client) for p in valid_platforms],
            return_exceptions=True,
        )
        for idx, item in enumerate(gathered):
            if isinstance(item, tuple):
                p_name, res = item
                results[p_name] = res
            elif isinstance(item, Exception):
                p_name = valid_platforms[idx]
                results[p_name] = {
                    "status": "transport_error",
                    "verdict": "unknown",
                    "unlocked": False,
                    "confidence": "unavailable",
                    "evidence": {
                        "http_status": None,
                        "signals": [],
                        "elapsed_ms": 0,
                        "error_code": "transport_error",
                    },
                    "error": str(item),
                }

    return results


async def check_download_speed(
    proxy_url: str,
    speedtest_url: str = "https://speed.cloudflare.com/__down?bytes=5000000",
    max_bytes: int = 5242880,  # 5MB 流量上限
    timeout_s: float = 2.0,
) -> dict[str, Any]:
    """执行受控小样本带宽测速，返回峰值与平均带宽 (Mbps)"""
    started = time.monotonic()
    total_bytes = 0
    timed_out = False
    try:
        async with httpx.AsyncClient(
            proxy=proxy_url,
            trust_env=False,
            timeout=timeout_s,
        ) as client:
            async with client.stream("GET", speedtest_url) as response:
                if response.status_code not in (200, 206):
                    return {
                        "status": "fail",
                        "speed_mbps": 0.0,
                        "bytes": 0,
                        "error": f"HTTP {response.status_code}",
                    }
                async for chunk in response.aiter_bytes():
                    total_bytes += len(chunk)
                    if total_bytes >= max_bytes:
                        break
                    if time.monotonic() - started >= timeout_s:
                        timed_out = True
                        break

        duration_s = max(time.monotonic() - started, 0.05)
        # 1 Byte = 8 bits
        speed_mbps = round((total_bytes * 8) / (duration_s * 1_000_000), 2)
        if timed_out or (time.monotonic() - started >= timeout_s):
            return {
                "status": "timeout",
                "speed_mbps": speed_mbps,
                "bytes": total_bytes,
                "duration_s": round(duration_s, 2),
                "error": "speed test timed out",
            }
        return {
            "status": "ok",
            "speed_mbps": speed_mbps,
            "bytes": total_bytes,
            "duration_s": round(duration_s, 2),
            "error": None,
        }
    except (asyncio.TimeoutError, httpx.TimeoutException):
        duration_s = max(time.monotonic() - started, 0.05)
        speed_mbps = round((total_bytes * 8) / (duration_s * 1_000_000), 2) if total_bytes > 0 else 0.0
        return {
            "status": "timeout",
            "speed_mbps": speed_mbps,
            "bytes": total_bytes,
            "duration_s": round(duration_s, 2),
            "error": "speed test timed out",
        }
    except Exception as exc:
        duration_s = max(time.monotonic() - started, 0.05)
        speed_mbps = round((total_bytes * 8) / (duration_s * 1_000_000), 2) if total_bytes > 0 else 0.0
        return {
            "status": "fail" if total_bytes == 0 else "partial",
            "speed_mbps": speed_mbps,
            "bytes": total_bytes,
            "duration_s": round(duration_s, 2),
            "error": str(exc),
        }
