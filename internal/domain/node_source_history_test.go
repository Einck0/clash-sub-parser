package domain_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestNodeSourceHistory_Validate(t *testing.T) {
	now := time.Now().UTC()
	subID := "sub-123"
	rev := int64(1)

	valid := domain.NodeSourceHistory{
		ID:                 "hist-1",
		NodeLogicalID:      "node-1",
		SubscriptionID:     &subID,
		SourceIdentity:     "sub:sub-123",
		SourceLabel:        "Provider A",
		ConnectionRevision: &rev,
		RelationState:      domain.RelationStateVerified,
		Cause:              domain.CauseLegacyImport,
		FirstObservedAt:    &now,
		LastObservedAt:     &now,
		EvidenceKind:       "legacy_archive_link",
		EvidenceKey:        "link:4",
		EvidenceJSON:       `{"archive_sha256":"abcdef123456","pk":4}`,
		CreatedAt:          now,
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid history to pass validation, got: %v", err)
	}

	// Missing NodeLogicalID
	invalid := valid
	invalid.NodeLogicalID = ""
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected error for empty NodeLogicalID")
	}

	// Invalid RelationState
	invalid = valid
	invalid.RelationState = "bogus_state"
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected error for invalid RelationState")
	}

	// Invalid Cause
	invalid = valid
	invalid.Cause = "bogus_cause"
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected error for invalid Cause")
	}

	// Secret in evidence_json must be rejected
	invalid = valid
	invalid.EvidenceJSON = `{"password":"secret-password-123"}`
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected error for secret password in evidence_json")
	}

	invalid = valid
	invalid.EvidenceJSON = `{"token":"secret-token-123"}`
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected error for token in evidence_json")
	}
}

func TestAggregateAttributionStatus(t *testing.T) {
	now := time.Now().UTC()
	subID := "sub-1"
	rev := int64(1)

	verifiedItem := domain.NodeSourceHistory{
		ID:                 "h1",
		NodeLogicalID:      "node-1",
		SubscriptionID:     &subID,
		SourceIdentity:     "sub:sub-1",
		SourceLabel:        "Sub 1",
		ConnectionRevision: &rev,
		RelationState:      domain.RelationStateVerified,
		Cause:              domain.CauseLegacyImport,
		CreatedAt:          now,
	}

	conflictItem := domain.NodeSourceHistory{
		ID:                 "h2",
		NodeLogicalID:      "node-1",
		SubscriptionID:     &subID,
		SourceIdentity:     "sub:sub-2",
		SourceLabel:        "Sub 2",
		ConnectionRevision: &rev,
		RelationState:      domain.RelationStateConflict,
		Cause:              domain.CauseUnresolved,
		CreatedAt:          now,
	}

	manualItem := domain.NodeSourceHistory{
		ID:                 "h3",
		NodeLogicalID:      "node-1",
		SubscriptionID:     nil,
		SourceIdentity:     "manual",
		SourceLabel:        "Manual Operator",
		ConnectionRevision: &rev,
		RelationState:      domain.RelationStateUnknown,
		Cause:              domain.CauseManualConfirmed,
		CreatedAt:          now,
	}

	// Rule 1: hasCurrentSources = true -> current
	if status := domain.AggregateAttributionStatus(true, nil); status != domain.AttributionStatusCurrent {
		t.Errorf("expected current, got %s", status)
	}
	if status := domain.AggregateAttributionStatus(true, []domain.NodeSourceHistory{conflictItem}); status != domain.AttributionStatusCurrent {
		t.Errorf("expected current even with history, got %s", status)
	}

	// Rule 2: hasVerified = true -> historical_verified, even if conflict also exists
	if status := domain.AggregateAttributionStatus(false, []domain.NodeSourceHistory{verifiedItem}); status != domain.AttributionStatusHistoricalVerified {
		t.Errorf("expected historical_verified, got %s", status)
	}
	if status := domain.AggregateAttributionStatus(false, []domain.NodeSourceHistory{conflictItem, verifiedItem}); status != domain.AttributionStatusHistoricalVerified {
		t.Errorf("expected historical_verified to take precedence over conflict, got %s", status)
	}

	// Rule 3: manual_confirmed
	if status := domain.AggregateAttributionStatus(false, []domain.NodeSourceHistory{manualItem}); status != domain.AttributionStatusManualConfirmed {
		t.Errorf("expected manual_confirmed, got %s", status)
	}

	// Rule 4: conflict
	if status := domain.AggregateAttributionStatus(false, []domain.NodeSourceHistory{conflictItem}); status != domain.AttributionStatusConflict {
		t.Errorf("expected conflict, got %s", status)
	}

	// Rule 5: unknown
	if status := domain.AggregateAttributionStatus(false, nil); status != domain.AttributionStatusUnknown {
		t.Errorf("expected unknown, got %s", status)
	}
}
