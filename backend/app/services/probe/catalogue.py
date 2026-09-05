"""Credential-free provider catalogue and consensus-based capability evaluators.

Implements versioned, credential-free evaluators for Netflix, YouTube Premium,
Disney+, ChatGPT, Bilibili, Meta AI, Google Gemini, YouTube CDN routing,
and exit identity consensus conforming to OpenSpec.
"""
from __future__ import annotations

import asyncio
import logging
import re
import time
from typing import Any

import httpx

from app.services.probe.models import (
    CONFIDENCE_TAXONOMY,
    STATUS_TAXONOMY,
    VERDICT_TAXONOMY,
    CdnRoutingObservation,
    IdentityObservation,
    ProviderEvidence,
    ProviderResult,
)

logger = logging.getLogger(__name__)

EVIDENCE_VERSION = "catalogue-2026-09-05"

DEFAULT_UA = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

NETFLIX_FAST_PATH_ENDPOINT = "https://api.fast.com/netflix/speedtest/v2"
NETFLIX_FAST_PATH_ENABLED = False  # Disabled by default per Task 2.5 because unauthenticated endpoint requires an app token

AISTUDIO_API_URL = "https://generativelanguage.googleapis.com/v1beta/models?key=AIzaSyDummyCheckKey"
AISTUDIO_PORTAL_URL = "https://aistudio.google.com/"

GEMINI_ALPHA3_RE = re.compile(r',2,1,200,"([A-Z]{3})"')
GEMINI_BLOCKED_ALPHA3 = {
    "CHN", "RUS", "BLR", "CUB", "IRN", "PRK", "SYR", "HKG", "MAC",
}
ALPHA3_TO_ALPHA2 = {
    "USA": "US", "GBR": "GB", "JPN": "JP", "SGP": "SG", "DEU": "DE",
    "FRA": "FR", "CAN": "CA", "AUS": "AU", "TWN": "TW", "KOR": "KR",
    "IND": "IN", "NLD": "NL", "SWE": "SE", "CHE": "CH", "HKG": "HK",
    "CHN": "CN", "MAC": "MO", "RUS": "RU", "BRA": "BR", "ZAF": "ZA",
    "MEX": "MX", "MYS": "MY", "THA": "TH", "VNM": "VN", "IDN": "ID",
    "PHL": "PH", "NZL": "NZ", "IRL": "IE", "ITA": "IT", "ESP": "ES",
}


async def execute_request_with_retry(
    client: httpx.AsyncClient,
    method: str,
    url: str,
    *,
    headers: dict[str, str] | None = None,
    deadline_monotonic: float,
    follow_redirects: bool = True,
) -> httpx.Response:
    """Execute an HTTP request respecting service deadline and bounded retry discipline.

    At most ONE retry is permitted only for transient transport failure (ConnectError,
    ReadError, RemoteProtocolError) if remaining deadline permits. Definitive HTTP
    responses (including 200, 3xx, 403, 404, 429) never retry.
    """
    remaining = deadline_monotonic - time.monotonic()
    if remaining <= 0:
        raise asyncio.TimeoutError("service deadline exceeded before request dispatch")

    request_headers = {"User-Agent": DEFAULT_UA}
    if headers:
        request_headers.update(headers)

    attempts = 0
    while True:
        attempts += 1
        current_timeout = max(0.01, min(remaining, 10.0))
        try:
            if method.upper() == "GET":
                return await client.get(
                    url,
                    headers=request_headers,
                    timeout=current_timeout,
                    follow_redirects=follow_redirects,
                )
            elif method.upper() == "POST":
                return await client.post(
                    url,
                    headers=request_headers,
                    timeout=current_timeout,
                    follow_redirects=follow_redirects,
                )
            else:
                return await client.request(
                    method,
                    url,
                    headers=request_headers,
                    timeout=current_timeout,
                    follow_redirects=follow_redirects,
                )
        except (httpx.ConnectError, httpx.ReadError, httpx.RemoteProtocolError) as exc:
            remaining = deadline_monotonic - time.monotonic()
            if attempts == 1 and remaining > 0.05:
                logger.debug("Transient transport error on %s, retrying once (remaining=%.2fs): %s", url, remaining, exc)
                continue
            raise
        except (asyncio.TimeoutError, httpx.TimeoutException):
            raise


def _classify_error_outcome(
    exc: Exception,
    elapsed_ms: int,
    host: str | None = None,
) -> ProviderResult:
    """Classify unhandled exceptions into structured ProviderResult outcomes."""
    if isinstance(exc, (asyncio.TimeoutError, httpx.TimeoutException)):
        return ProviderResult(
            status="timeout",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(
                final_host=host,
                elapsed_ms=elapsed_ms,
                error_code="timeout",
            ).to_dict(),
            error=f"probe timed out: {exc}" if str(exc) else "probe timed out",
        )
    return ProviderResult(
        status="transport_error",
        verdict="unknown",
        unlocked=False,
        confidence="unavailable",
        evidence=ProviderEvidence(
            final_host=host,
            elapsed_ms=elapsed_ms,
            error_code="transport_error",
        ).to_dict(),
        error=f"transport error: {exc}" if str(exc) else "transport error",
    )


# ---------------------------------------------------------------------------
# Platform Evaluators
# ---------------------------------------------------------------------------

async def eval_netflix(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
    enable_fast_path: bool | None = None,
) -> ProviderResult:
    """Evaluate Netflix streaming capability (full catalogue vs originals-only)."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)

    non_orig_available = False
    orig_available = False
    signals: list[str] = []
    final_host: str | None = "www.netflix.com"
    last_status: int | None = None

    if enable_fast_path is None:
        enable_fast_path = NETFLIX_FAST_PATH_ENABLED

    if enable_fast_path:
        try:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise asyncio.TimeoutError("deadline exceeded before fast path dispatch")

            resp_fast = await execute_request_with_retry(
                client,
                "GET",
                NETFLIX_FAST_PATH_ENDPOINT,
                deadline_monotonic=deadline,
            )
            elapsed_ms = int((time.monotonic() - started) * 1000)
            final_host = resp_fast.url.host if resp_fast.url else "api.fast.com"

            if resp_fast.status_code == 403:
                return ProviderResult(
                    status="ip_blocked",
                    verdict="blocked",
                    unlocked=False,
                    confidence="verified",
                    evidence=ProviderEvidence(
                        http_status=403,
                        final_host=final_host,
                        signals=["fast_com_blocked"],
                        elapsed_ms=elapsed_ms,
                    ).to_dict(),
                    label="IP被阻断",
                )
            elif resp_fast.status_code == 200:
                try:
                    data = resp_fast.json()
                    country: str | None = None
                    if isinstance(data, dict):
                        targets = data.get("targets")
                        if isinstance(targets, list) and len(targets) > 0 and isinstance(targets[0], dict):
                            loc = targets[0].get("location")
                            if isinstance(loc, dict):
                                c = loc.get("country")
                                if isinstance(c, str) and len(c.strip()) == 2 and c.strip().isalpha():
                                    country = c.strip().upper()
                        if not country:
                            client_info = data.get("client")
                            if isinstance(client_info, dict):
                                loc = client_info.get("location")
                                if isinstance(loc, dict):
                                    c = loc.get("country")
                                    if isinstance(c, str) and len(c.strip()) == 2 and c.strip().isalpha():
                                        country = c.strip().upper()

                    if country:
                        return ProviderResult(
                            status="verified",
                            verdict="full",
                            unlocked=True,
                            region=country,
                            confidence="verified",
                            evidence=ProviderEvidence(
                                http_status=200,
                                final_host=final_host,
                                signals=["fast_com_verified", f"fast_com_region_{country.lower()}"],
                                elapsed_ms=elapsed_ms,
                            ).to_dict(),
                            label="原生全解锁",
                        )
                except Exception:
                    pass
        except (asyncio.TimeoutError, httpx.TimeoutException):
            elapsed_ms = int((time.monotonic() - started) * 1000)
            return ProviderResult(
                status="timeout",
                verdict="unknown",
                unlocked=False,
                confidence="unavailable",
                evidence=ProviderEvidence(
                    final_host=final_host,
                    elapsed_ms=elapsed_ms,
                    error_code="timeout",
                ).to_dict(),
                error="netflix fast path probe timed out",
            )
        except Exception:
            pass

    try:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise asyncio.TimeoutError("deadline exceeded before title check")

        # 1. Non-original licensed title (Breaking Bad: 70143836)
        resp_non = await execute_request_with_retry(
            client,
            "GET",
            "https://www.netflix.com/title/70143836",
            deadline_monotonic=deadline,
        )
        last_status = resp_non.status_code
        final_host = resp_non.url.host if resp_non.url else "www.netflix.com"

        if resp_non.status_code == 429:
            elapsed_ms = int((time.monotonic() - started) * 1000)
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=429, final_host=final_host, signals=["rate_limited"], elapsed_ms=elapsed_ms).to_dict(),
                label="限流",
            )
        if resp_non.status_code == 403 or "cf-challenge" in resp_non.text:
            elapsed_ms = int((time.monotonic() - started) * 1000)
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp_non.status_code, final_host=final_host, signals=["challenge_detected"], elapsed_ms=elapsed_ms).to_dict(),
                label="被风控或阻断",
            )

        if resp_non.status_code == 200:
            url_str = str(resp_non.url)
            text = resp_non.text
            if "/title/" in url_str and ("title-70143836" in text or "Breaking Bad" in text or "watch-button" in text):
                non_orig_available = True
                signals.append("non_original_available")

        # 2. Original title (Stranger Things: 80018499)
        resp_orig = await execute_request_with_retry(
            client,
            "GET",
            "https://www.netflix.com/title/80018499",
            deadline_monotonic=deadline,
        )
        last_status = resp_orig.status_code
        if resp_orig.url:
            final_host = resp_orig.url.host

        if resp_orig.status_code == 429:
            elapsed_ms = int((time.monotonic() - started) * 1000)
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=429, final_host=final_host, signals=["rate_limited"], elapsed_ms=elapsed_ms).to_dict(),
                label="限流",
            )
        if resp_orig.status_code == 403 or "cf-challenge" in resp_orig.text:
            elapsed_ms = int((time.monotonic() - started) * 1000)
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp_orig.status_code, final_host=final_host, signals=["challenge_detected"], elapsed_ms=elapsed_ms).to_dict(),
                label="被风控或阻断",
            )

        if resp_orig.status_code == 200:
            url_str = str(resp_orig.url)
            text = resp_orig.text
            if "/title/" in url_str and ("title-80018499" in text or "Stranger Things" in text or "watch-button" in text or "Netflix Original" in text):
                orig_available = True
                signals.append("original_available")

        elapsed_ms = int((time.monotonic() - started) * 1000)

        if non_orig_available and orig_available:
            return ProviderResult(
                status="verified",
                verdict="full",
                unlocked=True,
                confidence="verified",
                evidence=ProviderEvidence(http_status=200, final_host=final_host, signals=signals, elapsed_ms=elapsed_ms).to_dict(),
                label="原生全解锁",
            )
        if orig_available and not non_orig_available:
            return ProviderResult(
                status="partial",
                verdict="originals_only",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=200, final_host=final_host, signals=signals, elapsed_ms=elapsed_ms).to_dict(),
                label="仅自制剧",
            )

        # Neither title matched
        # Check if restriction marker or redirect away
        if (
            resp_non.status_code in (403, 404)
            or resp_orig.status_code in (403, 404)
            or "/title/" not in str(resp_non.url)
            or "not available" in resp_non.text.lower()
            or "unlimited movies" in resp_non.text.lower()
        ):
            signals.append("title_unavailable")
            return ProviderResult(
                status="restricted",
                verdict="unsupported_region",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=last_status, final_host=final_host, signals=signals, elapsed_ms=elapsed_ms).to_dict(),
                label="未解锁",
            )

        signals.append("contract_drift")
        return ProviderResult(
            status="inconclusive",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(http_status=last_status, final_host=final_host, signals=signals, elapsed_ms=elapsed_ms).to_dict(),
            label="未知",
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return _classify_error_outcome(exc, elapsed_ms, final_host)


async def eval_youtube(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
) -> ProviderResult:
    """Evaluate YouTube Premium unlock capability and detected region."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)
    final_host = "www.youtube.com"

    try:
        resp = await execute_request_with_retry(
            client,
            "GET",
            "https://www.youtube.com/premium",
            deadline_monotonic=deadline,
        )
        elapsed_ms = int((time.monotonic() - started) * 1000)
        final_host = resp.url.host if resp.url else final_host

        if resp.status_code == 429:
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=429, final_host=final_host, signals=["rate_limited"], elapsed_ms=elapsed_ms).to_dict(),
                label="限流",
            )
        if resp.status_code == 403 or "Before you continue" in resp.text or "unusual traffic" in resp.text:
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["challenge_detected"], elapsed_ms=elapsed_ms).to_dict(),
                label="被风控或阻断",
            )

        if resp.status_code == 200:
            text = resp.text
            if "Premium is not available in your country" in text or "YouTube Premium 在你所在的国家/地区不可用" in text:
                return ProviderResult(
                    status="restricted",
                    verdict="unsupported_region",
                    unlocked=False,
                    region=None,
                    confidence="verified",
                    evidence=ProviderEvidence(http_status=200, final_host=final_host, signals=["country_unavailable"], elapsed_ms=elapsed_ms).to_dict(),
                    label="地区不可用",
                )

            match = re.search(r'"countryCode":\s*"([A-Z]{2})"', text)
            if match:
                region = match.group(1)
                return ProviderResult(
                    status="verified",
                    verdict="available",
                    unlocked=True,
                    region=region,
                    confidence="verified",
                    evidence=ProviderEvidence(
                        http_status=200,
                        final_host=final_host,
                        signals=["premium_available", f"region_{region.lower()}"],
                        elapsed_ms=elapsed_ms,
                    ).to_dict(),
                    label="解锁",
                )

            return ProviderResult(
                status="inconclusive",
                verdict="unknown",
                unlocked=False,
                confidence="unavailable",
                evidence=ProviderEvidence(http_status=200, final_host=final_host, signals=["contract_drift"], elapsed_ms=elapsed_ms).to_dict(),
                label="未知",
            )

        return ProviderResult(
            status="inconclusive",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["unexpected_status"], elapsed_ms=elapsed_ms).to_dict(),
            label="未知",
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return _classify_error_outcome(exc, elapsed_ms, final_host)


async def eval_disney(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
) -> ProviderResult:
    """Evaluate Disney+ streaming capability through credential-free landing redirect."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)
    final_host = "www.disneyplus.com"

    try:
        resp = await execute_request_with_retry(
            client,
            "GET",
            "https://www.disneyplus.com/",
            deadline_monotonic=deadline,
            follow_redirects=True,
        )
        elapsed_ms = int((time.monotonic() - started) * 1000)
        final_host = resp.url.host if resp.url else final_host

        if resp.status_code == 429:
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=429, final_host=final_host, signals=["rate_limited"], elapsed_ms=elapsed_ms).to_dict(),
                label="限流",
            )
        if resp.status_code == 403:
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=403, final_host=final_host, signals=["challenge_detected"], elapsed_ms=elapsed_ms).to_dict(),
                label="被风控或阻断",
            )

        loc = str(resp.headers.get("location") or "")
        url_str = str(resp.url)
        if "unavailable" in loc or "preview" in loc or "unavailable" in url_str:
            return ProviderResult(
                status="restricted",
                verdict="unsupported_region",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, redirect_class="location_unavailable", signals=["region_unavailable"], elapsed_ms=elapsed_ms).to_dict(),
                label="未支持地区",
            )

        if resp.status_code in (200, 301, 302):
            if "disneyplus" in url_str or "Disney" in resp.text:
                return ProviderResult(
                    status="verified",
                    verdict="available",
                    unlocked=True,
                    confidence="verified",
                    evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, redirect_class="landing_ok", signals=["landing_available"], elapsed_ms=elapsed_ms).to_dict(),
                    label="解锁",
                )

        return ProviderResult(
            status="inconclusive",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["contract_drift"], elapsed_ms=elapsed_ms).to_dict(),
            label="未知",
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return _classify_error_outcome(exc, elapsed_ms, final_host)


async def eval_chatgpt(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
) -> ProviderResult:
    """Evaluate ChatGPT / OpenAI API accessibility."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)
    final_host = "ios.chat.openai.com"

    try:
        resp = await execute_request_with_retry(
            client,
            "GET",
            "https://ios.chat.openai.com/public-api/mobile/server_status/v1",
            deadline_monotonic=deadline,
        )
        elapsed_ms = int((time.monotonic() - started) * 1000)
        final_host = resp.url.host if resp.url else final_host

        if resp.status_code == 429:
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=429, final_host=final_host, signals=["rate_limited"], elapsed_ms=elapsed_ms).to_dict(),
                label="限流",
            )
        if resp.status_code in (403, 1020) or "Cloudflare" in resp.text:
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["cf_blocked"], elapsed_ms=elapsed_ms).to_dict(),
                label="被风控或阻断",
            )

        if resp.status_code == 200:
            try:
                data = resp.json()
                if data.get("status") in ("normal", "ok"):
                    return ProviderResult(
                        status="verified",
                        verdict="available",
                        unlocked=True,
                        confidence="verified",
                        evidence=ProviderEvidence(http_status=200, final_host=final_host, signals=["server_status_normal"], elapsed_ms=elapsed_ms).to_dict(),
                        label="允许访问",
                    )
            except Exception:
                pass

        return ProviderResult(
            status="inconclusive",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["contract_drift"], elapsed_ms=elapsed_ms).to_dict(),
            label="未知",
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return _classify_error_outcome(exc, elapsed_ms, final_host)


async def eval_bilibili(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
) -> ProviderResult:
    """Evaluate Bilibili HK/MO/TW versus Mainland regional availability."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)
    final_host = "api.bilibili.com"

    try:
        resp = await execute_request_with_retry(
            client,
            "GET",
            "https://api.bilibili.com/pgc/player/web/v2/playurl?ep_id=268176",
            deadline_monotonic=deadline,
        )
        elapsed_ms = int((time.monotonic() - started) * 1000)
        final_host = resp.url.host if resp.url else final_host

        if resp.status_code == 429:
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=429, final_host=final_host, signals=["rate_limited"], elapsed_ms=elapsed_ms).to_dict(),
                label="限流",
            )
        if resp.status_code in (403, 412):
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["challenge_detected"], elapsed_ms=elapsed_ms).to_dict(),
                label="被风控或阻断",
            )

        if resp.status_code == 200:
            try:
                data = resp.json()
                code = data.get("code")
                if code == 0:
                    return ProviderResult(
                        status="verified",
                        verdict="available",
                        unlocked=True,
                        region="港澳台地区",
                        confidence="verified",
                        evidence=ProviderEvidence(http_status=200, final_host=final_host, signals=["playurl_code_0"], elapsed_ms=elapsed_ms).to_dict(),
                        label="港澳台限定",
                    )
                elif code == -10403:
                    return ProviderResult(
                        status="restricted",
                        verdict="unsupported_region",
                        unlocked=False,
                        region="CN",
                        confidence="verified",
                        evidence=ProviderEvidence(http_status=200, final_host=final_host, signals=["mainland_only_code_10403"], elapsed_ms=elapsed_ms).to_dict(),
                        label="仅大陆",
                    )
            except Exception:
                pass

        return ProviderResult(
            status="inconclusive",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["contract_drift"], elapsed_ms=elapsed_ms).to_dict(),
            label="未知",
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return _classify_error_outcome(exc, elapsed_ms, final_host)


async def eval_meta_ai(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
) -> ProviderResult:
    """Evaluate Meta AI service accessibility."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)
    final_host = "www.meta.ai"

    try:
        resp = await execute_request_with_retry(
            client,
            "GET",
            "https://www.meta.ai/",
            deadline_monotonic=deadline,
            follow_redirects=True,
        )
        elapsed_ms = int((time.monotonic() - started) * 1000)
        final_host = resp.url.host if resp.url else final_host

        if resp.status_code == 429:
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=429, final_host=final_host, signals=["rate_limited"], elapsed_ms=elapsed_ms).to_dict(),
                label="限流",
            )
        if resp.status_code == 403:
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=403, final_host=final_host, signals=["challenge_detected"], elapsed_ms=elapsed_ms).to_dict(),
                label="被风控或阻断",
            )

        loc = str(resp.headers.get("location") or "")
        text = resp.text
        if (
            "Meta AI isn't available yet in your country" in text
            or "not available in your country" in text
            or "unavailable" in loc
        ):
            return ProviderResult(
                status="restricted",
                verdict="unsupported_region",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["country_unavailable"], elapsed_ms=elapsed_ms).to_dict(),
                label="地区不可用",
            )

        if resp.status_code == 200 and ("Meta AI" in text or "Imagine" in text):
            return ProviderResult(
                status="verified",
                verdict="available",
                unlocked=True,
                confidence="verified",
                evidence=ProviderEvidence(http_status=200, final_host=final_host, signals=["portal_available"], elapsed_ms=elapsed_ms).to_dict(),
                label="可用",
            )

        return ProviderResult(
            status="inconclusive",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["contract_drift"], elapsed_ms=elapsed_ms).to_dict(),
            label="未知",
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return _classify_error_outcome(exc, elapsed_ms, final_host)


async def eval_gemini(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
) -> ProviderResult:
    """Evaluate Google Gemini service accessibility with fast 3-letter country code matching."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)
    final_host = "gemini.google.com"

    try:
        resp = await execute_request_with_retry(
            client,
            "GET",
            "https://gemini.google.com/",
            deadline_monotonic=deadline,
            follow_redirects=True,
        )
        elapsed_ms = int((time.monotonic() - started) * 1000)
        final_host = resp.url.host if resp.url else final_host

        if resp.status_code == 429:
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=429, final_host=final_host, signals=["rate_limited"], elapsed_ms=elapsed_ms).to_dict(),
                label="限流",
            )
        if resp.status_code == 403:
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=403, final_host=final_host, signals=["challenge_detected"], elapsed_ms=elapsed_ms).to_dict(),
                label="被风控或阻断",
            )

        loc = str(resp.headers.get("location") or "")
        text = resp.text
        url_str = str(resp.url)

        # 1. Fast subcheck 3-letter country code match
        match = GEMINI_ALPHA3_RE.search(text)
        if match:
            alpha3 = match.group(1).upper()
            if alpha3 in GEMINI_BLOCKED_ALPHA3:
                return ProviderResult(
                    status="restricted",
                    verdict="unsupported_region",
                    unlocked=False,
                    region=ALPHA3_TO_ALPHA2.get(alpha3, alpha3),
                    confidence="verified",
                    evidence=ProviderEvidence(
                        http_status=resp.status_code,
                        final_host=final_host,
                        signals=["country_unsupported", f"alpha3_{alpha3.lower()}"],
                        elapsed_ms=elapsed_ms,
                    ).to_dict(),
                    label="未支持地区",
                )
            region_code = ALPHA3_TO_ALPHA2.get(alpha3, alpha3)
            return ProviderResult(
                status="verified",
                verdict="available",
                unlocked=True,
                region=region_code,
                confidence="verified",
                evidence=ProviderEvidence(
                    http_status=resp.status_code,
                    final_host=final_host,
                    signals=["gemini_available", f"alpha3_{alpha3.lower()}"],
                    elapsed_ms=elapsed_ms,
                ).to_dict(),
                label="可用",
            )

        # 2. Existing fallback checks
        if (
            "unavailable" in loc
            or "sorry" in url_str
            or "Gemini isn't currently supported in your country" in text
        ):
            return ProviderResult(
                status="restricted",
                verdict="unsupported_region",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["country_unsupported"], elapsed_ms=elapsed_ms).to_dict(),
                label="未支持地区",
            )

        if resp.status_code in (200, 302, 303) and ("Google Gemini" in text or "Gemini" in text):
            return ProviderResult(
                status="verified",
                verdict="available",
                unlocked=True,
                confidence="verified",
                evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["gemini_available"], elapsed_ms=elapsed_ms).to_dict(),
                label="可用",
            )

        return ProviderResult(
            status="inconclusive",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(http_status=resp.status_code, final_host=final_host, signals=["contract_drift"], elapsed_ms=elapsed_ms).to_dict(),
            label="未知",
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return _classify_error_outcome(exc, elapsed_ms, final_host)


async def eval_aistudio(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
) -> ProviderResult:
    """Evaluate Google AI Studio service accessibility through zero-credential geofence probe."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)
    final_host = "generativelanguage.googleapis.com"

    try:
        resp = await execute_request_with_retry(
            client,
            "GET",
            AISTUDIO_API_URL,
            deadline_monotonic=deadline,
        )
        elapsed_ms = int((time.monotonic() - started) * 1000)
        if resp.url:
            final_host = resp.url.host

        if resp.status_code == 429:
            return ProviderResult(
                status="rate_limited",
                verdict="rate_limited",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(
                    http_status=429,
                    final_host=final_host,
                    signals=["rate_limited"],
                    elapsed_ms=elapsed_ms,
                ).to_dict(),
                label="限流",
            )
        if resp.status_code == 403:
            return ProviderResult(
                status="challenged",
                verdict="challenge",
                unlocked=False,
                confidence="verified",
                evidence=ProviderEvidence(
                    http_status=403,
                    final_host=final_host,
                    signals=["challenge_detected"],
                    elapsed_ms=elapsed_ms,
                ).to_dict(),
                label="被风控或阻断",
            )

        text = resp.text
        if resp.status_code == 400:
            if "API_KEY_INVALID" in text or "API key not valid" in text:
                return ProviderResult(
                    status="verified",
                    verdict="available",
                    unlocked=True,
                    confidence="verified",
                    evidence=ProviderEvidence(
                        http_status=400,
                        final_host=final_host,
                        signals=["geofence_passed", "api_key_invalid"],
                        elapsed_ms=elapsed_ms,
                    ).to_dict(),
                    label="可用",
                )
            if "FAILED_PRECONDITION" in text or "User location is not supported" in text:
                return ProviderResult(
                    status="restricted",
                    verdict="unsupported_region",
                    unlocked=False,
                    confidence="verified",
                    evidence=ProviderEvidence(
                        http_status=400,
                        final_host=final_host,
                        signals=["user_location_unsupported"],
                        elapsed_ms=elapsed_ms,
                    ).to_dict(),
                    label="未支持地区",
                )

        if resp.status_code == 200:
            return ProviderResult(
                status="verified",
                verdict="available",
                unlocked=True,
                confidence="verified",
                evidence=ProviderEvidence(
                    http_status=200,
                    final_host=final_host,
                    signals=["api_available"],
                    elapsed_ms=elapsed_ms,
                ).to_dict(),
                label="可用",
            )

        # Auxiliary Web Portal check if primary API inconclusive and deadline permits
        remaining = deadline - time.monotonic()
        if remaining > 0.1:
            try:
                portal_resp = await execute_request_with_retry(
                    client,
                    "GET",
                    AISTUDIO_PORTAL_URL,
                    deadline_monotonic=deadline,
                    follow_redirects=True,
                )
                elapsed_ms = int((time.monotonic() - started) * 1000)
                portal_host = portal_resp.url.host if portal_resp.url else "aistudio.google.com"
                portal_url_str = str(portal_resp.url)
                portal_text = portal_resp.text

                if "sorry" in portal_url_str or "unavailable" in portal_url_str or "not supported" in portal_text.lower():
                    return ProviderResult(
                        status="restricted",
                        verdict="unsupported_region",
                        unlocked=False,
                        confidence="verified",
                        evidence=ProviderEvidence(
                            http_status=portal_resp.status_code,
                            final_host=portal_host,
                            signals=["portal_unsupported"],
                            elapsed_ms=elapsed_ms,
                        ).to_dict(),
                        label="未支持地区",
                    )
                if portal_resp.status_code in (200, 302, 303) and ("welcome" in portal_url_str or "aistudio" in portal_url_str or "MakerSuite" in portal_text):
                    return ProviderResult(
                        status="verified",
                        verdict="available",
                        unlocked=True,
                        confidence="verified",
                        evidence=ProviderEvidence(
                            http_status=portal_resp.status_code,
                            final_host=portal_host,
                            signals=["portal_available"],
                            elapsed_ms=elapsed_ms,
                        ).to_dict(),
                        label="可用",
                    )
            except Exception:
                pass

        elapsed_ms = int((time.monotonic() - started) * 1000)
        return ProviderResult(
            status="inconclusive",
            verdict="unknown",
            unlocked=False,
            confidence="unavailable",
            evidence=ProviderEvidence(
                http_status=resp.status_code,
                final_host=final_host,
                signals=["contract_drift"],
                elapsed_ms=elapsed_ms,
            ).to_dict(),
            label="未知",
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return _classify_error_outcome(exc, elapsed_ms, final_host)


async def eval_youtube_cdn(
    client: httpx.AsyncClient,
    timeout_s: float = 2.0,
    deadline_monotonic: float | None = None,
) -> CdnRoutingObservation:
    """Evaluate YouTube CDN mapping hint independently from node exit identity."""
    started = time.monotonic()
    deadline = min(deadline_monotonic, started + timeout_s) if deadline_monotonic is not None else (started + timeout_s)

    try:
        resp = await execute_request_with_retry(
            client,
            "GET",
            "https://redirector.googlevideo.com/report_mapping",
            deadline_monotonic=deadline,
        )
        elapsed_ms = int((time.monotonic() - started) * 1000)
        final_host = resp.url.host if resp.url else "redirector.googlevideo.com"

        if resp.status_code == 200:
            text = resp.text.strip()
            # Format: client_ip => edge_server (e.g. 198.51.100.22 => ord38s01-in-f1.1e100.net)
            match = re.search(r"=>\s*([a-zA-Z0-9.-]+)", text)
            if match:
                route_hint = match.group(1).strip()
                iata_match = re.search(r"^([a-zA-Z]{3})\d+", route_hint)
                iata_code = iata_match.group(1).upper() if iata_match else None
                return CdnRoutingObservation(
                    status="verified",
                    verdict="available",
                    confidence="verified",
                    route_hint=route_hint,
                    iata_code=iata_code,
                    checked_at=int(time.time()),
                    evidence_version=EVIDENCE_VERSION,
                    evidence=ProviderEvidence(
                        http_status=200,
                        final_host=final_host,
                        signals=["mapping_observed"],
                        elapsed_ms=elapsed_ms,
                    ).to_dict(),
                )

            return CdnRoutingObservation(
                status="inconclusive",
                verdict="unknown",
                confidence="unavailable",
                route_hint=None,
                iata_code=None,
                checked_at=int(time.time()),
                evidence_version=EVIDENCE_VERSION,
                evidence=ProviderEvidence(
                    http_status=200,
                    final_host=final_host,
                    signals=["contract_drift"],
                    elapsed_ms=elapsed_ms,
                ).to_dict(),
            )

        return CdnRoutingObservation(
            status="challenged" if resp.status_code == 403 else "inconclusive",
            verdict="challenge" if resp.status_code == 403 else "unknown",
            confidence="unavailable",
            route_hint=None,
            iata_code=None,
            checked_at=int(time.time()),
            evidence_version=EVIDENCE_VERSION,
            evidence=ProviderEvidence(
                http_status=resp.status_code,
                final_host=final_host,
                signals=["challenge_detected" if resp.status_code == 403 else "unexpected_status"],
                elapsed_ms=elapsed_ms,
            ).to_dict(),
        )

    except Exception as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        is_to = isinstance(exc, (asyncio.TimeoutError, httpx.TimeoutException))
        return CdnRoutingObservation(
            status="timeout" if is_to else "transport_error",
            verdict="unknown",
            confidence="unavailable",
            route_hint=None,
            iata_code=None,
            checked_at=int(time.time()),
            evidence_version=EVIDENCE_VERSION,
            evidence=ProviderEvidence(
                http_status=None,
                final_host="redirector.googlevideo.com",
                signals=[],
                elapsed_ms=elapsed_ms,
                error_code="timeout" if is_to else "transport_error",
            ).to_dict(),
            error=str(exc),
        )


# ---------------------------------------------------------------------------
# Exit Identity Consensus
# ---------------------------------------------------------------------------

async def _fetch_ip_sb(client: httpx.AsyncClient, deadline: float) -> dict[str, Any]:
    """Query ip.sb for IP, ISO country, ASN, and organization."""
    resp = await execute_request_with_retry(
        client,
        "GET",
        "https://api.ip.sb/geoip",
        deadline_monotonic=deadline,
    )
    if resp.status_code == 200:
        data = resp.json()
        return {
            "provider": "ip.sb",
            "status": 200,
            "ip": str(data.get("ip") or "").strip(),
            "country": str(data.get("country_code") or "").strip().upper(),
            "asn": data.get("asn"),
            "organization": str(data.get("organization") or "").strip(),
        }
    return {"provider": "ip.sb", "status": resp.status_code, "error": f"HTTP {resp.status_code}"}


async def _fetch_cloudflare_trace(client: httpx.AsyncClient, deadline: float) -> dict[str, Any]:
    """Query cloudflare.com trace for IP and ISO country."""
    resp = await execute_request_with_retry(
        client,
        "GET",
        "https://cloudflare.com/cdn-cgi/trace",
        deadline_monotonic=deadline,
    )
    if resp.status_code == 200:
        lines = resp.text.splitlines()
        trace_dict: dict[str, str] = {}
        for line in lines:
            if "=" in line:
                k, v = line.split("=", 1)
                trace_dict[k.strip()] = v.strip()
        return {
            "provider": "cloudflare",
            "status": 200,
            "ip": str(trace_dict.get("ip") or "").strip(),
            "country": str(trace_dict.get("loc") or "").strip().upper(),
            "asn": None,
            "organization": "Cloudflare Egress",
        }
    return {"provider": "cloudflare", "status": resp.status_code, "error": f"HTTP {resp.status_code}"}


async def eval_geo_identity_consensus(
    proxy_url: str,
    timeout_s: float = 2.0,
) -> IdentityObservation:
    """Evaluate egress identity requiring consensus between at least two independent providers.

    All outbound traffic is routed through proxy_url with trust_env=False.
    If providers agree on IP and ISO country -> status='verified', confidence='verified'.
    If providers disagree -> status='conflicted', confidence='conflicted' (no country claim).
    If only one provider succeeds -> status='unavailable', confidence='unavailable' (no country claim).
    If all fail -> status='timeout' or 'fail', confidence='unavailable'.
    """
    started = time.monotonic()
    deadline = started + timeout_s

    async def _safe_call(coro) -> dict[str, Any] | None:
        try:
            return await coro
        except Exception as exc:
            return {"error": str(exc), "status": 0}

    async with httpx.AsyncClient(
        proxy=proxy_url,
        trust_env=False,
        timeout=timeout_s,
        headers={"User-Agent": DEFAULT_UA},
    ) as client:
        res1, res2 = await asyncio.gather(
            _safe_call(_fetch_ip_sb(client, deadline)),
            _safe_call(_fetch_cloudflare_trace(client, deadline)),
            return_exceptions=False,
        )

    providers_evidence = []
    p1_ok = res1 and res1.get("status") == 200 and res1.get("ip") and res1.get("country")
    p2_ok = res2 and res2.get("status") == 200 and res2.get("ip") and res2.get("country")

    if res1:
        providers_evidence.append(res1)
    if res2:
        providers_evidence.append(res2)

    # 1. Both succeeded: check consensus
    if p1_ok and p2_ok and res1 is not None and res2 is not None:
        ip1 = res1["ip"]
        c1 = res1["country"]
        ip2 = res2["ip"]
        c2 = res2["country"]

        if ip1 == ip2 and c1 == c2:
            return IdentityObservation(
                status="verified",
                confidence="verified",
                ip=ip1,
                country=c1,
                asn=res1.get("asn"),
                organization=res1.get("organization"),
                checked_at=int(time.time()),
                identity_evidence={
                    "confidence": "verified",
                    "agreement": True,
                    "providers": providers_evidence,
                },
            )
        else:
            logger.info("Identity consensus conflicted between providers: ip.sb=(%s,%s) vs cloudflare=(%s,%s)", ip1, c1, ip2, c2)
            return IdentityObservation(
                status="conflicted",
                confidence="conflicted",
                ip=None,
                country=None,
                asn=None,
                organization=None,
                checked_at=int(time.time()),
                identity_evidence={
                    "confidence": "conflicted",
                    "agreement": False,
                    "providers": providers_evidence,
                },
            )

    # 2. Only one succeeded: quarantine as unavailable
    if p1_ok or p2_ok:
        return IdentityObservation(
            status="unavailable",
            confidence="unavailable",
            ip=None,
            country=None,
            asn=None,
            organization=None,
            checked_at=int(time.time()),
            identity_evidence={
                "confidence": "unavailable",
                "agreement": False,
                "providers": providers_evidence,
            },
            error="single provider succeeded; consensus not met",
        )

    # 3. Neither succeeded
    is_timeout = (time.monotonic() - started) >= timeout_s
    return IdentityObservation(
        status="timeout" if is_timeout else "fail",
        confidence="unavailable",
        ip=None,
        country=None,
        asn=None,
        organization=None,
        checked_at=int(time.time()),
        identity_evidence={
            "confidence": "unavailable",
            "agreement": False,
            "providers": providers_evidence,
        },
        error="geo identity lookup timed out" if is_timeout else "geo identity providers unavailable",
    )
