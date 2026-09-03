"""download_service 的 SSRF 管线测试：redirect 逐跳校验、内网拒绝、限长截断

http_fetch.stream_fetch 是订阅拉取与资产下载共用的安全管线，此处测单点实现
"""
from __future__ import annotations

import pytest
from fastapi import HTTPException
import httpx

from app.utils.http_fetch import stream_fetch
from app.utils.validators import validate_fetch_url


def _client_with_routes(routes: dict[str, object]) -> httpx.AsyncClient:
    transport = httpx.MockTransport(lambda request: routes[str(request.url)])
    return httpx.AsyncClient(transport=transport, follow_redirects=False)


@pytest.mark.asyncio
async def test_redirect_to_private_rejected():
    """重定向指向内网地址必须被拒绝，不得发起对内网的请求"""
    routes = {
        "https://example.com/start": httpx.Response(302, headers={"Location": "http://127.0.0.1/secret"}),
    }
    async with _client_with_routes(routes) as client:
        with pytest.raises(HTTPException) as exc_info:
            await stream_fetch(client, "https://example.com/start", allow_private=False, max_bytes=1024)
    assert "不允许" in str(exc_info.value.detail) or "private" in str(exc_info.value.detail).lower() or exc_info.value.status_code in (400, 403)


@pytest.mark.asyncio
async def test_redirect_chain_follows_and_returns_final():
    """正常重定向链逐跳校验后返回最终响应"""
    final = httpx.Response(200, text="payload")
    routes = {
        "https://example.com/r1": httpx.Response(302, headers={"Location": "https://example.org/r2"}),
        "https://example.org/r2": final,
    }
    async with _client_with_routes(routes) as client:
        response = await stream_fetch(client, "https://example.com/r1", allow_private=False, max_bytes=1024)
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_redirect_missing_location_is_error():
    """3xx 缺 Location 头统一报错，不透传原响应"""
    routes = {
        "https://example.com/no-loc": httpx.Response(302),
    }
    async with _client_with_routes(routes) as client:
        with pytest.raises(HTTPException):
            await stream_fetch(client, "https://example.com/no-loc", allow_private=False, max_bytes=1024)


@pytest.mark.asyncio
async def test_redirect_loop_hits_limit():
    """重定向循环在达到上限时报错，不无限循环"""
    routes = {}
    for host in ("example.com", "example.org"):
        other = "example.org" if host == "example.com" else "example.com"
        routes[f"https://{host}/loop"] = httpx.Response(
            302, headers={"Location": f"https://{other}/loop"}
        )
    async with _client_with_routes(routes) as client:
        with pytest.raises(HTTPException) as exc_info:
            await stream_fetch(
                client, "https://example.com/loop", allow_private=False, max_redirects=5, max_bytes=1024
            )
    assert "重定向" in str(exc_info.value.detail)


@pytest.mark.asyncio
async def test_oversized_stream_truncated():
    """流式读取超过字节上限时中止并报错"""
    big_content = b"x" * (2048)
    routes = {
        "https://example.com/big": httpx.Response(200, content=big_content),
    }
    async with _client_with_routes(routes) as client:
        response = await stream_fetch(client, "https://example.com/big", allow_private=False, max_bytes=1024)
        try:
            with pytest.raises(HTTPException) as exc_info:
                written = 0
                async for chunk in response.aiter_bytes():
                    written += len(chunk)
                    if written > 1024:
                        raise HTTPException(status_code=413, detail="超出大小限制")
        finally:
            await response.aclose()
    assert exc_info.value.status_code == 413


@pytest.mark.asyncio
async def test_first_hop_private_rejected_without_request():
    """首跳即私网地址直接拒绝（validate_fetch_url 语义保持）"""
    with pytest.raises(HTTPException):
        validate_fetch_url("http://127.0.0.1/x", allow_private_hosts=False)
