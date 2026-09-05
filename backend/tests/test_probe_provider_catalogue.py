"""Unit and contract tests for evidence-grade probe catalogue evaluators.

Validates the versioned, credential-free provider catalogue, normalized
result taxonomy, secret-free evidence boundaries, and contract drift handling.
"""
from __future__ import annotations

import json
from pathlib import Path
from typing import Any
from unittest.mock import AsyncMock, patch
import pytest
import httpx

from app.services.probe.catalogue import (
    EVIDENCE_VERSION,
    STATUS_TAXONOMY,
    VERDICT_TAXONOMY,
    eval_bilibili,
    eval_chatgpt,
    eval_disney,
    eval_gemini,
    eval_aistudio,
    eval_meta_ai,
    eval_netflix,
    eval_youtube,
    eval_youtube_cdn,
)
from app.services.probe.models import (
    CdnRoutingObservation,
    IdentityObservation,
    ProviderEvidence,
    ProviderResult,
)


FIXTURES_DIR = Path(__file__).parent / "fixtures" / "probe_catalogue"


def _read_fixture(filename: str) -> str:
    return (FIXTURES_DIR / filename).read_text(encoding="utf-8")


def _build_mock_response(
    status_code: int,
    text: str = "",
    json_data: Any = None,
    headers: dict[str, str] | None = None,
    url: str = "https://example.com",
) -> httpx.Response:
    request = httpx.Request("GET", url)
    if json_data is not None:
        import json
        return httpx.Response(status_code, text=json.dumps(json_data), headers=headers or {}, request=request)
    return httpx.Response(status_code, text=text, headers=headers or {}, request=request)


# ---------------------------------------------------------------------------
# 1. Normalized Result Types & Secret Redaction Boundaries
# ---------------------------------------------------------------------------

def test_status_taxonomy_completeness():
    expected_statuses = {
        "verified",
        "partial",
        "restricted",
        "ip_blocked",
        "challenged",
        "rate_limited",
        "timeout",
        "transport_error",
        "inconclusive",
        "disabled",
    }
    assert STATUS_TAXONOMY == expected_statuses


def test_verdict_taxonomy_completeness():
    expected_verdicts = {
        "full",
        "originals_only",
        "available",
        "unsupported_region",
        "blocked",
        "challenge",
        "rate_limited",
        "unknown",
    }
    assert VERDICT_TAXONOMY == expected_verdicts


def test_provider_result_serialization_and_bounded_evidence():
    evidence = ProviderEvidence(
        http_status=200,
        final_host="www.netflix.com",
        redirect_class="none",
        signals=["non_original_available", "original_available"],
        elapsed_ms=45,
    )
    res = ProviderResult(
        status="verified",
        verdict="full",
        unlocked=True,
        region="US",
        checked_at=1788546780,
        evidence_version=EVIDENCE_VERSION,
        confidence="verified",
        evidence=evidence.to_dict(),
        label="原生全解锁",
    )
    d = res.to_dict()

    # Required top-level keys
    assert d["status"] == "verified"
    assert d["verdict"] == "full"
    assert d["unlocked"] is True
    assert d["region"] == "US"
    assert d["checked_at"] == 1788546780
    assert d["evidence_version"] == EVIDENCE_VERSION
    assert d["confidence"] == "verified"
    assert d["label"] == "原生全解锁"
    assert isinstance(d["evidence"], dict)

    # Allowed evidence keys ONLY
    allowed_evidence_keys = {"http_status", "final_host", "redirect_class", "signals", "elapsed_ms", "error_code"}
    assert set(d["evidence"].keys()).issubset(allowed_evidence_keys)

    # Strictly no secrets or raw bodies
    forbidden_keys = {"body", "response", "request", "authorization", "cookie", "token", "password", "proxy_url"}
    for k in d["evidence"].keys():
        assert k.lower() not in forbidden_keys


# ---------------------------------------------------------------------------
# 2. Netflix Evaluator Fixtures
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_netflix_evaluator_verified_full():
    html_non_orig = _read_fixture("netflix_full_non_original.html")
    html_orig = _read_fixture("netflix_full_original.html")

    async def mock_get(url, *args, **kwargs):
        if "70143836" in str(url):
            return _build_mock_response(200, text=html_non_orig, url=str(url))
        return _build_mock_response(200, text=html_orig, url=str(url))

    client = AsyncMock(spec=httpx.AsyncClient)
    client.get = AsyncMock(side_effect=mock_get)

    res = await eval_netflix(client)
    d = res.to_dict()
    assert d["status"] == "verified"
    assert d["verdict"] == "full"
    assert d["unlocked"] is True
    assert "non_original_available" in d["evidence"]["signals"]
    assert "original_available" in d["evidence"]["signals"]
    assert d["label"] == "原生全解锁"


@pytest.mark.asyncio
async def test_netflix_evaluator_partial_originals_only():
    html_orig = _read_fixture("netflix_full_original.html")

    async def mock_get(url, *args, **kwargs):
        if "70143836" in str(url):
            return _build_mock_response(404, text="Not found", url=str(url))
        return _build_mock_response(200, text=html_orig, url=str(url))

    client = AsyncMock(spec=httpx.AsyncClient)
    client.get = AsyncMock(side_effect=mock_get)

    res = await eval_netflix(client)
    d = res.to_dict()
    assert d["status"] == "partial"
    assert d["verdict"] == "originals_only"
    assert d["unlocked"] is False
    assert "original_available" in d["evidence"]["signals"]
    assert "non_original_available" not in d["evidence"]["signals"]
    assert d["label"] == "仅自制剧"


@pytest.mark.asyncio
async def test_netflix_evaluator_restricted():
    html_restricted = _read_fixture("netflix_restricted.html")

    async def mock_get(url, *args, **kwargs):
        return _build_mock_response(200, text=html_restricted, url="https://www.netflix.com/browse")

    client = AsyncMock(spec=httpx.AsyncClient)
    client.get = AsyncMock(side_effect=mock_get)

    res = await eval_netflix(client)
    d = res.to_dict()
    assert d["status"] == "restricted"
    assert d["verdict"] == "unsupported_region"
    assert d["unlocked"] is False


@pytest.mark.asyncio
async def test_netflix_evaluator_challenge_and_rate_limit():
    html_challenge = _read_fixture("netflix_challenge.html")

    # 1. 403 Challenge
    client_403 = AsyncMock(spec=httpx.AsyncClient)
    client_403.get = AsyncMock(return_value=_build_mock_response(403, text=html_challenge))
    res_403 = await eval_netflix(client_403)
    assert res_403.status == "challenged"
    assert res_403.verdict == "challenge"
    assert res_403.unlocked is False

    # 2. 429 Rate limited
    client_429 = AsyncMock(spec=httpx.AsyncClient)
    client_429.get = AsyncMock(return_value=_build_mock_response(429, text="Too many requests"))
    res_429 = await eval_netflix(client_429)
    assert res_429.status == "rate_limited"
    assert res_429.verdict == "rate_limited"
    assert res_429.unlocked is False


@pytest.mark.asyncio
async def test_netflix_evaluator_contract_drift():
    html_drift = _read_fixture("netflix_contract_drift.html")

    client = AsyncMock(spec=httpx.AsyncClient)
    client.get = AsyncMock(return_value=_build_mock_response(200, text=html_drift, url="https://www.netflix.com/title/70143836"))

    res = await eval_netflix(client)
    d = res.to_dict()
    assert d["status"] == "inconclusive"
    assert d["verdict"] == "unknown"
    assert d["unlocked"] is False
    assert "contract_drift" in d["evidence"]["signals"]


# ---------------------------------------------------------------------------
# 3. YouTube Evaluator Fixtures
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_youtube_evaluator_verified():
    html_verified = _read_fixture("youtube_verified.html")
    client = AsyncMock(spec=httpx.AsyncClient)
    client.get = AsyncMock(return_value=_build_mock_response(200, text=html_verified, url="https://www.youtube.com/premium"))

    res = await eval_youtube(client)
    d = res.to_dict()
    assert d["status"] == "verified"
    assert d["verdict"] == "available"
    assert d["unlocked"] is True
    assert d["region"] == "US"
    assert "premium_available" in d["evidence"]["signals"]


@pytest.mark.asyncio
async def test_youtube_evaluator_restricted():
    html_restricted = _read_fixture("youtube_restricted.html")
    client = AsyncMock(spec=httpx.AsyncClient)
    client.get = AsyncMock(return_value=_build_mock_response(200, text=html_restricted, url="https://www.youtube.com/premium"))

    res = await eval_youtube(client)
    d = res.to_dict()
    assert d["status"] == "restricted"
    assert d["verdict"] == "unsupported_region"
    assert d["unlocked"] is False
    assert d["region"] is None
    assert "country_unavailable" in d["evidence"]["signals"]


@pytest.mark.asyncio
async def test_youtube_evaluator_challenge_and_drift():
    html_challenge = _read_fixture("youtube_challenge.html")
    client_chal = AsyncMock(spec=httpx.AsyncClient)
    client_chal.get = AsyncMock(return_value=_build_mock_response(403, text=html_challenge))
    res_chal = await eval_youtube(client_chal)
    assert res_chal.status == "challenged"
    assert res_chal.unlocked is False

    html_drift = _read_fixture("youtube_contract_drift.html")
    client_drift = AsyncMock(spec=httpx.AsyncClient)
    client_drift.get = AsyncMock(return_value=_build_mock_response(200, text=html_drift, url="https://www.youtube.com/premium"))
    res_drift = await eval_youtube(client_drift)
    assert res_drift.status == "inconclusive"
    assert res_drift.verdict == "unknown"
    assert res_drift.unlocked is False


# ---------------------------------------------------------------------------
# 4. Disney+ Evaluator Fixtures
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_disney_evaluator_outcomes():
    # 1. Verified landing
    client_ok = AsyncMock(spec=httpx.AsyncClient)
    client_ok.get = AsyncMock(return_value=_build_mock_response(200, text="Disney+ Stream Now", url="https://www.disneyplus.com/home"))
    res_ok = await eval_disney(client_ok)
    assert res_ok.status == "verified"
    assert res_ok.verdict == "available"
    assert res_ok.unlocked is True

    # 2. Restricted preview / unavailable
    client_restr = AsyncMock(spec=httpx.AsyncClient)
    client_restr.get = AsyncMock(return_value=_build_mock_response(302, headers={"location": "https://preview.disneyplus.com/unavailable"}))
    res_restr = await eval_disney(client_restr)
    assert res_restr.status == "restricted"
    assert res_restr.verdict == "unsupported_region"
    assert res_restr.unlocked is False

    # 3. 403 Challenge
    client_403 = AsyncMock(spec=httpx.AsyncClient)
    client_403.get = AsyncMock(return_value=_build_mock_response(403))
    res_403 = await eval_disney(client_403)
    assert res_403.status == "challenged"
    assert res_403.unlocked is False


# ---------------------------------------------------------------------------
# 5. ChatGPT Evaluator Fixtures
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_chatgpt_evaluator_outcomes():
    # 1. Verified normal
    client_ok = AsyncMock(spec=httpx.AsyncClient)
    client_ok.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("chatgpt_normal.json")))
    res_ok = await eval_chatgpt(client_ok)
    assert res_ok.status == "verified"
    assert res_ok.verdict == "available"
    assert res_ok.unlocked is True

    # 2. Blocked 1020 / 403
    client_blk = AsyncMock(spec=httpx.AsyncClient)
    client_blk.get = AsyncMock(return_value=_build_mock_response(403, text=_read_fixture("chatgpt_blocked.html")))
    res_blk = await eval_chatgpt(client_blk)
    assert res_blk.status == "challenged"
    assert res_blk.unlocked is False

    # 3. 429
    client_429 = AsyncMock(spec=httpx.AsyncClient)
    client_429.get = AsyncMock(return_value=_build_mock_response(429))
    res_429 = await eval_chatgpt(client_429)
    assert res_429.status == "rate_limited"
    assert res_429.unlocked is False

    # 4. Inconclusive drift
    client_drift = AsyncMock(spec=httpx.AsyncClient)
    client_drift.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("chatgpt_drift.json")))
    res_drift = await eval_chatgpt(client_drift)
    assert res_drift.status == "inconclusive"
    assert res_drift.verdict == "unknown"


# ---------------------------------------------------------------------------
# 6. Bilibili Evaluator Fixtures
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_bilibili_evaluator_outcomes():
    # 1. Verified HK/MO/TW (code == 0)
    client_ok = AsyncMock(spec=httpx.AsyncClient)
    client_ok.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("bilibili_hk_mo_tw.json")))
    res_ok = await eval_bilibili(client_ok)
    assert res_ok.status == "verified"
    assert res_ok.verdict == "available"
    assert res_ok.region == "港澳台地区"
    assert res_ok.unlocked is True

    # 2. Restricted Mainland (code == -10403)
    client_main = AsyncMock(spec=httpx.AsyncClient)
    client_main.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("bilibili_mainland.json")))
    res_main = await eval_bilibili(client_main)
    assert res_main.status == "restricted"
    assert res_main.verdict == "unsupported_region"
    assert res_main.region == "CN"
    assert res_main.unlocked is False

    # 3. Drift (unknown code)
    client_drift = AsyncMock(spec=httpx.AsyncClient)
    client_drift.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("bilibili_drift.json")))
    res_drift = await eval_bilibili(client_drift)
    assert res_drift.status == "inconclusive"
    assert res_drift.verdict == "unknown"


# ---------------------------------------------------------------------------
# 7. Meta AI & Gemini Evaluators
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_meta_ai_evaluator_outcomes():
    client_ok = AsyncMock(spec=httpx.AsyncClient)
    client_ok.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("meta_ai_available.html")))
    res_ok = await eval_meta_ai(client_ok)
    assert res_ok.status == "verified"
    assert res_ok.unlocked is True

    client_restr = AsyncMock(spec=httpx.AsyncClient)
    client_restr.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("meta_ai_restricted.html")))
    res_restr = await eval_meta_ai(client_restr)
    assert res_restr.status == "restricted"
    assert res_restr.unlocked is False

    client_drift = AsyncMock(spec=httpx.AsyncClient)
    client_drift.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("meta_ai_drift.html")))
    res_drift = await eval_meta_ai(client_drift)
    assert res_drift.status == "inconclusive"
    assert res_drift.unlocked is False


@pytest.mark.asyncio
async def test_gemini_evaluator_outcomes():
    client_ok = AsyncMock(spec=httpx.AsyncClient)
    client_ok.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("gemini_available.html")))
    res_ok = await eval_gemini(client_ok)
    assert res_ok.status == "verified"
    assert res_ok.unlocked is True

    client_restr = AsyncMock(spec=httpx.AsyncClient)
    client_restr.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("gemini_restricted.html")))
    res_restr = await eval_gemini(client_restr)
    assert res_restr.status == "restricted"
    assert res_restr.unlocked is False

    client_drift = AsyncMock(spec=httpx.AsyncClient)
    client_drift.get = AsyncMock(return_value=_build_mock_response(200, text=_read_fixture("gemini_drift.html")))
    res_drift = await eval_gemini(client_drift)
    assert res_drift.status == "inconclusive"
    assert res_drift.unlocked is False


# ---------------------------------------------------------------------------
# 8. YouTube CDN Routing Hint Evaluator
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_youtube_cdn_evaluator_outcomes():
    mapping_text = _read_fixture("youtube_cdn_mapping.txt")
    client_ok = AsyncMock(spec=httpx.AsyncClient)
    client_ok.get = AsyncMock(return_value=_build_mock_response(200, text=mapping_text, url="https://redirector.googlevideo.com/report_mapping"))

    res_ok = await eval_youtube_cdn(client_ok)
    d = res_ok.to_dict()
    assert d["status"] == "verified"
    assert d["verdict"] == "available"
    assert d["confidence"] == "verified"
    assert d["route_hint"] == "ord38s01-in-f1.1e100.net"
    assert d["iata_code"] == "ORD"
    assert "mapping_observed" in d["evidence"]["signals"]

    drift_text = _read_fixture("youtube_cdn_drift.txt")
    client_drift = AsyncMock(spec=httpx.AsyncClient)
    client_drift.get = AsyncMock(return_value=_build_mock_response(200, text=drift_text, url="https://redirector.googlevideo.com/report_mapping"))

    res_drift = await eval_youtube_cdn(client_drift)
    assert res_drift.status == "inconclusive"
    assert res_drift.verdict == "unknown"
    assert res_drift.route_hint is None
    assert res_drift.iata_code is None


@pytest.mark.asyncio
async def test_netflix_fast_path_outcomes():
    """Verify Fast.com fast conclusion and safe fallback branches."""
    # 1. Fast path 200 with valid country -> verified full with country
    client_fast_ok = AsyncMock(spec=httpx.AsyncClient)
    fast_json = {
        "client": {"ip": "1.2.3.4", "location": {"country": "US"}},
        "targets": [{"name": "target-1", "url": "https://example.com", "location": {"country": "US"}}],
    }
    client_fast_ok.get = AsyncMock(return_value=_build_mock_response(200, json_data=fast_json, url="https://api.fast.com/netflix/speedtest/v2"))
    res_fast_ok = await eval_netflix(client_fast_ok, enable_fast_path=True)
    assert res_fast_ok.status == "verified"
    assert res_fast_ok.verdict == "full"
    assert res_fast_ok.unlocked is True
    assert res_fast_ok.region == "US"
    assert any("fast_com" in s for s in res_fast_ok.evidence.get("signals", []))

    # 2. Fast path 403 -> IP blocked
    client_fast_403 = AsyncMock(spec=httpx.AsyncClient)
    client_fast_403.get = AsyncMock(return_value=_build_mock_response(403, text="Forbidden", url="https://api.fast.com/netflix/speedtest/v2"))
    res_fast_403 = await eval_netflix(client_fast_403, enable_fast_path=True)
    assert res_fast_403.status == "ip_blocked"
    assert res_fast_403.verdict == "blocked"
    assert res_fast_403.unlocked is False

    # 3. Fast path 500 error -> falls back to title check (where title check succeeds)
    client_fallback = AsyncMock(spec=httpx.AsyncClient)
    calls = 0
    async def mock_fallback_get(url, *args, **kwargs):
        nonlocal calls
        calls += 1
        if "fast.com" in str(url):
            return _build_mock_response(500, text="Server Error", url="https://api.fast.com/netflix/speedtest/v2")
        if "70143836" in str(url):
            return _build_mock_response(200, text=_read_fixture("netflix_full_non_original.html"), url="https://www.netflix.com/title/70143836")
        if "80018499" in str(url):
            return _build_mock_response(200, text=_read_fixture("netflix_full_original.html"), url="https://www.netflix.com/title/80018499")
        return _build_mock_response(404, text="Not Found")

    client_fallback.get = AsyncMock(side_effect=mock_fallback_get)
    res_fallback = await eval_netflix(client_fallback, enable_fast_path=True)
    assert res_fallback.error is None, f"Error: {res_fallback.error}"
    assert res_fallback.status == "verified"
    assert res_fallback.verdict == "full"
    assert res_fallback.unlocked is True
    assert calls >= 3, "Must request fast path then both title fallback endpoints"


@pytest.mark.asyncio
async def test_aistudio_evaluator_outcomes():
    """Verify Google AI Studio zero-credential geofence probe outcomes."""
    # 1. 400 with API_KEY_INVALID -> Geofence passed, full unlock
    client_ok = AsyncMock(spec=httpx.AsyncClient)
    api_key_invalid_body = json.dumps({
        "error": {
            "code": 400,
            "message": "API key not valid. Please pass a valid API key.",
            "status": "INVALID_ARGUMENT",
            "details": [{"reason": "API_KEY_INVALID"}],
        }
    })
    client_ok.get = AsyncMock(return_value=_build_mock_response(
        400,
        text=api_key_invalid_body,
        url="https://generativelanguage.googleapis.com/v1beta/models?key=AIzaSyDummyCheckKey",
    ))
    res_ok = await eval_aistudio(client_ok)
    assert res_ok.status == "verified"
    assert res_ok.verdict == "available"
    assert res_ok.unlocked is True
    assert res_ok.label == "可用"

    # 2. 400 with FAILED_PRECONDITION / User location is not supported -> Restricted
    client_restr = AsyncMock(spec=httpx.AsyncClient)
    location_unsupported_body = json.dumps({
        "error": {
            "code": 400,
            "message": "User location is not supported for the API use without a project.",
            "status": "FAILED_PRECONDITION",
        }
    })
    client_restr.get = AsyncMock(return_value=_build_mock_response(
        400,
        text=location_unsupported_body,
        url="https://generativelanguage.googleapis.com/v1beta/models?key=AIzaSyDummyCheckKey",
    ))
    res_restr = await eval_aistudio(client_restr)
    assert res_restr.status == "restricted"
    assert res_restr.verdict == "unsupported_region"
    assert res_restr.unlocked is False
    assert res_restr.label == "未支持地区"

    # 3. 429 Rate limited
    client_429 = AsyncMock(spec=httpx.AsyncClient)
    client_429.get = AsyncMock(return_value=_build_mock_response(429, text="Too Many Requests"))
    res_429 = await eval_aistudio(client_429)
    assert res_429.status == "rate_limited"
    assert res_429.verdict == "rate_limited"

    # 4. 403 Challenge
    client_403 = AsyncMock(spec=httpx.AsyncClient)
    client_403.get = AsyncMock(return_value=_build_mock_response(403, text="Forbidden"))
    res_403 = await eval_aistudio(client_403)
    assert res_403.status == "challenged"
    assert res_403.verdict == "challenge"


@pytest.mark.asyncio
async def test_gemini_alpha3_fast_optimization():
    """Verify Gemini fast 3-letter country code match and blocked country interception."""
    # 1. Supported 3-letter country code (USA -> US)
    client_us = AsyncMock(spec=httpx.AsyncClient)
    us_body = '<html><script>window.DATA = [1,2,1,200,"USA",10];</script></html>'
    client_us.get = AsyncMock(return_value=_build_mock_response(200, text=us_body, url="https://gemini.google.com/"))
    res_us = await eval_gemini(client_us)
    assert res_us.status == "verified"
    assert res_us.verdict == "available"
    assert res_us.unlocked is True
    assert res_us.region == "US"

    # 2. Blocked 3-letter country code (HKG -> HK, restricted)
    client_hk = AsyncMock(spec=httpx.AsyncClient)
    hk_body = '<html><script>window.DATA = [1,2,1,200,"HKG",10];</script></html>'
    client_hk.get = AsyncMock(return_value=_build_mock_response(200, text=hk_body, url="https://gemini.google.com/"))
    res_hk = await eval_gemini(client_hk)
    assert res_hk.status == "restricted"
    assert res_hk.verdict == "unsupported_region"
    assert res_hk.unlocked is False
    assert res_hk.region == "HK"

    # 3. Blocked 3-letter country code (CHN -> CN, restricted)
    client_cn = AsyncMock(spec=httpx.AsyncClient)
    cn_body = '<html><script>window.DATA = [1,2,1,200,"CHN",10];</script></html>'
    client_cn.get = AsyncMock(return_value=_build_mock_response(200, text=cn_body, url="https://gemini.google.com/"))
    res_cn = await eval_gemini(client_cn)
    assert res_cn.status == "restricted"
    assert res_cn.verdict == "unsupported_region"
    assert res_cn.unlocked is False
    assert res_cn.region == "CN"
