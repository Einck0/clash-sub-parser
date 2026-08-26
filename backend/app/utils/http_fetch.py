"""统一的流式 HTTP 抓取管线：redirect 逐跳 SSRF 校验 + 字节限长 + 深度上限。

订阅拉取与资产下载共用此单点实现，保证两条路径的安全行为一致。
"""
from __future__ import annotations

from dataclasses import dataclass
from urllib.parse import urljoin

import httpx
from fastapi import HTTPException

from app.utils.validators import validate_fetch_url


@dataclass
class FetchResult:
    """stream_fetch 的返回：最终响应与实际读取字节数。"""

    response: httpx.Response
    final_url: str


async def stream_fetch(
    client: httpx.AsyncClient,
    url: str,
    *,
    allow_private: bool,
    max_redirects: int = 5,
    max_bytes: int | None = None,
) -> httpx.Response:
    """GET url 并手动跟随重定向，每一跳做 SSRF 校验。

    - 重定向缺 Location、超深度、目标不合法均抛 HTTPException
    - 返回的响应为流式，调用方负责 async with 关闭并自行限长读取
    """
    current_url = validate_fetch_url(url, allow_private_hosts=allow_private)
    for _ in range(max_redirects + 1):
        response = await client.send(client.build_request("GET", current_url), stream=True)
        if not response.is_redirect:
            return response
        location = response.headers.get("location")
        if not location:
            await response.aclose()
            raise HTTPException(status_code=502, detail="重定向缺少 Location 头")
        resolved = urljoin(str(response.url), location)
        try:
            next_url = validate_fetch_url(resolved, allow_private_hosts=allow_private)
        except HTTPException:
            await response.aclose()
            raise
        except Exception as exc:
            await response.aclose()
            raise HTTPException(status_code=400, detail=f"重定向目标不合法: {resolved}") from exc
        current_url = next_url
        await response.aclose()
        continue
    raise HTTPException(status_code=502, detail=f"重定向次数超过上限 {max_redirects}")
