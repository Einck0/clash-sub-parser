"""Normalized data models for evidence-grade node capability probing.

Defines the taxonomy, bounded evidence structure, provider results,
identity observations, and CDN routing hints conforming to OpenSpec.
"""
from __future__ import annotations

import time
from dataclasses import asdict, dataclass, field
from typing import Any


STATUS_TAXONOMY = {
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

VERDICT_TAXONOMY = {
    "full",
    "originals_only",
    "available",
    "unsupported_region",
    "blocked",
    "challenge",
    "rate_limited",
    "unknown",
}

CONFIDENCE_TAXONOMY = {
    "verified",
    "conflicted",
    "unavailable",
}

ALLOWED_EVIDENCE_KEYS = {
    "http_status",
    "final_host",
    "redirect_class",
    "signals",
    "elapsed_ms",
    "error_code",
}


@dataclass(frozen=True)
class ProviderEvidence:
    """Bounded, sanitized evidence from external probe HTTP interactions."""

    http_status: int | None = None
    final_host: str | None = None
    redirect_class: str | None = None
    signals: list[str] = field(default_factory=list)
    elapsed_ms: int = 0
    error_code: str | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert evidence to a sanitized dictionary containing only allowed keys."""
        data = asdict(self)
        return {k: v for k, v in data.items() if k in ALLOWED_EVIDENCE_KEYS}


@dataclass
class ProviderResult:
    """Normalized evaluation result for a streaming or AI platform probe."""

    status: str
    verdict: str
    unlocked: bool
    region: str | None = None
    checked_at: int = field(default_factory=lambda: int(time.time()))
    evidence_version: str = "catalogue-2026-09-05"
    confidence: str = "verified"
    evidence: dict[str, Any] = field(default_factory=dict)
    label: str | None = None
    error: str | None = None

    def __post_init__(self) -> None:
        if self.status not in STATUS_TAXONOMY:
            raise ValueError(f"Invalid status: {self.status} not in {STATUS_TAXONOMY}")
        if self.verdict not in VERDICT_TAXONOMY:
            raise ValueError(f"Invalid verdict: {self.verdict} not in {VERDICT_TAXONOMY}")
        if self.confidence not in CONFIDENCE_TAXONOMY:
            raise ValueError(f"Invalid confidence: {self.confidence} not in {CONFIDENCE_TAXONOMY}")

    def to_dict(self) -> dict[str, Any]:
        """Serialize to dictionary while maintaining legacy-compatible fields."""
        res: dict[str, Any] = {
            "status": self.status,
            "verdict": self.verdict,
            "unlocked": self.unlocked,
            "region": self.region,
            "checked_at": self.checked_at,
            "evidence_version": self.evidence_version,
            "confidence": self.confidence,
            "evidence": self.evidence,
        }
        if self.label is not None:
            res["label"] = self.label
        if self.error is not None:
            res["error"] = self.error
        return res


@dataclass
class IdentityObservation:
    """Consensus-based egress identity and geolocation observation."""

    status: str
    confidence: str
    ip: str | None = None
    country: str | None = None
    asn: int | None = None
    organization: str | None = None
    checked_at: int = field(default_factory=lambda: int(time.time()))
    identity_evidence: dict[str, Any] = field(default_factory=dict)
    error: str | None = None

    def __post_init__(self) -> None:
        if self.confidence not in CONFIDENCE_TAXONOMY:
            raise ValueError(f"Invalid confidence: {self.confidence} not in {CONFIDENCE_TAXONOMY}")

    def to_dict(self) -> dict[str, Any]:
        """Serialize to dictionary compatible with node probe identity fields."""
        res: dict[str, Any] = {
            "status": self.status,
            "confidence": self.confidence,
            "ip": self.ip or "",
            "country": self.country or "",
            "asn": self.asn,
            "organization": self.organization or "",
            "checked_at": self.checked_at,
            "identity_evidence": self.identity_evidence,
        }
        if self.error is not None:
            res["error"] = self.error
        return res


@dataclass
class CdnRoutingObservation:
    """Independent observation of CDN mapping / routing hints (e.g. YouTube CDN)."""

    status: str
    verdict: str
    confidence: str
    route_hint: str | None = None
    iata_code: str | None = None
    checked_at: int = field(default_factory=lambda: int(time.time()))
    evidence_version: str = "catalogue-2026-09-05"
    evidence: dict[str, Any] = field(default_factory=dict)
    error: str | None = None

    def to_dict(self) -> dict[str, Any]:
        """Serialize to dictionary."""
        res: dict[str, Any] = {
            "status": self.status,
            "verdict": self.verdict,
            "confidence": self.confidence,
            "route_hint": self.route_hint,
            "iata_code": self.iata_code,
            "checked_at": self.checked_at,
            "evidence_version": self.evidence_version,
            "evidence": self.evidence,
        }
        if self.error is not None:
            res["error"] = self.error
        return res
