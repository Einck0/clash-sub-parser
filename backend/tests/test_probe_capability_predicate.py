"""Tests for centralized verified-only capability predicate and filter compatibility.

Covers Task 2.2:
- Shared pure predicate is_media_full_unlocked
- Historical full and ok pass
- Netflix originals / originals_only, restricted, challenged, rate-limited,
  timeout, transport_error, and inconclusive fail full-unlock filters
- Integration with is_node_capability_qualified, canonical_compiler, and
  eligibility_evaluator.
"""
from __future__ import annotations

from app.services.canonical_compiler import _is_qualified_for_policy
from app.services.node_identity import canonical_node_key
from app.services.probe.eligibility_evaluator import evaluate_observation_eligibility
from app.utils.capability_filter import (
    filter_nodes_by_capabilities,
    is_media_full_unlocked,
    is_node_capability_qualified,
)


class TestMediaFullUnlockedPredicate:
    """Unit tests for the centralized is_media_full_unlocked predicate."""

    def test_empty_or_none(self):
        assert is_media_full_unlocked(None) is False
        assert is_media_full_unlocked({}) is False
        assert is_media_full_unlocked([]) is False
        assert is_media_full_unlocked("") is False

    def test_legacy_full_passes(self):
        assert is_media_full_unlocked({"status": "full", "region": "HK"}) is True
        assert is_media_full_unlocked({"status": "FULL"}) is True

    def test_legacy_ok_passes(self):
        assert is_media_full_unlocked({"status": "ok", "region": "US"}) is True
        assert is_media_full_unlocked({"status": "OK"}) is True

    def test_legacy_originals_fails_full_unlock(self):
        """Historical originals must not pass a full-unlock requirement."""
        assert is_media_full_unlocked({"status": "originals", "region": "HK"}) is False

    def test_legacy_fail_or_blocked_fails(self):
        assert is_media_full_unlocked({"status": "fail"}) is False
        assert is_media_full_unlocked({"status": "blocked"}) is False
        assert is_media_full_unlocked({"status": "unknown"}) is False

    def test_evidence_grade_verified_full_passes(self):
        result = {
            "status": "verified",
            "verdict": "full",
            "unlocked": True,
            "region": "US",
            "confidence": "verified",
            "evidence_version": "catalogue-2026-09-05",
            "evidence": {"http_status": 200, "signals": ["non_original_available", "original_available"]},
        }
        assert is_media_full_unlocked(result) is True

    def test_evidence_grade_verified_available_passes(self):
        result = {
            "status": "verified",
            "verdict": "available",
            "unlocked": True,
            "region": "US",
            "confidence": "verified",
            "evidence_version": "catalogue-2026-09-05",
            "evidence": {"http_status": 200, "signals": ["service_available"]},
        }
        assert is_media_full_unlocked(result) is True

    def test_evidence_grade_originals_only_fails(self):
        """Netflix partial / originals_only must fail full-unlock condition."""
        result = {
            "status": "partial",
            "verdict": "originals_only",
            "unlocked": False,
            "region": "US",
            "confidence": "verified",
            "evidence_version": "catalogue-2026-09-05",
            "evidence": {"http_status": 200, "signals": ["original_available"]},
        }
        assert is_media_full_unlocked(result) is False

    def test_evidence_grade_restricted_fails(self):
        result = {
            "status": "restricted",
            "verdict": "unsupported_region",
            "unlocked": False,
            "confidence": "verified",
        }
        assert is_media_full_unlocked(result) is False

    def test_evidence_grade_ip_blocked_fails(self):
        result = {
            "status": "ip_blocked",
            "verdict": "blocked",
            "unlocked": False,
            "confidence": "verified",
        }
        assert is_media_full_unlocked(result) is False

    def test_evidence_grade_challenged_fails(self):
        result = {
            "status": "challenged",
            "verdict": "challenge",
            "unlocked": False,
            "confidence": "verified",
        }
        assert is_media_full_unlocked(result) is False

    def test_evidence_grade_rate_limited_fails(self):
        result = {
            "status": "rate_limited",
            "verdict": "rate_limited",
            "unlocked": False,
            "confidence": "verified",
        }
        assert is_media_full_unlocked(result) is False

    def test_evidence_grade_timeout_fails(self):
        result = {
            "status": "timeout",
            "verdict": "unknown",
            "unlocked": False,
            "confidence": "unavailable",
        }
        assert is_media_full_unlocked(result) is False

    def test_evidence_grade_transport_error_fails(self):
        result = {
            "status": "transport_error",
            "verdict": "unknown",
            "unlocked": False,
            "confidence": "unavailable",
        }
        assert is_media_full_unlocked(result) is False

    def test_evidence_grade_inconclusive_fails(self):
        """Contract drift / unrecognized response must never pass unlock filter."""
        result = {
            "status": "inconclusive",
            "verdict": "unknown",
            "unlocked": False,
            "confidence": "verified",
            "evidence": {"http_status": 200, "signals": ["unrecognized_page"]},
        }
        assert is_media_full_unlocked(result) is False

    def test_conflicted_or_unavailable_confidence_fails(self):
        # Even if verdict says full, conflicted confidence must fail
        result = {
            "status": "verified",
            "verdict": "full",
            "unlocked": True,
            "confidence": "conflicted",
        }
        assert is_media_full_unlocked(result) is False

        result_unavail = {
            "status": "verified",
            "verdict": "full",
            "unlocked": True,
            "confidence": "unavailable",
        }
        assert is_media_full_unlocked(result_unavail) is False


class TestCapabilityFilterIntegration:
    """Tests integration of centralized predicate with capability filtering consumers."""

    def test_is_node_capability_qualified_with_legacy_and_new_records(self):
        legacy_full = {
            "status": "ok",
            "media": {
                "netflix": {"status": "full", "region": "HK"},
                "youtube": {"status": "ok", "region": "HK"},
            },
        }
        legacy_originals = {
            "status": "ok",
            "media": {
                "netflix": {"status": "originals", "region": "HK"},
                "youtube": {"status": "ok", "region": "HK"},
            },
        }
        new_verified_full = {
            "status": "ok",
            "media": {
                "netflix": {
                    "status": "verified",
                    "verdict": "full",
                    "unlocked": True,
                    "confidence": "verified",
                },
            },
        }
        new_originals_only = {
            "status": "ok",
            "media": {
                "netflix": {
                    "status": "partial",
                    "verdict": "originals_only",
                    "unlocked": False,
                    "confidence": "verified",
                },
            },
        }
        new_inconclusive = {
            "status": "ok",
            "media": {
                "netflix": {
                    "status": "inconclusive",
                    "verdict": "unknown",
                    "unlocked": False,
                },
            },
        }

        # Legacy full passes Netflix
        assert is_node_capability_qualified(legacy_full, required_media=["netflix"]) is True
        # Legacy originals FAILS Netflix
        assert is_node_capability_qualified(legacy_originals, required_media=["netflix"]) is False
        # Legacy YouTube passes
        assert is_node_capability_qualified(legacy_originals, required_media=["youtube"]) is True

        # New verified full passes Netflix
        assert is_node_capability_qualified(new_verified_full, required_media=["netflix"]) is True
        # New originals-only FAILS Netflix
        assert is_node_capability_qualified(new_originals_only, required_media=["netflix"]) is False
        # New inconclusive FAILS Netflix
        assert is_node_capability_qualified(new_inconclusive, required_media=["netflix"]) is False

    def test_filter_nodes_by_capabilities_excludes_non_full(self):
        nodes = [
            {"name": "Node-A-Full", "server": "1.1.1.1", "port": 443, "type": "ss"},
            {"name": "Node-B-Originals", "server": "2.2.2.2", "port": 443, "type": "ss"},
            {"name": "Node-C-Challenged", "server": "3.3.3.3", "port": 443, "type": "ss"},
        ]
        probe_map = {
            canonical_node_key(nodes[0]): {
                "status": "ok",
                "media": {"netflix": {"status": "verified", "verdict": "full", "unlocked": True, "confidence": "verified"}},
            },
            canonical_node_key(nodes[1]): {
                "status": "ok",
                "media": {"netflix": {"status": "partial", "verdict": "originals_only", "unlocked": False, "confidence": "verified"}},
            },
            canonical_node_key(nodes[2]): {
                "status": "ok",
                "media": {"netflix": {"status": "challenged", "verdict": "challenge", "unlocked": False, "confidence": "verified"}},
            },
        }
        filtered = filter_nodes_by_capabilities(nodes, probe_map, required_media=["netflix"])
        assert len(filtered) == 1
        assert filtered[0]["name"] == "Node-A-Full"

    def test_canonical_compiler_policy_qualification(self):
        node = {"name": "TestNode"}
        obs_full = {
            "status": "ok",
            "media": {"netflix": {"status": "verified", "verdict": "full", "unlocked": True, "confidence": "verified"}},
        }
        obs_orig = {
            "status": "ok",
            "media": {"netflix": {"status": "partial", "verdict": "originals_only", "unlocked": False, "confidence": "verified"}},
        }
        obs_legacy_orig = {
            "status": "ok",
            "media": {"netflix": {"status": "originals"}},
        }

        assert _is_qualified_for_policy(node, obs_full, min_speed_mbps=None, required_media=["netflix"]) is True
        assert _is_qualified_for_policy(node, obs_orig, min_speed_mbps=None, required_media=["netflix"]) is False
        assert _is_qualified_for_policy(node, obs_legacy_orig, min_speed_mbps=None, required_media=["netflix"]) is False

    def test_eligibility_evaluator_integration(self):
        obs_full = {
            "job_id": 1,
            "node_id": 1,
            "status": "ok",
            "media_results": {
                "netflix": {"status": "verified", "verdict": "full", "unlocked": True, "confidence": "verified"}
            },
        }
        obs_orig = {
            "job_id": 1,
            "node_id": 1,
            "status": "ok",
            "media_results": {
                "netflix": {"status": "partial", "verdict": "originals_only", "unlocked": False, "confidence": "verified"}
            },
        }
        obs_legacy_orig = {
            "job_id": 1,
            "node_id": 1,
            "status": "ok",
            "media_results": {
                "netflix": {"status": "originals"}
            },
        }

        res_full = evaluate_observation_eligibility(obs_full, required_media=["netflix"])
        assert res_full.eligible is True
        assert res_full.status == "valid"

        res_orig = evaluate_observation_eligibility(obs_orig, required_media=["netflix"])
        assert res_orig.eligible is False
        assert res_orig.status == "insufficient"

        res_legacy_orig = evaluate_observation_eligibility(obs_legacy_orig, required_media=["netflix"])
        assert res_legacy_orig.eligible is False
        assert res_legacy_orig.status == "insufficient"
