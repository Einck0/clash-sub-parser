package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"clash-sub-parser/migrations"
)

func TestCompactionMembershipSupportedSchemasAndIndexedPlan(t *testing.T) {
	for version := 27; version <= 30; version++ {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			ctx := context.Background()
			db := compactionOrderDB(t)
			runner := NewMigrationRunner(db, migrations.FS)
			// Establish a legacy database before preparation: a fresh schema30
			// runner intentionally seals empty storage and is not this fixture.
			if err := runner.RunUpTo(ctx, 27); err != nil {
				t.Fatal(err)
			}
			if err := runner.RunUpTo(ctx, version); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO subscriptions(id,name,source_url_secret_ref,enabled,prefix,refresh_interval_seconds,refresh_timeout_seconds,refresh_max_response_bytes,config_json,revision,is_primary,userinfo,created_at,updated_at)
 VALUES('s','Synthetic','secret://synthetic',1,'',86400,30,10485760,'{}','revision',0,'','now','now');
 INSERT INTO nodes(logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at,connection_revision) VALUES('n','ss','N','192.0.2.1',8388,'{}',1,'now','now',1);
 INSERT INTO subscription_fetches(id,subscription_id,started_at,outcome,nodes_parsed,nodes_valid) VALUES
 ('old','s','2026-11-10T00:00:00Z','success',1,1),('partial','s','2026-11-10T01:00:00Z','partial',1,1),('failed','s','2026-11-10T02:00:00Z','failed',0,0);
 INSERT INTO subscription_payloads(id,subscription_id,fetch_id,content_digest,body_blob,created_at) VALUES('po','s','old','old',X'','now'),('pp','s','partial','partial',X'','now');
 INSERT INTO subscription_entries(id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at) VALUES('eo','po','s',0,'k','N','ss','proxy','n','now'),('ep','pp','s',0,'k','N','ss','proxy','n','now');
 INSERT INTO node_sources(node_logical_id,subscription_id,last_seen_fetch_id) VALUES('n','s','partial')`); err != nil {
				t.Fatal(err)
			}
			plan, err := InspectSubscriptionCompaction(ctx, db)
			if err != nil || !plan.ApplySupported || len(plan.Subscriptions) != 1 || plan.Subscriptions[0].SelectedPayloadID != "pp" || plan.Subscriptions[0].LastAttemptID != "failed" {
				t.Fatalf("latest partial must beat old success, not be replaced by failure: plan=%+v err=%v", plan, err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if err := checkCompactionMembershipIndex(ctx, tx); err != nil {
				t.Fatal(err)
			}
			rows, err := tx.Query(`EXPLAIN QUERY PLAN SELECT COUNT(*) FROM node_sources ns WHERE NOT EXISTS (` + compactionSourceMembership + `)`)
			if err != nil {
				t.Fatal(err)
			}
			indexed := false
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(detail, "sqlite_autoindex_subscription_entries_2") && strings.Contains(detail, "payload_id=?") {
					indexed = true
				}
			}
			rowErr := rows.Err()
			rows.Close()
			tx.Rollback()
			if rowErr != nil || !indexed {
				t.Fatalf("membership must search payload range, indexed=%v err=%v", indexed, rowErr)
			}
			// All mutations are isolated and rolled back. Check the real inspector
			// keeps rejecting evidence it cannot explain, including corrupt cross-
			// subscription facts which must never be accepted as membership.
			for _, mutation := range []string{
				`UPDATE node_sources SET last_seen_fetch_id='missing'`,
				`UPDATE node_sources SET last_seen_fetch_id='failed'`,
				`UPDATE subscription_entries SET node_logical_id=NULL WHERE payload_id='pp'`,
				`UPDATE subscription_entries SET subscription_id='other' WHERE payload_id='pp'`,
				`UPDATE subscription_payloads SET subscription_id='other' WHERE id='pp'`,
				`DELETE FROM subscription_payloads WHERE id='pp'`,
				`INSERT INTO subscription_payloads(id,subscription_id,fetch_id,content_digest,body_blob,created_at) VALUES('duplicate','s','partial','dup',X'','now')`,
				`UPDATE subscription_fetches SET started_at='invalid' WHERE id='partial'`,
				`UPDATE subscriptions SET revision=''`,
			} {
				tx, err := db.Begin()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(mutation); err != nil {
					t.Fatal(err)
				}
				blocked, err := inspectSubscriptionCompactionTx(ctx, tx)
				tx.Rollback()
				if err != nil || blocked.ApplySupported {
					t.Fatalf("unexplained evidence admitted for %s: plan=%+v err=%v", mutation, blocked, err)
				}
			}
		})
	}
}

func TestCompactionMissingOrWrongMembershipIndexFailsClosed(t *testing.T) {
	for _, ddl := range []string{
		`CREATE TABLE subscription_entries(id TEXT PRIMARY KEY,payload_id TEXT,ordinal INTEGER)`,
		`CREATE TABLE subscription_entries(id TEXT PRIMARY KEY,payload_id TEXT,ordinal INTEGER,UNIQUE(ordinal,payload_id))`,
		`CREATE TABLE subscription_entries(id TEXT PRIMARY KEY,payload_id TEXT,ordinal INTEGER,UNIQUE(payload_id,ordinal DESC))`,
	} {
		db := compactionOrderDB(t)
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		err = checkCompactionMembershipIndex(context.Background(), tx)
		tx.Rollback()
		if err == nil {
			t.Fatalf("unsupported index accepted: %s", ddl)
		}
	}
}
