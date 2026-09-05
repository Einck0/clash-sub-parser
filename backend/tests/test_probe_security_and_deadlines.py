"""Tests for probe security, deadline enforcement, bounded retry discipline, and host proxy isolation.

Verifies loopback proxy pinning with trust_env=False, bounded retry (at most 1
retry for transient transport failure only; 0 retries for restriction/block/challenge/429/drift),
and service deadline enforcement preserving sibling results.
"""
from __future__ import annotations

import asyncio
from typing import Any
from unittest.mock import AsyncMock, patch
import httpx
import pytest

from app.schemas.probe import ResolvedProbeConfig
from app.services.probe.catalogue import (
    eval_chatgpt,
    eval_netflix,
    execute_request_with_retry,
)
from app.services.probe.providers import check_media_unlock, check_transport
from app.services.probe.service import probe_single_node


@pytest.mark.asyncio
async def test_fake_host_proxy_environment_isolation(monkeypatch):
    """Host environment proxy variables must NOT contaminate probe requests."""
    fake_host_proxy = "http://10.255.255.254:9999"
    monkeypatch.setenv("HTTP_PROXY", fake_host_proxy)
    monkeypatch.setenv("HTTPS_PROXY", fake_host_proxy)
    monkeypatch.setenv("ALL_PROXY", fake_host_proxy)
    monkeypatch.setenv("http_proxy", fake_host_proxy)
    monkeypatch.setenv("https_proxy", fake_host_proxy)
    monkeypatch.setenv("all_proxy", fake_host_proxy)

    # Inspect httpx.AsyncClient construction in check_transport
    original_client_init = httpx.AsyncClient.__init__
    captured_clients = []

    def mock_init(self, *args, **kwargs):
        captured_clients.append(kwargs)
        original_client_init(self, *args, **kwargs)

    with patch.object(httpx.AsyncClient, "__init__", side_effect=mock_init, autospec=True):
        # Even if request fails because loopback proxy is not live, inspect the client options
        await check_transport("http://127.0.0.1:21000", timeout_s=0.2)

    assert len(captured_clients) > 0
    for client_kwargs in captured_clients:
        assert client_kwargs.get("trust_env") is False, "trust_env must be False to prevent host proxy leakage"
        assert str(client_kwargs.get("proxy")) == "http://127.0.0.1:21000"


@pytest.mark.asyncio
async def test_transient_transport_error_retries_at_most_once():
    """Transient network errors (e.g. ConnectError) retry at most once within deadline."""
    client = AsyncMock(spec=httpx.AsyncClient)
    call_count = 0

    async def mock_get(*args, **kwargs):
        nonlocal call_count
        call_count += 1
        if call_count == 1:
            raise httpx.ConnectError("Transient connection reset")
        req = httpx.Request("GET", "https://example.com")
        return httpx.Response(200, text="success", request=req)

    client.get = AsyncMock(side_effect=mock_get)

    import time
    deadline = time.monotonic() + 2.0
    resp = await execute_request_with_retry(client, "GET", "https://example.com", deadline_monotonic=deadline)
    assert resp.status_code == 200
    assert call_count == 2, "Must retry exactly once for transient transport error"


@pytest.mark.asyncio
async def test_definitive_responses_never_retry():
    """Definitive HTTP outcomes (403, 404, 429, restriction, drift) must NEVER retry."""
    # 1. 403 Challenge
    client_403 = AsyncMock(spec=httpx.AsyncClient)
    calls_403 = 0
    async def mock_403(*args, **kwargs):
        nonlocal calls_403
        calls_403 += 1
        req = httpx.Request("GET", "https://ios.chat.openai.com/public-api/mobile/server_status/v1")
        return httpx.Response(403, text="Forbidden", request=req)
    client_403.get = AsyncMock(side_effect=mock_403)

    res_403 = await eval_chatgpt(client_403)
    assert res_403.status == "challenged"
    assert calls_403 == 1, "403 must not retry"

    # 2. 429 Rate limited
    client_429 = AsyncMock(spec=httpx.AsyncClient)
    calls_429 = 0
    async def mock_429(*args, **kwargs):
        nonlocal calls_429
        calls_429 += 1
        req = httpx.Request("GET", "https://ios.chat.openai.com/public-api/mobile/server_status/v1")
        return httpx.Response(429, text="Too Many Requests", request=req)
    client_429.get = AsyncMock(side_effect=mock_429)

    res_429 = await eval_chatgpt(client_429)
    assert res_429.status == "rate_limited"
    assert calls_429 == 1, "429 must not retry"

    # 3. Contract drift (200 with unknown body)
    client_drift = AsyncMock(spec=httpx.AsyncClient)
    calls_drift = 0
    async def mock_drift(*args, **kwargs):
        nonlocal calls_drift
        calls_drift += 1
        req = httpx.Request("GET", "https://ios.chat.openai.com/public-api/mobile/server_status/v1")
        return httpx.Response(200, text='{"unrecognized": true}', request=req)
    client_drift.get = AsyncMock(side_effect=mock_drift)

    res_drift = await eval_chatgpt(client_drift)
    assert res_drift.status == "inconclusive"
    assert calls_drift == 1, "Contract drift must not retry"


@pytest.mark.asyncio
async def test_service_deadline_enforcement_and_timeout():
    """When wall-clock deadline expires, return timeout and stop retries."""
    client = AsyncMock(spec=httpx.AsyncClient)

    async def mock_slow_get(*args, **kwargs):
        await asyncio.sleep(0.5)
        raise httpx.ConnectTimeout("Timed out")

    client.get = AsyncMock(side_effect=mock_slow_get)

    res = await eval_netflix(client, timeout_s=0.1)
    assert res.status == "timeout"
    assert res.verdict == "unknown"
    assert res.unlocked is False


@pytest.mark.asyncio
async def test_sibling_result_preservation_in_media_check():
    """A timeout or failure in one provider must not abort or discard sibling results."""
    mock_runner_url = "http://127.0.0.1:21000"

    async def mock_eval_dispatch(client):
        # inspect client url
        url_str = str(client)
        return {}

    # Mock individual evaluators: youtube succeeds, netflix times out
    async def mock_yt(client, timeout_s=2.0):
        from app.services.probe.models import ProviderResult
        return ProviderResult(
            status="verified",
            verdict="available",
            unlocked=True,
            region="US",
            checked_at=1788546780,
            evidence_version="catalogue-2026-09-05",
            confidence="verified",
            evidence={"http_status": 200, "signals": ["premium_available"], "elapsed_ms": 50},
        )

    async def mock_nf(client, timeout_s=2.0):
        from app.services.probe.models import ProviderResult
        return ProviderResult(
            status="timeout",
            verdict="unknown",
            unlocked=False,
            region=None,
            checked_at=1788546780,
            evidence_version="catalogue-2026-09-05",
            confidence="unavailable",
            evidence={"http_status": None, "signals": [], "elapsed_ms": 2000, "error_code": "timeout"},
            error="netflix probe timed out",
        )

    with patch("app.services.probe.providers.eval_youtube", side_effect=mock_yt), \
         patch("app.services.probe.providers.eval_netflix", side_effect=mock_nf):

        res = await check_media_unlock(mock_runner_url, platforms=["youtube", "netflix"], timeout_s=2.0)
        assert "youtube" in res
        assert "netflix" in res
        assert res["youtube"]["status"] == "verified"
        assert res["youtube"]["unlocked"] is True
        assert res["netflix"]["status"] == "timeout"
        assert res["netflix"]["unlocked"] is False


@pytest.mark.asyncio
async def test_node_liveness_authority_preserved_on_platform_failure():
    """Transport success is liveness authority; platform failures must not downgrade node to failed."""
    node = {
        "name": "resilient-node",
        "server": "1.2.3.4",
        "port": 443,
        "type": "ss",
        "password": "pass",
        "cipher": "aes-128-gcm",
    }

    mock_runner_ctx = {"proxy_url": "http://127.0.0.1:21000", "port": 21000, "is_runner": True}
    from contextlib import asynccontextmanager
    @asynccontextmanager
    async def mock_spawn(n, **kwargs):
        yield mock_runner_ctx

    mock_transport = {"status": "ok", "latency_ms": 45, "target": "https://cp.cloudflare.com/generate_204", "error": None}
    mock_geo = {"status": "ok", "ip": "1.2.3.4", "country": "US", "asn": 13335, "organization": "Cloudflare"}

    async def mock_failing_media(proxy_url, platforms, timeout_s=2.0):
        return {
            "youtube": {"status": "timeout", "unlocked": False, "error": "timeout"},
            "netflix": {"status": "transport_error", "unlocked": False, "error": "connection refused"},
        }

    with patch("app.services.probe.service.spawn_node_runner", side_effect=mock_spawn), \
         patch("app.services.probe.service.check_transport", return_value=mock_transport), \
         patch("app.services.probe.service.check_geo_identity", return_value=mock_geo), \
         patch("app.services.probe.service.check_media_unlock", side_effect=mock_failing_media):

        resolved = ResolvedProbeConfig(service_timeout_s=2.0, media_platforms=("youtube", "netflix"))
        res = await probe_single_node(node, resolved_config=resolved, use_cache=False)

        # Node status MUST remain ok because transport handshake succeeded!
        assert res["status"] == "ok"
        assert res["latency_ms"] == 45
        assert res["media"]["youtube"]["status"] == "timeout"
        assert res["media"]["netflix"]["status"] == "transport_error"


@pytest.mark.asyncio
async def test_shared_media_session_constructs_single_client():
    """One healthy node probing multiple media platforms must create exactly ONE AsyncClient."""
    mock_runner_url = "http://127.0.0.1:21000"
    created_clients: list[dict[str, Any]] = []
    original_init = httpx.AsyncClient.__init__

    def intercept_init(self, *args, **kwargs):
        created_clients.append({
            "proxy": kwargs.get("proxy"),
            "trust_env": kwargs.get("trust_env"),
            "follow_redirects": kwargs.get("follow_redirects"),
            "headers": kwargs.get("headers"),
        })
        original_init(self, *args, **kwargs)

    async def mock_eval_fn(client, timeout_s=2.0, deadline_monotonic=None):
        from app.services.probe.models import ProviderResult
        return ProviderResult(
            status="verified",
            verdict="available",
            unlocked=True,
            evidence={"http_status": 200, "signals": [], "elapsed_ms": 10},
        )

    with patch.object(httpx.AsyncClient, "__init__", side_effect=intercept_init, autospec=True), \
         patch("app.services.probe.providers.eval_youtube", side_effect=mock_eval_fn), \
         patch("app.services.probe.providers.eval_netflix", side_effect=mock_eval_fn), \
         patch("app.services.probe.providers.eval_gemini", side_effect=mock_eval_fn):

        res = await check_media_unlock(mock_runner_url, platforms=["youtube", "netflix", "gemini"], timeout_s=2.0)

        # Must create EXACTLY ONE client for all platforms
        assert len(created_clients) == 1, f"Expected 1 AsyncClient for shared session, got {len(created_clients)}"
        client_cfg = created_clients[0]
        assert client_cfg["trust_env"] is False, "trust_env must be False"
        assert str(client_cfg["proxy"]) == mock_runner_url, f"proxy must be {mock_runner_url}"
        assert client_cfg["follow_redirects"] is True
        assert client_cfg["headers"] == {"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"}
        assert len(res) == 3
        assert res["youtube"]["status"] == "verified"
        assert res["netflix"]["status"] == "verified"
        assert res["gemini"]["status"] == "verified"


@pytest.mark.asyncio
async def test_transport_handshake_failure_never_opens_media_session():
    """When transport handshake fails, media unlock check must NEVER be called or open clients."""
    node = {
        "name": "failing-handshake-node",
        "server": "1.2.3.4",
        "port": 443,
        "type": "ss",
        "password": "pass",
        "cipher": "aes-128-gcm",
    }
    mock_runner_ctx = {"proxy_url": "http://127.0.0.1:21000", "port": 21000, "is_runner": True}
    from contextlib import asynccontextmanager
    @asynccontextmanager
    async def mock_spawn(n, **kwargs):
        yield mock_runner_ctx

    mock_failed_transport = {"status": "fail", "latency_ms": None, "target": "https://cp.cloudflare.com/generate_204", "error": "Handshake failed"}
    mock_media_unlock = AsyncMock()

    with patch("app.services.probe.service.spawn_node_runner", side_effect=mock_spawn), \
         patch("app.services.probe.service.check_transport", return_value=mock_failed_transport), \
         patch("app.services.probe.service.check_media_unlock", mock_media_unlock):

        resolved = ResolvedProbeConfig(
            service_timeout_s=2.0,
            media_check_enabled=True,
            media_platforms=("youtube", "netflix", "gemini"),
        )
        res = await probe_single_node(node, resolved_config=resolved, use_cache=False)

        assert res["status"] == "fail"
        assert mock_media_unlock.call_count == 0, "check_media_unlock must NEVER be called when transport handshake fails"


@pytest.mark.asyncio
async def test_media_timeout_s_threaded_and_bounded_by_node_budget():
    """media_timeout_s must be passed to check_media_unlock and bounded by remaining node_timeout_s."""
    node = {
        "name": "timeout-bounded-node",
        "server": "1.2.3.4",
        "port": 443,
        "type": "ss",
        "password": "pass",
        "cipher": "aes-128-gcm",
    }
    mock_runner_ctx = {"proxy_url": "http://127.0.0.1:21000", "port": 21000, "is_runner": True}
    from contextlib import asynccontextmanager
    @asynccontextmanager
    async def mock_spawn(n, **kwargs):
        yield mock_runner_ctx

    mock_transport = {"status": "ok", "latency_ms": 50, "target": "https://cp.cloudflare.com/generate_204", "error": None}
    mock_geo = {"status": "ok", "ip": "1.2.3.4", "country": "US"}
    captured_media_timeouts: list[float] = []

    async def mock_check_media(proxy_url, platforms, timeout_s=2.0):
        captured_media_timeouts.append(timeout_s)
        return {p: {"status": "verified", "unlocked": True} for p in platforms}

    # Case 1: Configured media_timeout_s=8.0 with ample node budget
    with patch("app.services.probe.service.spawn_node_runner", side_effect=mock_spawn), \
         patch("app.services.probe.service.check_transport", return_value=mock_transport), \
         patch("app.services.probe.service.check_geo_identity", return_value=mock_geo), \
         patch("app.services.probe.service.check_media_unlock", side_effect=mock_check_media):

        resolved = ResolvedProbeConfig(
            service_timeout_s=2.0,
            media_timeout_s=8.0,
            node_timeout_s=30.0,
            media_platforms=("youtube", "netflix"),
        )
        res = await probe_single_node(node, resolved_config=resolved, use_cache=False)
        assert res["status"] == "ok"
        assert len(captured_media_timeouts) == 1
        assert abs(captured_media_timeouts[0] - 8.0) < 0.2, f"Expected media_timeout_s ~ 8.0, got {captured_media_timeouts[0]}"

    # Case 2: Configured media_timeout_s=10.0 but remaining node budget is small (0.5s)
    captured_media_timeouts.clear()
    with patch("app.services.probe.service.spawn_node_runner", side_effect=mock_spawn), \
         patch("app.services.probe.service.check_transport", return_value=mock_transport), \
         patch("app.services.probe.service.check_geo_identity", return_value=mock_geo), \
         patch("app.services.probe.service.check_media_unlock", side_effect=mock_check_media):

        resolved = ResolvedProbeConfig(
            service_timeout_s=2.0,
            media_timeout_s=10.0,
            node_timeout_s=0.5,
            media_platforms=("youtube", "netflix"),
        )
        res = await probe_single_node(node, resolved_config=resolved, use_cache=False)
        assert res["status"] == "ok"
        assert len(captured_media_timeouts) == 1
        assert captured_media_timeouts[0] <= 0.5, f"Media timeout {captured_media_timeouts[0]} must not exceed remaining node budget 0.5"
