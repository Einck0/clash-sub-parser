package pipeline_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/pipeline"
)

type mockBaselineVerifier struct {
	available bool
}

func (m *mockBaselineVerifier) CheckHostConnectivity(ctx context.Context) bool {
	return m.available
}

func TestBaselineVerifier_HostOnline(t *testing.T) {
	v := &mockBaselineVerifier{available: true}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if !v.CheckHostConnectivity(ctx) {
		t.Errorf("expected host online")
	}
}

func TestBaselineVerifier_HostOffline(t *testing.T) {
	v := &mockBaselineVerifier{available: false}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if v.CheckHostConnectivity(ctx) {
		t.Errorf("expected host offline")
	}
}

func TestDefaultBaselineVerifier_Creation(t *testing.T) {
	dv := pipeline.NewDefaultBaselineVerifier(500 * time.Millisecond)
	if dv == nil {
		t.Fatalf("expected non-nil default baseline verifier")
	}
}
