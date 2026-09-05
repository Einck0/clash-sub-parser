"""Tests for exit identity consensus and YouTube CDN-routing independence.

Validates multi-provider geo consensus (at least two independent providers
agree on IP and ISO country), disagreement reporting, one-unavailable quarantine,
and CDN routing hint isolation without host fallback.
"""
from __future__ import annotations

import asyncio
from pathlib import Path
from unittest.mock import AsyncMock, patch
import httpx
import pytest

from app.services.probe.catalogue import eval_geo_identity_consensus, eval_youtube_cdn
from app.services.probe.providers import check_geo_identity


FIXTURES_DIR = Path(__file__).parent / "fixtures" / "probe_catalogue"


def _read_fixture(filename: str) -> str:
    return (FIXTURES_DIR / filename).read_text(encoding="utf-8")


def _build_mock_response(status_code: int, text: str, url: str = "https://example.com") -> httpx.Response:
    request = httpx.Request("GET", url)
    return httpx.Response(status_code, text=text, request=request)


@pytest.mark.asyncio
async def test_identity_consensus_agreement():
    ip_sb_text = _read_fixture("ip_sb_geoip.json")
    cf_trace_text = _read_fixture("cloudflare_trace.txt")

    async def mock_fetch(url, *args, **kwargs):
        if "ip.sb" in str(url):
            return _build_mock_response(200, text=ip_sb_text, url=str(url))
        elif "cloudflare.com" in str(url):
            return _build_mock_response(200, text=cf_trace_text, url=str(url))
        raise RuntimeError(f"Unexpected url: {url}")

    with patch("httpx.AsyncClient.get", side_effect=mock_fetch):
        obs = await eval_geo_identity_consensus("http://127.0.0.1:21000", timeout_s=2.0)
        d = obs.to_dict()

        assert d["status"] == "verified"
        assert d["confidence"] == "verified"
        assert d["ip"] == "198.51.100.22"
        assert d["country"] == "US"
        assert d["asn"] == 13335
        assert d["organization"] == "Cloudflare Inc."
        assert d["identity_evidence"]["agreement"] is True
        assert len(d["identity_evidence"]["providers"]) == 2

        # Verify legacy check_geo_identity compatibility
        legacy = await check_geo_identity("http://127.0.0.1:21000", timeout_s=2.0)
        assert legacy["ip"] == "198.51.100.22"
        assert legacy["country"] == "US"
        assert legacy["asn"] == 13335
        assert legacy["organization"] == "Cloudflare Inc."
        assert legacy["confidence"] == "verified"


@pytest.mark.asyncio
async def test_identity_consensus_disagreement_on_country():
    ip_sb_text = _read_fixture("ip_sb_geoip.json")  # US
    cf_trace_mismatch = _read_fixture("cloudflare_trace_mismatch.txt")  # GB

    async def mock_fetch(url, *args, **kwargs):
        if "ip.sb" in str(url):
            return _build_mock_response(200, text=ip_sb_text, url=str(url))
        elif "cloudflare.com" in str(url):
            return _build_mock_response(200, text=cf_trace_mismatch, url=str(url))
        raise RuntimeError(f"Unexpected url: {url}")

    with patch("httpx.AsyncClient.get", side_effect=mock_fetch):
        obs = await eval_geo_identity_consensus("http://127.0.0.1:21000", timeout_s=2.0)
        d = obs.to_dict()

        # Must report conflicted, MUST NOT claim a confirmed country
        assert d["status"] == "conflicted"
        assert d["confidence"] == "conflicted"
        assert not d["country"]
        assert not d["ip"]
        assert d["asn"] is None
        assert d["identity_evidence"]["agreement"] is False

        # Legacy check_geo_identity must not provide false country
        legacy = await check_geo_identity("http://127.0.0.1:21000", timeout_s=2.0)
        assert legacy["status"] == "conflicted"
        assert not legacy["country"]
        assert not legacy["ip"]


@pytest.mark.asyncio
async def test_identity_consensus_one_unavailable_yields_no_verified_country():
    ip_sb_text = _read_fixture("ip_sb_geoip.json")

    async def mock_fetch(url, *args, **kwargs):
        if "ip.sb" in str(url):
            return _build_mock_response(200, text=ip_sb_text, url=str(url))
        elif "cloudflare.com" in str(url):
            raise httpx.ConnectTimeout("Cloudflare trace timed out")
        raise RuntimeError(f"Unexpected url: {url}")

    with patch("httpx.AsyncClient.get", side_effect=mock_fetch):
        obs = await eval_geo_identity_consensus("http://127.0.0.1:21000", timeout_s=2.0)
        d = obs.to_dict()

        # Single provider cannot claim verified consensus
        assert d["status"] == "unavailable"
        assert d["confidence"] == "unavailable"
        assert not d["country"]
        assert not d["ip"]

        legacy = await check_geo_identity("http://127.0.0.1:21000", timeout_s=2.0)
        assert legacy["status"] in ("unavailable", "fail")
        assert not legacy["country"]


@pytest.mark.asyncio
async def test_identity_consensus_both_unavailable():
    async def mock_fetch(url, *args, **kwargs):
        raise httpx.ConnectError("Connection refused by proxy")

    with patch("httpx.AsyncClient.get", side_effect=mock_fetch):
        obs = await eval_geo_identity_consensus("http://127.0.0.1:21000", timeout_s=2.0)
        d = obs.to_dict()

        assert d["status"] in ("fail", "unavailable", "transport_error")
        assert d["confidence"] == "unavailable"
        assert not d["country"]
        assert not d["ip"]


@pytest.mark.asyncio
async def test_cdn_routing_mismatch_never_overwrites_exit_identity():
    # Egress identity agrees on JP
    ip_sb_jp = '{"ip": "203.0.113.99", "country_code": "JP", "asn": 2516, "organization": "KDDI"}'
    cf_trace_jp = "ip=203.0.113.99\nloc=JP\nts=1788546780\nvisit_scheme=https\n"

    # YouTube CDN reports ORD (Chicago / US)
    yt_cdn_text = "203.0.113.99 => ord38s01-in-f1.1e100.net"

    async def mock_fetch(url, *args, **kwargs):
        url_str = str(url)
        if "ip.sb" in url_str:
            return _build_mock_response(200, text=ip_sb_jp, url=url_str)
        elif "cloudflare.com" in url_str:
            return _build_mock_response(200, text=cf_trace_jp, url=url_str)
        elif "googlevideo.com" in url_str:
            return _build_mock_response(200, text=yt_cdn_text, url=url_str)
        raise RuntimeError(f"Unexpected url: {url_str}")

    with patch("httpx.AsyncClient.get", side_effect=mock_fetch):
        # 1. Evaluate Geo consensus
        geo = await eval_geo_identity_consensus("http://127.0.0.1:21000", timeout_s=2.0)
        assert geo.country == "JP"
        assert geo.confidence == "verified"

        # 2. Evaluate YouTube CDN routing hint
        async with httpx.AsyncClient(proxy="http://127.0.0.1:21000", trust_env=False) as client:
            cdn = await eval_youtube_cdn(client)

        assert cdn.iata_code == "ORD"
        assert cdn.route_hint == "ord38s01-in-f1.1e100.net"

        # 3. CDN observation must NOT overwrite exit identity country
        assert geo.country == "JP"
        assert cdn.iata_code != geo.country
