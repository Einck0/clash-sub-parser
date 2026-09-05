"""Tests for additive persistence, historical compatibility, API regression, and secret redaction.

Covers:
- Task 2.1: Additive persistence without identity-key rewrite or historical data loss.
- Task 2.2: Legacy and new capability compatibility in persistence round-trip.
- Task 2.3: Route/schema/API regression and strict secret redaction scan.
"""
from __future__ import annotations

import json
import re
from typing import Any
import pytest
from httpx import ASGITransport, AsyncClient
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.main import app
from app.models.node_probe_result import NodeProbeResult
from app.services.probe.models import ALLOWED_EVIDENCE_KEYS
from app.services.probe.service import (
    get_all_db_probe_results,
    merge_probe_media,
    sanitize_probe_evidence,
    save_probe_result_to_db,
    save_probe_results_batch_to_db,
)


def _scan_for_secrets(obj: Any, path: str = "") -> list[str]:
    """Recursively scan an object for secret leakage (headers, tokens, cookies, bodies)."""
    violations: list[str] = []
    forbidden_keys = {
        "cookie",
        "cookies",
        "authorization",
        "proxy-authorization",
        "token",
        "access_token",
        "refresh_token",
        "secret",
        "password",
        "raw_body",
        "body",
        "response_body",
    }

    if isinstance(obj, dict):
        for k, v in obj.items():
            k_lower = str(k).lower().strip()
            current_path = f"{path}.{k}" if path else str(k)

            # Check forbidden keys
            if k_lower in forbidden_keys:
                violations.append(f"Forbidden secret key found at {current_path}: {k}")

            # Check evidence allowlist if inside an 'evidence' block
            if k_lower == "evidence" and isinstance(v, dict):
                for ev_k in v.keys():
                    if ev_k not in ALLOWED_EVIDENCE_KEYS:
                        violations.append(
                            f"Non-allowlisted evidence key '{ev_k}' at {current_path}"
                        )

            # Check forbidden values
            if isinstance(v, str):
                if re.search(r"Bearer\s+[a-zA-Z0-9_\-\.]{10,}", v, re.IGNORECASE):
                    violations.append(f"Bearer token pattern found in {current_path}")
                if re.search(r"https?://[^:]+:[^@]+@", v):
                    violations.append(f"Credential-bearing URL found in {current_path}")

            violations.extend(_scan_for_secrets(v, current_path))

    elif isinstance(obj, list):
        for idx, item in enumerate(obj):
            violations.extend(_scan_for_secrets(item, f"{path}[{idx}]"))

    return violations


@pytest.mark.asyncio
class TestProbeAdditivePersistence:
    """Task 2.1: Additive persistence and historical compatibility."""

    async def test_merge_probe_media_structural_additive(self):
        """Structural additive merge retains existing platforms and updates probed platforms."""
        existing = {
            "youtube": {"status": "ok", "region": "US"},
            "custom_legacy": {"status": "full", "label": "Legacy"},
        }
        new_obs = {
            "netflix": {
                "status": "verified",
                "verdict": "full",
                "unlocked": True,
                "region": "US",
                "confidence": "verified",
                "evidence_version": "catalogue-2026-09-05",
                "evidence": {
                    "http_status": 200,
                    "final_host": "www.netflix.com",
                    "signals": ["non_original_available", "original_available"],
                    "elapsed_ms": 120,
                },
            }
        }

        merged = merge_probe_media(existing, new_obs)

        # Existing platforms must be preserved
        assert "youtube" in merged
        assert merged["youtube"]["status"] == "ok"
        assert merged["custom_legacy"]["status"] == "full"

        # New platform is added with full evidence
        assert "netflix" in merged
        assert merged["netflix"]["status"] == "verified"
        assert merged["netflix"]["verdict"] == "full"
        assert merged["netflix"]["evidence"]["http_status"] == 200

    async def test_mixed_historical_and_new_round_trip_in_db(self):
        """Simulate existing historical DB row updated with new evidence-grade probe."""
        # Use app's test DB session
        async for db in get_db():
            node_key = "Test-Mixed-Node|ss|192.0.2.1:8388"

            # 1. Clean up any existing row for this node_key
            existing_row = (
                await db.execute(select(NodeProbeResult).where(NodeProbeResult.node_key == node_key))
            ).scalar_one_or_none()
            if existing_row:
                await db.delete(existing_row)
                await db.commit()

            # 2. Insert historical record with old simple media dictionary
            historical_result = {
                "node_key": node_key,
                "name": "Test-Mixed-Node",
                "server": "192.0.2.1",
                "port": 8388,
                "type": "ss",
                "status": "ok",
                "latency_ms": 45,
                "speed_mbps": 22.0,
                "ip": "192.0.2.1",
                "country": "HK",
                "media": {
                    "youtube": {"status": "ok", "region": "HK"},
                    "bilibili": {"status": "ok", "region": "HK"},
                },
            }
            await save_probe_result_to_db(db, historical_result)

            # Check inserted record
            db_map = await get_all_db_probe_results(db)
            assert node_key in db_map
            orig_row_id = (
                await db.execute(select(NodeProbeResult.id).where(NodeProbeResult.node_key == node_key))
            ).scalar_one()

            # 3. Perform a new probe that updates Netflix with evidence-grade outcome
            new_probe_result = {
                "node_key": node_key,
                "name": "Test-Mixed-Node",
                "server": "192.0.2.1",
                "port": 8388,
                "type": "ss",
                "status": "ok",
                "latency_ms": 40,
                "speed_mbps": 25.0,
                "ip": "192.0.2.1",
                "country": "HK",
                "media": {
                    "netflix": {
                        "status": "verified",
                        "verdict": "full",
                        "unlocked": True,
                        "region": "HK",
                        "confidence": "verified",
                        "evidence_version": "catalogue-2026-09-05",
                        "evidence": {
                            "http_status": 200,
                            "final_host": "www.netflix.com",
                            "signals": ["non_original_available", "original_available"],
                            "elapsed_ms": 110,
                        },
                    }
                },
            }
            await save_probe_result_to_db(db, new_probe_result)

            # 4. Read back and verify
            updated_map = await get_all_db_probe_results(db)
            node_data = updated_map[node_key]

            # Same database row ID (no deletion/re-creation)
            current_row_id = (
                await db.execute(select(NodeProbeResult.id).where(NodeProbeResult.node_key == node_key))
            ).scalar_one()
            assert current_row_id == orig_row_id

            # Node identity key and parameters remain intact
            assert node_data["node_key"] == node_key
            assert node_data["name"] == "Test-Mixed-Node"
            assert node_data["server"] == "192.0.2.1"
            assert node_data["port"] == 8388
            assert node_data["type"] == "ss"

            # Historical platforms are preserved
            media = node_data["media"]
            assert "youtube" in media
            assert media["youtube"]["status"] == "ok"
            assert "bilibili" in media
            assert media["bilibili"]["status"] == "ok"

            # New platform is present with additive fields
            assert "netflix" in media
            assert media["netflix"]["status"] == "verified"
            assert media["netflix"]["verdict"] == "full"
            assert media["netflix"]["evidence_version"] == "catalogue-2026-09-05"
            assert media["netflix"]["evidence"]["http_status"] == 200

            # Cleanup
            del_row = (
                await db.execute(select(NodeProbeResult).where(NodeProbeResult.node_key == node_key))
            ).scalar_one_or_none()
            if del_row:
                await db.delete(del_row)
                await db.commit()
            break

    async def test_identity_and_cdn_evidence_does_not_overwrite_node_key_or_params(self):
        """CDN observation and identity consensus must never overwrite node identity key."""
        async for db in get_db():
            node_key = "Node-Consensus|ss|203.0.113.10:443"

            result = {
                "node_key": node_key,
                "name": "Node-Consensus",
                "server": "203.0.113.10",
                "port": 443,
                "type": "ss",
                "status": "ok",
                "ip": "203.0.113.10",
                "country": "JP",
                "media": {
                    "youtube_cdn": {
                        "status": "verified",
                        "verdict": "available",
                        "confidence": "verified",
                        "iata_code": "ORD",
                        "route_hint": "ord38s01.1e100.net",
                        "evidence": {"http_status": 200, "signals": ["cdn_mapping"]},
                    }
                },
            }
            await save_probe_result_to_db(db, result)

            db_map = await get_all_db_probe_results(db)
            saved = db_map[node_key]

            # Country must remain exit identity (JP), not CDN airport code (ORD)
            assert saved["country"] == "JP"
            assert saved["server"] == "203.0.113.10"
            assert saved["node_key"] == node_key
            assert saved["media"]["youtube_cdn"]["iata_code"] == "ORD"

            # Clean up
            del_row = (
                await db.execute(select(NodeProbeResult).where(NodeProbeResult.node_key == node_key))
            ).scalar_one_or_none()
            if del_row:
                await db.delete(del_row)
                await db.commit()
            break


class TestProbeSecretRedactionAndAPIRegression:
    """Task 2.3: Secret redaction scan and API regression coverage."""

    def test_sanitize_probe_evidence_strips_forbidden_artifacts(self):
        """Sanitizer must remove cookies, authorization headers, passwords, and raw bodies."""
        raw_evidence_payload = {
            "status": "verified",
            "verdict": "full",
            "unlocked": True,
            "region": "US",
            "cookie": "session_id=abcdef123456",
            "Authorization": "Bearer eyJhbGciOi...",
            "password": "supersecretpassword",
            "response_body": "<html>Netflix content</html>",
            "evidence": {
                "http_status": 200,
                "final_host": "www.netflix.com",
                "signals": ["non_original_available"],
                "elapsed_ms": 150,
                # Forbidden items inside evidence
                "headers": {"Authorization": "Bearer secret"},
                "cookies": ["uid=9999"],
                "body": "raw sensitive text",
            },
            "error": "Error connecting to http://user:secret123@proxy.example.com:8080",
        }

        sanitized = sanitize_probe_evidence(raw_evidence_payload)

        # Forbidden keys removed at top level
        assert "cookie" not in sanitized
        assert "Authorization" not in sanitized
        assert "password" not in sanitized
        assert "response_body" not in sanitized

        # Evidence dictionary strictly adheres to ALLOWED_EVIDENCE_KEYS
        ev = sanitized["evidence"]
        for k in ev.keys():
            assert k in ALLOWED_EVIDENCE_KEYS
        assert "headers" not in ev
        assert "cookies" not in ev
        assert "body" not in ev

        # Error string credential scrubbed
        assert "secret123" not in sanitized["error"]
        assert "[REDACTED]" in sanitized["error"]

        # Run recursive secret scanner
        violations = _scan_for_secrets(sanitized)
        assert not violations, f"Secret violations found: {violations}"

    @pytest.mark.asyncio
    async def test_api_results_endpoint_backward_compatible_and_redacted(self):
        """Verify GET /api/probe/results returns valid data without any secrets and paged envelope."""
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
            res = await ac.get("/api/probe/results")
            assert res.status_code == 200
            data = res.json()
            assert isinstance(data, dict)
            assert "results" in data
            assert "next_cursor" in data
            assert "has_more" in data
            assert isinstance(data["results"], dict)

            # Secret redaction scan across all returned items
            violations = _scan_for_secrets(data)
            assert not violations, f"API response contains forbidden secret artifacts: {violations}"

            # Detail route secret scan
            if data["results"]:
                first_key = next(iter(data["results"].keys()))
                import urllib.parse
                detail_res = await ac.get(f"/api/probe/results/detail?node_key={urllib.parse.quote(first_key)}")
                if detail_res.status_code == 200:
                    detail_data = detail_res.json()
                    detail_violations = _scan_for_secrets(detail_data)
                    assert not detail_violations, f"Detail API response leaked secrets: {detail_violations}"

    @pytest.mark.asyncio
    async def test_api_single_node_probe_request_shape_preserved(self):
        """Verify POST /api/probe/node works with original request shape and returns additive fields."""
        node_payload = {
            "node": {
                "name": "API-Test-Node",
                "server": "192.0.2.55",
                "port": 8388,
                "type": "ss",
                "cipher": "aes-128-gcm",
                "password": "testpassword",
            },
            "include_media": False,
            "include_speed": False,
            "use_cache": False,
        }

        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
            res = await ac.post("/api/probe/node", json=node_payload)
            assert res.status_code == 200
            data = res.json()

            # Original response fields preserved
            for required_field in ("node_key", "name", "server", "port", "type", "status", "media", "checked_at"):
                assert required_field in data

            # Secret redaction scan
            violations = _scan_for_secrets(data)
            assert not violations, f"Single node probe response leaked secrets: {violations}"
