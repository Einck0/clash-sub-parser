package resolver

import (
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestEvaluateNodeAdmission_LegacyRulesRetired_ActiveNodesAdmitted(t *testing.T) {
	// FR-C3: evaluateNodeAdmission retains signature (nodes, admissionRules, excludedNodeIDs)
	// but admission rule DSL matching is completely retired.
	// Active nodes are never excluded by legacy admission rules (reject, quarantine, etc.).
	// Inactive nodes and explicit risk exclusions remain strictly excluded.
	nodes := []domain.Node{
		{LogicalID: "node-active-1", DisplayName: "Active VMess HK", Protocol: domain.ProtocolVMess, Active: true},
		{LogicalID: "node-active-2", DisplayName: "Active Trojan US", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: "node-active-3", DisplayName: "Active SS JP", Protocol: domain.ProtocolSS, Active: true},
		{LogicalID: "node-inactive-4", DisplayName: "Inactive Node", Protocol: domain.ProtocolVMess, Active: false},
		{LogicalID: "node-risk-5", DisplayName: "Risk Excluded Node", Protocol: domain.ProtocolTrojan, Active: true},
	}

	legacyRules := []domain.AdmissionRule{
		{ID: "r1", Name: "legacy-reject-hk", Expression: "country == 'HK'", Action: domain.RuleActionReject, Position: 0},
		{ID: "r2", Name: "legacy-quarantine-us", Expression: "name contains 'US'", Action: domain.RuleActionQuarantine, Position: 1},
		{ID: "r3", Name: "legacy-allow-all", Expression: "*", Action: domain.RuleActionAllow, Position: 2},
	}

	excludedNodeIDs := []string{"node-risk-5"}

	res := evaluateNodeAdmission(nodes, legacyRules, excludedNodeIDs)

	// 1. Verify active nodes are admitted (legacy reject/quarantine ignored)
	if len(res.AdmittedNodes) != 3 {
		t.Fatalf("expected 3 admitted active nodes, got %d: %#v", len(res.AdmittedNodes), res.AdmittedNodes)
	}
	expectedAdmitted := map[string]bool{
		"node-active-1": true,
		"node-active-2": true,
		"node-active-3": true,
	}
	for _, n := range res.AdmittedNodes {
		if !expectedAdmitted[n.LogicalID] {
			t.Errorf("unexpected admitted node: %s", n.LogicalID)
		}
		if _, ok := res.AdmittedNodeMap[n.LogicalID]; !ok {
			t.Errorf("admitted node map missing %s", n.LogicalID)
		}
	}

	// 2. Verify hard exclusions: inactive and risk nodes
	if len(res.ExcludedNodeIDs) != 2 {
		t.Fatalf("expected 2 excluded node IDs, got %d: %v", len(res.ExcludedNodeIDs), res.ExcludedNodeIDs)
	}
	expectedExcluded := map[string]bool{
		"node-inactive-4": true,
		"node-risk-5":     true,
	}
	for _, id := range res.ExcludedNodeIDs {
		if !expectedExcluded[id] {
			t.Errorf("unexpected excluded node ID: %s", id)
		}
	}

	// 3. Verify exactly one diagnostic warning for legacy admission rules
	var legacyDiagCount int
	var inactiveDiagCount int
	var riskDiagCount int
	for _, d := range res.Diagnostics {
		switch d.Code {
		case "legacy_admission_rules_ignored":
			legacyDiagCount++
			if d.Severity != DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for legacy_admission_rules_ignored, got: %s", d.Severity)
			}
			// Diagnostic message MUST NOT leak expression text
			if strings.Contains(d.Message, "country == 'HK'") || strings.Contains(d.Message, "name contains 'US'") {
				t.Errorf("diagnostic message must not contain expression text, got: %s", d.Message)
			}
		case "node_inactive":
			inactiveDiagCount++
			if d.Target != "node-inactive-4" {
				t.Errorf("expected target node-inactive-4 for node_inactive, got: %s", d.Target)
			}
		case "node_excluded_by_risk_policy":
			riskDiagCount++
			if d.Target != "node-risk-5" {
				t.Errorf("expected target node-risk-5 for node_excluded_by_risk_policy, got: %s", d.Target)
			}
		default:
			t.Errorf("unexpected diagnostic code: %s", d.Code)
		}
	}

	if legacyDiagCount != 1 {
		t.Fatalf("expected exactly 1 legacy_admission_rules_ignored diagnostic, got %d", legacyDiagCount)
	}
	if inactiveDiagCount != 1 {
		t.Fatalf("expected exactly 1 node_inactive diagnostic, got %d", inactiveDiagCount)
	}
	if riskDiagCount != 1 {
		t.Fatalf("expected exactly 1 node_excluded_by_risk_policy diagnostic, got %d", riskDiagCount)
	}
}

func TestEvaluateNodeAdmission_EmptyRules_NoWarningDiagnostic(t *testing.T) {
	nodes := []domain.Node{
		{LogicalID: "node-1", DisplayName: "Node 1", Protocol: domain.ProtocolVMess, Active: true},
		{LogicalID: "node-2", DisplayName: "Node 2", Protocol: domain.ProtocolVMess, Active: false},
	}

	res := evaluateNodeAdmission(nodes, nil, nil)

	if len(res.AdmittedNodes) != 1 || res.AdmittedNodes[0].LogicalID != "node-1" {
		t.Fatalf("expected node-1 admitted, got %#v", res.AdmittedNodes)
	}
	if len(res.ExcludedNodeIDs) != 1 || res.ExcludedNodeIDs[0] != "node-2" {
		t.Fatalf("expected node-2 excluded, got %v", res.ExcludedNodeIDs)
	}

	for _, d := range res.Diagnostics {
		if d.Code == "legacy_admission_rules_ignored" {
			t.Errorf("legacy_admission_rules_ignored must not be emitted when admissionRules is empty")
		}
	}
}

func TestEvaluateNodeAdmission_DeterministicSorting(t *testing.T) {
	nodes := []domain.Node{
		{LogicalID: "z-node", DisplayName: "Z", Protocol: domain.ProtocolVMess, Active: true},
		{LogicalID: "a-node", DisplayName: "A", Protocol: domain.ProtocolVMess, Active: true},
		{LogicalID: "m-node", DisplayName: "M", Protocol: domain.ProtocolVMess, Active: true},
		{LogicalID: "y-inactive", DisplayName: "Y", Protocol: domain.ProtocolVMess, Active: false},
		{LogicalID: "b-inactive", DisplayName: "B", Protocol: domain.ProtocolVMess, Active: false},
	}

	res := evaluateNodeAdmission(nodes, nil, nil)

	if len(res.AdmittedNodeIDs) != 3 ||
		res.AdmittedNodeIDs[0] != "a-node" ||
		res.AdmittedNodeIDs[1] != "m-node" ||
		res.AdmittedNodeIDs[2] != "z-node" {
		t.Errorf("AdmittedNodeIDs not sorted ASC: %v", res.AdmittedNodeIDs)
	}

	if len(res.ExcludedNodeIDs) != 2 ||
		res.ExcludedNodeIDs[0] != "b-inactive" ||
		res.ExcludedNodeIDs[1] != "y-inactive" {
		t.Errorf("ExcludedNodeIDs not sorted ASC: %v", res.ExcludedNodeIDs)
	}
}
