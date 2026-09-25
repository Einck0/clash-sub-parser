package platform

import (
	"testing"
)

func TestPlatformMetadata(t *testing.T) {
	if Version == "" {
		t.Fatal("expected Version to be non-empty")
	}
	if Commit == "" {
		t.Fatal("expected Commit to be non-empty")
	}
	if BuildDate == "" {
		t.Fatal("expected BuildDate to be non-empty")
	}
}
