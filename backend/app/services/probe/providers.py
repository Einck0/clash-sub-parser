"""模块化探测器：连通性、出口地理位置、流媒体与 AI 解锁与受控带宽测速

所有探测请求显式通过指定的本地回环代理（trust_env=False）
严禁继承或受到宿主机环境代理污染
"""
from __future__ import annotations

import asyncio
import re
import time
from typing import Any

import httpx

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
    timeout_s: float = 5.0,
) -> dict[str, Any]:
    """检测节点代理握手与基础连通性，测量往返延迟 (ms)"""
    started = time.monotonic()
    per_url_timeout = max(2.0, min(timeout_s, 5.0))
    for target_url in TRANSPORT_204_URLS:
        try:
            async with httpx.AsyncClient(
                proxy=proxy_url,
                trust_env=False,
                timeout=per_url_timeout,
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
        except Exception:
            continue

    elapsed_ms = int((time.monotonic() - started) * 1000)
    return {
        "status": "fail",
        "latency_ms": elapsed_ms,
        "target": TRANSPORT_204_URLS[0],
        "error": "handshake or transport timeout",
    }


async def check_geo_identity(
    proxy_url: str,
    timeout_s: float = 4.0,
) -> dict[str, Any]:
    """检测经节点出站的真实出口 IP、国家代码与 ASN"""
    try:
        async with httpx.AsyncClient(
            proxy=proxy_url,
            trust_env=False,
            timeout=timeout_s,
            headers={"User-Agent": DEFAULT_UA},
        ) as client:
            resp = await client.get("https://api.ip.sb/geoip")
            if resp.status_code == 200:
                data = resp.json()
                return {
                    "ip": data.get("ip") or "",
                    "country": str(data.get("country_code") or "").upper(),
                    "asn": data.get("asn"),
                    "organization": data.get("organization") or "",
                }
    except Exception:
        pass

    # 备用方案 Cloudflare trace
    try:
        async with httpx.AsyncClient(
            proxy=proxy_url,
            trust_env=False,
            timeout=timeout_s,
            headers={"User-Agent": DEFAULT_UA},
        ) as client:
            resp = await client.get("https://cloudflare.com/cdn-cgi/trace")
            if resp.status_code == 200:
                lines = resp.text.splitlines()
                trace_dict = {}
                for line in lines:
                    if "=" in line:
                        k, v = line.split("=", 1)
                        trace_dict[k.strip()] = v.strip()
                return {
                    "ip": trace_dict.get("ip") or "",
                    "country": str(trace_dict.get("loc") or "").upper(),
                    "asn": None,
                    "organization": "Cloudflare Egress",
                }
    except Exception as exc:
        return {
            "ip": "",
            "country": "",
            "asn": None,
            "organization": "",
            "error": str(exc),
        }

    return {"ip": "", "country": "", "asn": None, "organization": ""}


async def check_youtube(client: httpx.AsyncClient) -> dict[str, Any]:
    """检测 YouTube Premium 解锁状态与地区"""
    try:
        resp = await client.get("https://www.youtube.com/premium")
        if resp.status_code == 200:
            text = resp.text
            if "Premium is not available in your country" in text or "YouTube Premium 在你所在的国家/地区不可用" in text:
                return {"status": "blocked", "region": None}
            match = re.search(r'"countryCode":\s*"([A-Z]{2})"', text)
            region = match.group(1) if match else None
            return {"status": "ok", "region": region}
        return {"status": "fail", "code": resp.status_code}
    except Exception as exc:
        return {"status": "fail", "error": str(exc)}


async def check_netflix(client: httpx.AsyncClient) -> dict[str, Any]:
    """检测 Netflix 解锁状态（Full 完整片库 或 Originals 仅自制剧 或 Blocked 被阻断）"""
    try:
        # 非自制版权剧（Breaking Bad）
        resp_full = await client.get("https://www.netflix.com/title/70143836")
        if resp_full.status_code == 200:
            return {"status": "full", "label": "原生全解锁"}

        # 自制剧（Stranger Things）
        resp_orig = await client.get("https://www.netflix.com/title/80018499")
        if resp_orig.status_code == 200:
            return {"status": "originals", "label": "仅自制剧"}

        if resp_full.status_code in (403, 404) or "title" not in str(resp_full.url):
            return {"status": "blocked", "label": "未解锁"}

        return {"status": "unknown", "label": "未知"}
    except Exception as exc:
        return {"status": "fail", "error": str(exc)}


async def check_disney(client: httpx.AsyncClient) -> dict[str, Any]:
    """检测 Disney+ 解锁状态"""
    try:
        resp = await client.get("https://www.disneyplus.com/")
        if resp.status_code in (200, 301, 302):
            loc = str(resp.headers.get("location") or "")
            if "unavailable" in loc or "preview" in loc:
                return {"status": "blocked", "label": "未支持地区"}
            return {"status": "ok", "label": "解锁"}
        return {"status": "blocked", "code": resp.status_code}
    except Exception as exc:
        return {"status": "fail", "error": str(exc)}


async def check_chatgpt(client: httpx.AsyncClient) -> dict[str, Any]:
    """检测 OpenAI 或 ChatGPT 出口访问权限"""
    try:
        resp = await client.get("https://ios.chat.openai.com/public-api/mobile/server_status/v1")
        if resp.status_code == 200:
            return {"status": "ok", "label": "允许访问"}
        if resp.status_code in (403, 1020):
            return {"status": "blocked", "label": "被风控或阻断"}
        return {"status": "unknown", "code": resp.status_code}
    except Exception as exc:
        return {"status": "fail", "error": str(exc)}


async def check_bilibili(client: httpx.AsyncClient) -> dict[str, Any]:
    """检测 Bilibili 港澳台与大陆限定区域解锁"""
    try:
        # 港澳台限定番剧测试
        resp = await client.get("https://api.bilibili.com/pgc/player/web/v2/playurl?ep_id=268176")
        if resp.status_code == 200:
            data = resp.json()
            code = data.get("code")
            if code == 0:
                return {"status": "ok", "region": "港澳台地区"}
            elif code == -10403:
                return {"status": "mainland", "region": "CN"}
        return {"status": "unknown"}
    except Exception as exc:
        return {"status": "fail", "error": str(exc)}


async def check_meta_ai(client: httpx.AsyncClient) -> dict[str, Any]:
    """检测 Meta AI (meta.ai 或 Imagine) 地区访问权限"""
    try:
        resp = await client.get("https://www.meta.ai/")
        if resp.status_code == 200:
            text = resp.text
            if "Meta AI isn't available yet in your country" in text or "not available in your country" in text:
                return {"status": "blocked", "label": "地区不可用"}
            return {"status": "ok", "label": "可用"}
        if resp.status_code in (403, 301, 302):
            loc = str(resp.headers.get("location") or "")
            if "unavailable" in loc:
                return {"status": "blocked", "label": "未开放地区"}
            return {"status": "ok", "label": "可用"}
        return {"status": "unknown", "code": resp.status_code}
    except Exception as exc:
        return {"status": "fail", "error": str(exc)}


async def check_gemini(client: httpx.AsyncClient) -> dict[str, Any]:
    """检测 Google Gemini (gemini.google.com) 出口访问权限"""
    try:
        resp = await client.get("https://gemini.google.com/")
        if resp.status_code in (200, 302, 303):
            loc = str(resp.headers.get("location") or "")
            if "unavailable" in loc or "sorry" in str(resp.url):
                return {"status": "blocked", "label": "未支持地区"}
            if "Gemini isn't currently supported in your country" in resp.text:
                return {"status": "blocked", "label": "未支持地区"}
            return {"status": "ok", "label": "可用"}
        if resp.status_code == 403:
            return {"status": "blocked", "label": "被风控或阻断"}
        return {"status": "unknown", "code": resp.status_code}
    except Exception as exc:
        return {"status": "fail", "error": str(exc)}


PROBE_MEDIA_DISPATCH = {
    "youtube": check_youtube,
    "netflix": check_netflix,
    "disney": check_disney,
    "chatgpt": check_chatgpt,
    "bilibili": check_bilibili,
    "meta_ai": check_meta_ai,
    "gemini": check_gemini,
}


async def check_media_unlock(
    proxy_url: str,
    platforms: list[str],
    timeout_s: float = 6.0,
) -> dict[str, Any]:
    """针对指定流媒体与 AI 平台执行出站能力检测（并行发起）"""
    results: dict[str, Any] = {}
    valid_platforms = [p for p in platforms if p.lower().strip() in PROBE_MEDIA_DISPATCH]
    if not valid_platforms:
        return results

    async with httpx.AsyncClient(
        proxy=proxy_url,
        trust_env=False,
        timeout=timeout_s,
        follow_redirects=True,
        headers={"User-Agent": DEFAULT_UA},
    ) as client:
        async def _check_one(p_name: str) -> tuple[str, dict[str, Any]]:
            fn = PROBE_MEDIA_DISPATCH[p_name.lower().strip()]
            try:
                res = await fn(client)
                return p_name, res
            except Exception as exc:
                return p_name, {"status": "fail", "error": str(exc)}

        gathered = await asyncio.gather(*[_check_one(p) for p in valid_platforms], return_exceptions=True)
        for item in gathered:
            if isinstance(item, tuple):
                p_name, res = item
                results[p_name] = res
    return results


async def check_download_speed(
    proxy_url: str,
    speedtest_url: str = "https://speed.cloudflare.com/__down?bytes=5000000",
    max_bytes: int = 5242880,  # 5MB 流量上限
    timeout_s: float = 5.0,
) -> dict[str, Any]:
    """执行受控小样本带宽测速，返回峰值与平均带宽 (Mbps)"""
    started = time.monotonic()
    total_bytes = 0
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
                        break

        duration_s = max(time.monotonic() - started, 0.05)
        # 1 Byte = 8 bits
        speed_mbps = round((total_bytes * 8) / (duration_s * 1_000_000), 2)
        return {
            "status": "ok",
            "speed_mbps": speed_mbps,
            "bytes": total_bytes,
            "duration_s": round(duration_s, 2),
            "error": None,
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
