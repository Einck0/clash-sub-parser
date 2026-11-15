package inventory

import (
	"encoding/json"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestParseErrorEntriesReadsPersistedPrefixVariants(t *testing.T) {
	payload := `{"rejected":1,"entries":[{"format":"yaml","entry":2,"protocol":"ss","reason":"missing_credential","field":"password"}]}`
	for _, prefix := range []string{
		"[stage:parse] ",
		"[stage:parse reason_code:parse_failed] [stage:parse] ",
		"[stage:parse reason_code:partial_subscription] [stage:parse] ",
		"[stage:parse reason_code:empty_subscription] [stage:parse] ",
		"[stage:parse reason_code:parse_failed] ",
	} {
		entries := parseErrorEntries(domain.SubscriptionFetch{ID: "fetch", SubscriptionID: "sub", RedactedError: prefix + payload})
		if len(entries) != 1 || entries[0].ID != "parse_fetch_0" || entries[0].NodeLogicalID != nil || entries[0].EntryKind != domain.EntryKindFormatError || !strings.Contains(entries[0].ClassificationReason, "fetch") {
			t.Fatalf("%s: %+v", prefix, entries)
		}
	}
}

func TestParseErrorEntriesBoundsAndSanitizesStoredPayload(t *testing.T) {
	payload := `[stage:parse] {"rejected":40,"entries":[{"format":"SECRET","entry":-1,"protocol":"SECRET","reason":"SECRET","field":"SECRET"}]}`
	entries := parseErrorEntries(domain.SubscriptionFetch{ID: "fetch", RedactedError: payload})
	encoded, _ := json.Marshal(entries)
	if len(entries) != 2 || strings.Contains(string(encoded), "SECRET") || entries[1].NodeLogicalID != nil {
		t.Fatalf("unsafe or lost truncated diagnostics: %s", encoded)
	}
	for _, invalid := range []string{"[stage:fetch] {}", "[stage:parse] not json", "[stage:parse] " + strings.Repeat("x", 65536)} {
		if got := parseErrorEntries(domain.SubscriptionFetch{RedactedError: invalid}); len(got) != 0 {
			t.Fatalf("invalid diagnostic exposed: %+v", got)
		}
	}
}
