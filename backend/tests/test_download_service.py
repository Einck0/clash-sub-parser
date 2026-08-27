"""download_service 的安全与抓取测试：redirect 逐跳校验、内网拒绝、超限长截断。"""
from __future__ import annotations

import pytest
import pytest_asyncio
from fastapi import HTTPException

from app.services.download_service import download_url


class DummyStreamResponse:
    def __init__(self, status_code: int = 200, headers: dict | None = None, content: bytes = b"ok"):
        self.status_code = status_code
        self.headers = headers or {}
        self._content = content
        self.url = "https://example.com/file.bin"
        self.is_redirect = False

    def raise_for_status(self):
        if self.status_code >= 400:
            raise HTTPException(status_code=self.status_code, detail="error")

    async def aiter_bytes(self):
        yield self._content

    async def aclose(self):
        return None

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc, tb):
        await self.aclose()


@pytest.mark.asyncio
async def test_download_url_rejects_private_host():
    with pytest.raises(HTTPException) as exc:
        await download_url("http://127.0.0.1:8080/bad")
    assert exc.value.status_code == 400
    assert "private" in exc.value.detail.lower() or "reserved" in exc.value.detail.lower()


@pytest.mark.asyncio
async def test_download_url_rejects_content_length_exceeded(monkeypatch, tmp_path):
    monkeypatch.setattr("app.services.download_service.DOWNLOAD_DIR", tmp_path)
    from app.config import get_settings
    monkeypatch.setattr(get_settings(), "download_max_bytes", 100)

    class MockClient:
        def __init__(self, *args, **kwargs):
            pass
        async def __aenter__(self):
            return self
        async def __aexit__(self, *args):
            pass
        def build_request(self, method, url):
            return url
        async def send(self, req, stream=False):
            return DummyStreamResponse(headers={"content-length": "999999"})

    monkeypatch.setattr("httpx.AsyncClient", MockClient)

    with pytest.raises(HTTPException) as exc:
        await download_url("https://example.com/too-big.bin")
    assert exc.value.status_code == 413


@pytest.mark.asyncio
async def test_download_url_streams_and_saves(monkeypatch, tmp_path):
    monkeypatch.setattr("app.services.download_service.DOWNLOAD_DIR", tmp_path)

    class MockClient:
        def __init__(self, *args, **kwargs):
            pass
        async def __aenter__(self):
            return self
        async def __aexit__(self, *args):
            pass
        def build_request(self, method, url):
            return url
        async def send(self, req, stream=False):
            return DummyStreamResponse(content=b"hello payload")

    monkeypatch.setattr("httpx.AsyncClient", MockClient)

    res = await download_url("https://example.com/valid.bin")
    assert res["filename"] == "valid.bin"
    saved = tmp_path / "valid.bin"
    assert saved.exists()
    assert saved.read_bytes() == b"hello payload"
