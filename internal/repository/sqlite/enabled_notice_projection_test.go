package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

// Frozen from 762d706, deliberately independent of the production payload and
// enabled-notice constants. This is a compatibility oracle, not a second runtime
// implementation. Keep it when changing either production query.
const reference762d706UsefulPayloads = `SELECT p.id
FROM subscription_payloads p
JOIN (
    SELECT p2.id,
           ROW_NUMBER() OVER (
               PARTITION BY p2.subscription_id
               ORDER BY p2.created_at DESC, p2.rowid DESC
           ) AS rn
    FROM subscription_payloads p2
    LEFT JOIN subscription_fetches sf ON p2.fetch_id = sf.id
    WHERE (sf.outcome IN ('success', 'partial') OR (sf.id IS NULL AND p2.http_status = 200))
      AND (sf.nodes_valid IS NULL OR sf.nodes_valid > 0 OR EXISTS (SELECT 1 FROM subscription_entries se_check WHERE se_check.payload_id = p2.id))
) ranked ON p.id = ranked.id AND ranked.rn = 1`

const reference762d706EnabledNotices = `SELECT se1.node_logical_id FROM subscription_entries se1
JOIN subscriptions notice_sub ON notice_sub.id = se1.subscription_id AND notice_sub.enabled = 1
WHERE se1.node_logical_id IS NOT NULL
  AND se1.payload_id IN (` + reference762d706UsefulPayloads + `)
  AND COALESCE(
      (SELECT seo.user_kind_override
       FROM subscription_entry_overrides seo
       WHERE seo.subscription_id = se1.subscription_id
         AND ((se1.override_anchor != '' AND seo.override_anchor = se1.override_anchor) OR (se1.source_key != '' AND seo.override_anchor = se1.source_key))
       ORDER BY seo.override_at DESC, seo.rowid DESC
       LIMIT 1),
      se1.entry_kind
  ) = 'notice'
  AND NOT EXISTS (
      SELECT 1 FROM subscription_entries se2
      JOIN subscriptions proxy_sub ON proxy_sub.id = se2.subscription_id AND proxy_sub.enabled = 1
      WHERE se2.node_logical_id = se1.node_logical_id
        AND se2.payload_id IN (` + reference762d706UsefulPayloads + `)
        AND (
            SELECT seo2.user_kind_override
            FROM subscription_entry_overrides seo2
            WHERE seo2.subscription_id = se2.subscription_id
              AND ((se2.override_anchor != '' AND seo2.override_anchor = se2.override_anchor) OR (se2.source_key != '' AND seo2.override_anchor = se2.source_key))
            ORDER BY seo2.override_at DESC, seo2.rowid DESC
            LIMIT 1
        ) = 'proxy'
  )`

type enabledDiagnosticQuery struct {
	name, query string
	args        []any
}

// Mirror nodeRepository.List's COUNT and full default page SELECT, using the
// real predicate builder for both alias forms. The projection/joins/order/limit
// matter to the planner; explaining only a hand-written ID predicate hid the
// enabled-query regression. repo.List below separately exercises the consumer.
func enabledDiagnosticQueries(filter domain.NodeFilter) []enabledDiagnosticQuery {
	countWhere, countArgs := buildNodeFilterPredicates(filter, "")
	where, args := buildNodeFilterPredicates(filter, "n.")
	return []enabledDiagnosticQuery{
		{"count", `SELECT COUNT(*) FROM nodes WHERE ` + strings.Join(countWhere, " AND "), countArgs},
		{"select", `SELECT n.logical_id, n.protocol, n.display_name,
		       COALESCE(json_extract(v.effective_config_json, '$.server'), n.server),
		       COALESCE(json_extract(v.effective_config_json, '$.port'), n.port),
		       COALESCE(json_extract(v.effective_config_json, '$.credentials'), v.effective_config_json, n.config_json),
		       n.active, n.created_at, n.updated_at,
		       COALESCE(h.connection_revision, n.connection_revision)
		FROM nodes n
		LEFT JOIN node_connection_heads h ON n.logical_id = h.logical_id
		LEFT JOIN node_connection_versions v ON h.logical_id = v.node_logical_id AND h.connection_revision = v.connection_revision
		WHERE ` + strings.Join(where, " AND ") + ` ORDER BY n.created_at DESC LIMIT ? OFFSET ?`, append(args, 100, 0)},
	}
}

func logEnabledQueryPlan(t *testing.T, ctx context.Context, db *sql.DB, label string, q enabledDiagnosticQuery) {
	t.Helper()
	rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+q.query, q.args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		t.Logf("plan variant=%s query=%s id=%d parent=%d %s", label, q.name, id, parent, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// Consume and close each result before another query: the fixture intentionally
// uses the production single-connection pool. Compare all projected values, not
// just cardinality. Measurements include row consumption, never fixture setup.
func readEnabledDiagnostic(t *testing.T, ctx context.Context, db *sql.DB, label string, q enabledDiagnosticQuery) [][]any {
	t.Helper()
	before, started := db.Stats(), time.Now()
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rows, err := db.QueryContext(readCtx, q.query, q.args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result [][]any
	for rows.Next() {
		values, ptrs := make([]any, len(cols)), make([]any, len(cols))
		for i := range ptrs {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	after := db.Stats()
	t.Logf("paired variant=%s query=%s elapsed=%s rows=%d pool_waits=%d pool_wait_duration=%s", label, q.name, time.Since(started), len(result), after.WaitCount-before.WaitCount, after.WaitDuration-before.WaitDuration)
	return result
}

func compareEnabledDiagnosticQueries(t *testing.T, ctx context.Context, db *sql.DB, filter domain.NodeFilter) {
	t.Helper()
	for _, current := range enabledDiagnosticQueries(filter) {
		if strings.Count(current.query, enabledNoticeExclusionSubquery) != 1 {
			t.Fatal("diagnostic must use the actual enabled notice predicate exactly once")
		}
		reference := current
		reference.query = strings.Replace(current.query, enabledNoticeExclusionSubquery, reference762d706EnabledNotices, 1)
		logEnabledQueryPlan(t, ctx, db, "legacy_enabled", reference)
		logEnabledQueryPlan(t, ctx, db, "materialized_enabled", current)
		// Reverse order in the second pair to expose cold/warm ordering bias.
		for pair := 0; pair < 2; pair++ {
			var oldRows, newRows [][]any
			if pair == 0 {
				oldRows = readEnabledDiagnostic(t, ctx, db, "legacy_enabled", reference)
				newRows = readEnabledDiagnostic(t, ctx, db, "materialized_enabled", current)
			} else {
				newRows = readEnabledDiagnostic(t, ctx, db, "materialized_enabled", current)
				oldRows = readEnabledDiagnostic(t, ctx, db, "legacy_enabled", reference)
			}
			if !reflect.DeepEqual(oldRows, newRows) {
				t.Fatalf("%s pair=%d legacy/materialized results differ: legacy=%v materialized=%v", current.name, pair, oldRows, newRows)
			}
		}
	}
}

func TestEnabledNoticeProjectionEquivalence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := OpenAndMigrate(ctx, DefaultConfig(filepath.Join(t.TempDir(), "projection.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stamp := "2026-01-01T00:00:00Z"
	later := "2026-01-02T00:00:00Z"
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("fixture query: %v", err)
		}
	}
	addSub := func(id string, enabled bool) {
		exec(`INSERT INTO subscriptions (id,name,source_url_secret_ref,enabled,revision,created_at,updated_at) VALUES (?,?,'synthetic',?,'1',?,?)`, id, id, enabled, stamp, stamp)
	}
	addPayload := func(sub, id, outcome, at string, valid int) {
		exec(`INSERT INTO subscription_fetches (id,subscription_id,started_at,outcome,nodes_valid) VALUES (?,?,?,?,?)`, id, sub, at, outcome, valid)
		exec(`INSERT INTO subscription_payloads (id,subscription_id,fetch_id,content_digest,body_blob,created_at) VALUES (?,?,?,?,X'00',?)`, id, sub, id, id, at)
	}
	addEntry := func(sub, payload, id, kind, key, anchor string, node any) {
		exec(`INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,override_anchor,raw_name,protocol,entry_kind,node_logical_id,created_at) SELECT ?,?,?,COALESCE(MAX(ordinal)+1,0),?,?,'synthetic','ss',?,?,? FROM subscription_entries WHERE payload_id=?`, id, payload, sub, key, anchor, kind, node, stamp, payload)
	}
	addOverride := func(sub, anchor, kind, at string) {
		exec(`INSERT INTO subscription_entry_overrides (subscription_id,override_anchor,user_kind_override,override_at) VALUES (?,?,?,?)`, sub, anchor, kind, at)
	}

	// Each logical node may have multiple current entries/subscriptions. A
	// natural proxy never rescues notice, but an enabled explicit proxy does.
	nodeIDs := []string{"notice", "natural-proxy-no-rescue", "explicit-rescue", "disabled-notice", "disabled-rescue", "unknown-clears", "anchor-time", "source-time", "rowid-tie", "old-notice", "failed-keeps", "partial-replaces", "zero-valid-entries", "empty-keeps", "payload-rowid", "duplicate-notice", "no-entries", "empty-anchor", "isolated-override", "legacy-200", "legacy-500", "valid-empty-replaces"}
	for i, id := range nodeIDs {
		node := domain.Node{LogicalID: id, Protocol: domain.ProtocolSS, DisplayName: id, Server: "192.0.2.1", Port: 10000 + i, Active: true, ConnectionRevision: 1, CreatedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC), UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		if err := NewNodeRepository(db).UpsertBatch(ctx, []domain.Node{node}); err != nil {
			t.Fatal(err)
		}
	}
	addSub("on", true)
	addSub("off", false)
	addPayload("on", "on-p", "success", stamp, 1)
	addPayload("off", "off-p", "success", stamp, 1)
	addEntry("on", "on-p", "notice-entry", "notice", "n", "", "notice")
	for _, id := range []string{"natural-proxy-no-rescue", "explicit-rescue", "disabled-rescue", "duplicate-notice"} {
		addEntry("on", "on-p", id+"-notice", "notice", id+"-n", "", id)
	}
	addEntry("on", "on-p", "natural", "proxy", "natural", "", "natural-proxy-no-rescue")
	addEntry("on", "on-p", "rescue", "notice", "rescue", "", "explicit-rescue")
	addOverride("on", "rescue", "proxy", stamp)
	addEntry("on", "on-p", "dup", "notice", "dup", "", "duplicate-notice")
	addEntry("on", "on-p", "disabled-notice-proxy", "proxy", "dn", "", "disabled-notice")
	addEntry("off", "off-p", "disabled-notice-entry", "notice", "dn", "", "disabled-notice")
	addEntry("off", "off-p", "disabled-rescue-entry", "proxy", "dr", "", "disabled-rescue")
	addOverride("off", "dr", "proxy", stamp)
	addEntry("on", "on-p", "unknown", "notice", "unknown", "", "unknown-clears")
	addOverride("on", "unknown", "unknown", stamp)
	addEntry("on", "on-p", "null-node", "notice", "null", "", nil)
	addEntry("on", "on-p", "blank-anchors", "notice", "", "", "empty-anchor")
	addOverride("on", "", "proxy", later) // Empty anchors must not match.
	addEntry("on", "on-p", "isolated", "notice", "isolated", "", "isolated-override")
	addOverride("off", "isolated", "proxy", later) // Subscription identity matters.

	// Resolve anchor OR source-key matches by time, then ledger rowid. There
	// is no unconditional preference for override_anchor over source_key.
	for _, item := range []struct{ id, first, second, at string }{
		{"anchor-time", "proxy", "notice", stamp},
		{"source-time", "notice", "proxy", stamp},
		{"rowid-tie", "proxy", "notice", later},
	} {
		addEntry("on", "on-p", item.id, "proxy", item.id+"-key", item.id+"-anchor", item.id)
		addOverride("on", item.id+"-anchor", item.first, item.at)
		addOverride("on", item.id+"-key", item.second, later)
	}

	for _, item := range []struct {
		id, outcome, oldKind, newKind string
		valid                         int
	}{
		{"old-notice", "success", "notice", "proxy", 1},
		{"failed-keeps", "failed", "notice", "proxy", 1},
		{"partial-replaces", "partial", "notice", "proxy", 1},
		{"zero-valid-entries", "partial", "proxy", "notice", 0},
		{"empty-keeps", "success", "notice", "", 0},
		{"valid-empty-replaces", "success", "notice", "", 1},
		{"payload-rowid", "success", "notice", "proxy", 1},
	} {
		addSub(item.id, true)
		addPayload(item.id, item.id+"-old", "success", stamp, 1)
		addEntry(item.id, item.id+"-old", item.id+"-old-entry", item.oldKind, "key", "", item.id)
		at := later
		if item.id == "payload-rowid" {
			at = stamp // Later insertion wins even with equal created_at.
		}
		addPayload(item.id, item.id+"-new", item.outcome, at, item.valid)
		if item.newKind != "" {
			addEntry(item.id, item.id+"-new", item.id+"-new-entry", item.newKind, "key", "", item.id)
		}
	}

	// Synthetic legacy imports can have payloads without fetch rows. Build
	// these only in this owned temporary DB; a 200 payload is useful, a newer
	// non-200 payload must not displace it. Restore FK enforcement immediately.
	exec(`PRAGMA foreign_keys = OFF`)
	for _, item := range []struct {
		id, node, kind, at string
		status             int
	}{
		{"legacy-good", "legacy-200", "notice", stamp, 200},
		{"legacy-bad-newer", "legacy-200", "proxy", later, 503},
		{"legacy-bad-only", "legacy-500", "notice", stamp, 500},
	} {
		addSub(item.id, true)
		exec(`INSERT INTO subscription_payloads (id,subscription_id,fetch_id,content_digest,body_blob,http_status,created_at) VALUES (?,?,?,'legacy',X'00',?,?)`, item.id, item.id, "missing-"+item.id, item.status, item.at)
		addEntry(item.id, item.id, item.id+"-entry", item.kind, "legacy", "", item.node)
	}
	// The newer failed-HTTP payload must be in the same subscription to test
	// snapshot selection, rather than merely contributing no classification.
	exec(`UPDATE subscription_payloads SET subscription_id='legacy-good' WHERE id='legacy-bad-newer'`)
	exec(`UPDATE subscription_entries SET subscription_id='legacy-good' WHERE payload_id='legacy-bad-newer'`)
	exec(`PRAGMA foreign_keys = ON`)

	// Enabled inventory includes the same logical nodes; the paired production
	// COUNT/SELECT now exercise the alias-free and aliased NOT IN consumers.
	for _, id := range nodeIDs {
		exec(`INSERT INTO node_sources (node_logical_id,subscription_id,last_seen_fetch_id) VALUES (?,'on','on-p')`, id)
	}
	querySet := func(label, predicate string) []string {
		q := enabledDiagnosticQuery{"exclusions", `SELECT DISTINCT node_logical_id FROM (` + predicate + `) ORDER BY node_logical_id`, nil}
		rows := readEnabledDiagnostic(t, ctx, db, label, q)
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, fmt.Sprint(row[0]))
		}
		return ids
	}
	want := []string{"anchor-time", "disabled-rescue", "duplicate-notice", "empty-anchor", "empty-keeps", "failed-keeps", "isolated-override", "legacy-200", "natural-proxy-no-rescue", "notice", "rowid-tie", "zero-valid-entries"}
	if got := querySet("legacy_enabled", reference762d706EnabledNotices); !reflect.DeepEqual(got, want) {
		t.Fatalf("reference exclusions=%v want=%v", got, want)
	}
	if got := querySet("materialized_enabled", enabledNoticeExclusionSubquery); !reflect.DeepEqual(got, want) {
		t.Fatalf("materialized exclusions=%v want=%v", got, want)
	}
	filter := domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true}
	compareEnabledDiagnosticQueries(t, ctx, db, filter)
	items, total, err := NewNodeRepository(db).List(ctx, filter)
	if err != nil || total != len(nodeIDs)-len(want) || len(items) != total {
		t.Fatalf("consumer list: total=%d items=%d err=%v", total, len(items), err)
	}
	// The unscoped legacy classifier intentionally still sees disabled notice
	// evidence and disabled explicit rescue. It must not adopt enabled semantics.
	legacy := querySet("unscoped", confirmedNoticeExclusionSubquery)
	contains := func(ids []string, id string) bool {
		for _, v := range ids {
			if v == id {
				return true
			}
		}
		return false
	}
	if !contains(legacy, "disabled-notice") || contains(legacy, "disabled-rescue") {
		t.Fatalf("unscoped classification changed: %v", legacy)
	}
	var historyCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_entries WHERE payload_id LIKE '%-old'`).Scan(&historyCount); err != nil || historyCount != 7 {
		t.Fatalf("classification must retain history: entries=%d err=%v", historyCount, err)
	}
}
