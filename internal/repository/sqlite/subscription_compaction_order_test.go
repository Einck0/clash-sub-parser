package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func compactionOrderDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// Deliberately retain the original full-column order and typed complete-row
// encoding as an oracle, independent of the optimized ordering decision.
func originalCompactionDigest(t *testing.T, tx *sql.Tx, source string) (int64, string) {
	t.Helper()
	ctx := context.Background()
	rows, err := tx.QueryContext(ctx, "SELECT * FROM "+source+" LIMIT 0")
	if err != nil {
		t.Fatal(err)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	order := make([]string, len(columns))
	for i, col := range columns {
		order[i] = quoteCompactionIdentifier(col) + " COLLATE BINARY"
	}
	rows, err = tx.QueryContext(ctx, "SELECT * FROM "+source+" ORDER BY "+strings.Join(order, ","))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	h := sha256.New()
	if err := writeCompactionDigest(h, columns); err != nil {
		t.Fatal(err)
	}
	var count int64
	for rows.Next() {
		values, targets := make([]any, len(columns)), make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			t.Fatal(err)
		}
		typed := make([]any, len(values))
		for i, value := range values {
			typed[i] = []any{fmt.Sprintf("%T", value), value}
		}
		if err := writeCompactionDigest(h, typed); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return count, hex.EncodeToString(h.Sum(nil))
}

func TestCompactionUniquePrefixPreservesOriginalDigest(t *testing.T) {
	for _, tc := range []struct {
		name, ddl, data string
		prefix          int
	}{
		{"binary primary", `CREATE TABLE fixture(id TEXT PRIMARY KEY, value)`, `INSERT INTO fixture VALUES('b',x'00ff'),('A','text'),('a',17)`, 1},
		{"nullable unique without nulls", `CREATE TABLE fixture(id TEXT UNIQUE, value)`, `INSERT INTO fixture VALUES('b',1.25),('a',NULL)`, 1},
		{"nullable primary duplicate null", `CREATE TABLE fixture(id TEXT PRIMARY KEY, value)`, `INSERT INTO fixture VALUES(NULL,'z'),(NULL,'a'),('a',x'01')`, 0},
		{"duplicate null unique", `CREATE TABLE fixture(id TEXT UNIQUE, value)`, `INSERT INTO fixture VALUES(NULL,'z'),(NULL,'a')`, 0},
		{"nocase", `CREATE TABLE fixture(id TEXT COLLATE NOCASE UNIQUE, value)`, `INSERT INTO fixture VALUES('b',1),('A',2),('C',3)`, 0},
		{"descending", `CREATE TABLE fixture(id TEXT, value); CREATE UNIQUE INDEX fixture_unique ON fixture(id DESC)`, `INSERT INTO fixture VALUES('b',1),('A',2)`, 0},
		{"expression", `CREATE TABLE fixture(id TEXT, value); CREATE UNIQUE INDEX fixture_unique ON fixture(lower(id))`, `INSERT INTO fixture VALUES('b',1),('A',2)`, 0},
		{"partial", `CREATE TABLE fixture(id TEXT, value); CREATE UNIQUE INDEX fixture_unique ON fixture(id) WHERE value>0`, `INSERT INTO fixture VALUES('a',-1),('a',2),('b',3)`, 0},
		{"no index", `CREATE TABLE fixture(id TEXT, value)`, `INSERT INTO fixture VALUES('a','z'),('a','a'),('b',NULL)`, 0},
		{"composite prefix", `CREATE TABLE fixture(id TEXT, part INTEGER, value, UNIQUE(id,part))`, `INSERT INTO fixture VALUES('a',2,x'00'),('a',1,'x'),('b',0,NULL)`, 2},
		{"composite null", `CREATE TABLE fixture(id TEXT, part INTEGER, value, UNIQUE(id,part))`, `INSERT INTO fixture VALUES('a',NULL,'z'),('a',NULL,'a')`, 0},
		{"nonprefix index", `CREATE TABLE fixture(id TEXT, value TEXT UNIQUE)`, `INSERT INTO fixture VALUES('b','a'),('a','z')`, 0},
		{"empty", `CREATE TABLE fixture(id TEXT PRIMARY KEY, value)`, `SELECT 1`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := compactionOrderDB(t)
			if _, err := db.Exec(tc.ddl + ";" + tc.data); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			rows, err := tx.Query(`SELECT * FROM fixture LIMIT 0`)
			if err != nil {
				t.Fatal(err)
			}
			columns, err := rows.Columns()
			rows.Close()
			if err != nil {
				t.Fatal(err)
			}
			prefix, err := compactionUniqueOrderPrefix(context.Background(), tx, "fixture", columns)
			if err != nil || prefix != tc.prefix {
				t.Fatalf("prefix=%d want=%d err=%v", prefix, tc.prefix, err)
			}
			for _, source := range []struct{ name, sql string }{
				{"fixture", `"fixture"`},
				{"fixture", `(SELECT value,id FROM fixture)`},
				{"fixture", `(SELECT * FROM fixture UNION ALL SELECT * FROM fixture)`},
				{"sqlite_schema", `"sqlite_schema"`},
			} {
				wantCount, want := originalCompactionDigest(t, tx, source.sql)
				count, got, err := digestCompactionSource(context.Background(), tx, source.name, source.sql)
				if err != nil || count != wantCount || got != want {
					t.Fatalf("source=%s count=%d/%d digest=%s/%s err=%v", source.sql, count, wantCount, got, want, err)
				}
			}
		})
	}
}

func TestCompactionPrefixHashesCompleteRowsAndTypes(t *testing.T) {
	db := compactionOrderDB(t)
	if _, err := db.Exec(`CREATE TABLE fixture(id TEXT PRIMARY KEY,value); INSERT INTO fixture VALUES('a','eA==')`); err != nil {
		t.Fatal(err)
	}
	var digests []string
	for _, value := range []string{`'eA=='`, `x'78'`, `17`, `17.0`, `NULL`} {
		if _, err := db.Exec(`UPDATE fixture SET value=` + value); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		_, got, err := digestCompactionTable(context.Background(), tx, "fixture")
		tx.Rollback()
		if err != nil {
			t.Fatal(err)
		}
		for _, prior := range digests {
			if prior == got {
				t.Fatalf("complete-row/type change %s was not detected", value)
			}
		}
		digests = append(digests, got)
	}
}

const originalCompactionLatestEntries = `(SELECT * FROM (SELECT e.*,ROW_NUMBER() OVER (
 PARTITION BY e.node_logical_id,e.subscription_id ORDER BY julianday(e.created_at) DESC,e.rowid DESC
 ) AS compaction_rank FROM subscription_entries e WHERE e.node_logical_id IS NOT NULL) WHERE compaction_rank=1)`

func TestCompactionNarrowRankingPreservesWinnersAndProjections(t *testing.T) {
	db := compactionOrderDB(t)
	if _, err := db.Exec(`CREATE TABLE subscriptions(id TEXT PRIMARY KEY,name TEXT);
 CREATE TABLE subscription_entries(id TEXT PRIMARY KEY,node_logical_id TEXT,subscription_id TEXT,created_at TEXT,raw_name TEXT,protocol TEXT,source_key TEXT,config BLOB);
 CREATE TABLE node_source_history(id TEXT,node_logical_id TEXT,subscription_id TEXT,source_identity TEXT,source_label TEXT,connection_revision INTEGER,relation_state TEXT,cause TEXT,first_observed_at TEXT,last_observed_at TEXT,evidence_kind TEXT,evidence_key TEXT,evidence_json TEXT,created_at TEXT);
 INSERT INTO subscriptions VALUES('s','Synthetic'),('other','Other');
 INSERT INTO subscription_entries VALUES
 ('first','n','s','2026-11-10T02:00:00Z','First','ss','k',x'00'),
 ('fraction','n','s','2026-11-10T02:00:00.500Z','Fraction','ss','k',x'01'),
 ('tie','n','s','2026-11-10T03:00:00.500+01:00','Tie','ss','k',x'02'),
 ('invalid','n','s','invalid','Invalid','ss','k',x'03'),
 ('other-sub','n','other','2026-11-10T00:00:00Z','Other','ss','o',x'04'),
 ('null-first','invalid-only','s','invalid','Null first','ss','i',x'05'),
 ('null-tie','invalid-only','s','also-invalid','Null tie','ss','i',x'06'),
 ('excluded',NULL,'s','2027-01-01T00:00:00Z','No node','ss','x',x'07');
 INSERT INTO node_source_history VALUES('existing','n','other','sub:other','Other',1,'present','observed','before','after','existing','key','{}','before')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM ` + compactionLatestEntries + ` ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var winners []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		winners = append(winners, id)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil || !reflect.DeepEqual(winners, []string{"null-tie", "other-sub", "tie"}) {
		t.Fatalf("winners=%v err=%v", winners, rowErr)
	}
	// Original SELECT e.* carried a private rank column; compare only the full
	// business row, and both actual downstream history/observation projections.
	for _, projection := range []string{
		`(SELECT id,node_logical_id,subscription_id,created_at,raw_name,protocol,source_key,config FROM ` + compactionLatestEntries + `)`,
		compactionProjectedHistory,
		`(SELECT e.node_logical_id,'sub:'||e.subscription_id,e.subscription_id,e.raw_name,e.protocol,e.source_key,e.created_at FROM ` + compactionLatestEntries + ` e)`,
	} {
		old := strings.ReplaceAll(projection, compactionLatestEntries, originalCompactionLatestEntries)
		wantCount, want := originalCompactionDigest(t, tx, old)
		count, got, err := digestCompactionSource(context.Background(), tx, "projection", projection)
		if err != nil || count != wantCount || got != want {
			t.Fatalf("projection changed count=%d/%d digest=%s/%s err=%v", count, wantCount, got, want, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM subscription_entries`); err != nil {
		t.Fatal(err)
	}
	if count, _, err := digestCompactionSource(context.Background(), tx, "empty", compactionLatestEntries); err != nil || count != 0 {
		t.Fatalf("empty winner projection count=%d err=%v", count, err)
	}
}
