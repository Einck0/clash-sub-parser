package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func compactionCLIIdentity(t *testing.T, path string) CompactionFileIdentity {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("stat identity unavailable")
	}
	return CompactionFileIdentity{Path: path, Device: st.Dev, Inode: st.Ino}
}
func compactionCLIWriteJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

type compactionCLIFixture struct {
	scratch, backup, active, plan, output, authorization string
	auth                                                 CompactionAuthorization
	original                                             []byte
}

func makeCompactionCLIFixture(t *testing.T) compactionCLIFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, "scratch")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	f := compactionCLIFixture{scratch: filepath.Join(stage, "scratch.db"), backup: filepath.Join(root, "backup.db"), active: filepath.Join(root, "active.db"), plan: filepath.Join(stage, "plan.json"), output: filepath.Join(stage, "output.db"), authorization: filepath.Join(stage, "authorization.json")}
	cfg := sqlite.DefaultConfig(f.scratch)
	cfg.WALMode = false
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewMigrationRunner(db, migrations.FS).RunUpTo(ctx, 27); err != nil {
		db.Close()
		t.Fatal(err)
	}
	// True schema27 input: no manually seeded raw/current-state, no heads or
	// versions. A later failed attempt must not resurrect removed partial members.
	statements := []string{
		`INSERT INTO subscriptions(id,name,source_url_secret_ref,enabled,prefix,refresh_interval_seconds,refresh_timeout_seconds,refresh_max_response_bytes,config_json,revision,is_primary,userinfo,created_at,updated_at)VALUES('sub','Synthetic','secret://synthetic',1,'',86400,30,10485760,'{}','revision-cut',0,'','2026-11-10T00:00:00Z','2026-11-10T00:00:00Z')`,
		`INSERT INTO nodes(logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at,connection_revision)VALUES('a','ss','A','192.0.2.1',8388,'{}',1,'2026-11-10T00:00:00Z','2026-11-10T00:00:00Z',1),('b','ss','B','192.0.2.2',8388,'{}',0,'2026-11-10T00:00:00Z','2026-11-10T00:00:00Z',1)`,
		`INSERT INTO subscription_fetches(id,subscription_id,started_at,outcome,nodes_parsed,nodes_valid)VALUES('old-fetch','sub','2026-11-10T00:00:00Z','success',2,2),('partial-fetch','sub','2026-11-10T01:00:00Z','partial',2,1),('failed-fetch','sub','2026-11-10T02:00:00Z','failed',0,0)`,
		`INSERT INTO subscription_payloads(id,subscription_id,fetch_id,content_digest,body_blob,pinned,created_at)VALUES('old','sub','old-fetch','old-digest',zeroblob(2097152),1,'2026-11-10T00:00:00Z'),('current','sub','partial-fetch','current-digest',X'','0','2026-11-10T01:00:00Z')`,
		`INSERT INTO subscription_entries(id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at)VALUES('old-a','old','sub',0,'a','A','ss','proxy','a','2026-11-10T00:00:00Z'),('old-b','old','sub',1,'b','B','ss','proxy','b','2026-11-10T00:00:00Z'),('current-a','current','sub',0,'a','A','ss','proxy','a','2026-11-10T01:00:00Z')`,
		`INSERT INTO node_sources VALUES('a','sub','partial-fetch'),('b','sub','old-fetch')`,
		`INSERT INTO publications(id,target,snapshot_digest,compiler_version,token_hash,state,created_at,content)VALUES('pub','mihomo','snapshot','1','synthetic-token','active','2026-11-10T00:00:00Z',X'00112233')`,
		`INSERT INTO publication_payload_refs VALUES('pub','old')`,
		`INSERT INTO node_overrides VALUES('a','display_name','"User A"','2026-11-10T00:00:00Z','2026-11-10T00:00:00Z')`,
		`CREATE TABLE protected_test_asset(id TEXT PRIMARY KEY,value BLOB); INSERT INTO protected_test_asset VALUES('sentinel',X'667788')`,
	}
	for _, q := range statements {
		if _, err := db.ExecContext(ctx, q); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f.scratch, 0600); err != nil {
		t.Fatal(err)
	}
	f.original, err = os.ReadFile(f.scratch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.backup, f.original, 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.active, f.original, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := runSubscriptionCompaction([]string{"plan", "--db", f.scratch, "--report", f.plan}, &out, &stderr); code != 0 {
		t.Fatalf("plan exit %d: %s", code, stderr.String())
	}
	raw, err := os.ReadFile(f.plan)
	if err != nil {
		t.Fatal(err)
	}
	f.auth = CompactionAuthorization{Version: 1, Scratch: compactionCLIIdentity(t, f.scratch), Backup: compactionCLIIdentity(t, f.backup), Active: compactionCLIIdentity(t, f.active), Output: f.output, PlanSHA256: compactionBytesHash(raw), BackupSHA256: compactionBytesHash(f.original), Offline: true, ConsumersDrained: true}
	compactionCLIWriteJSON(t, f.authorization, f.auth)
	return f
}
func (f compactionCLIFixture) args() []string {
	return []string{"apply", "--db", f.scratch, "--backup", f.backup, "--plan", f.plan, "--output", f.output, "--authorization", f.authorization, "--offline", "--consumers-drained"}
}

func TestSubscriptionCompactionCLISchema27ThroughPhysicalOutput(t *testing.T) {
	// The real guard must inspect every visible process, including root-owned
	// runner services. An unprivileged Actions worker cannot do that. Re-exec
	// only this already cloud-built test as root, with fresh root-owned fixtures;
	// never stub the guard, skip this spine, or relax production permissions.
	probe := filepath.Join(t.TempDir(), "process-inspection-probe")
	if err := os.WriteFile(probe, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := checkOpenDatabaseFiles(probe); err != nil {
		if !errors.Is(err, os.ErrPermission) || os.Geteuid() == 0 || os.Getenv("GITHUB_ACTIONS") != "true" {
			t.Fatalf("real process inspection prerequisite unavailable: %v", err)
		}
		// Assert that real apply also fails closed under this runner's
		// permissions before using a capable process for the success branch.
		f := makeCompactionCLIFixture(t)
		var out, stderr bytes.Buffer
		if code := runSubscriptionCompaction(f.args(), &out, &stderr); code == 0 || out.Len() != 0 || !strings.Contains(stderr.String(), "process inspection permission denied") {
			t.Fatalf("unprivileged apply must reject inspection failure: exit %d stdout=%s stderr=%s", code, out.String(), stderr.String())
		}
		for _, path := range []string{f.scratch, f.backup} {
			raw, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(raw, f.original) {
				t.Fatalf("permission refusal changed cut: %v", readErr)
			}
		}
		if _, statErr := os.Lstat(f.output); !os.IsNotExist(statErr) {
			t.Fatal("permission refusal produced output")
		}
		t.Logf("unprivileged cloud worker cannot inspect procfs: %v; re-executing this test with sudo", err)
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sudo", "-n", "--", executable, "-test.run=^TestSubscriptionCompactionCLISchema27ThroughPhysicalOutput$", "-test.v")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("privileged cloud spine failed: %v\n%s", err, output)
		}
		t.Logf("privileged cloud spine:\n%s", output)
		return
	}
	f := makeCompactionCLIFixture(t)
	assertCompactionRejectsRealOpenFile(t, f)
	var out, stderr bytes.Buffer
	if code := runSubscriptionCompaction(f.args(), &out, &stderr); code != 0 {
		t.Fatalf("apply exit %d: %s", code, stderr.String())
	}
	var receipt compactionApplyReceipt
	if err := json.Unmarshal(out.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "VERIFIED_OFFLINE_OUTPUT" || receipt.Verification == nil || !receipt.Verification.Verified || receipt.Verification.ConsumerClosureSealed || receipt.OutputBytes >= receipt.BeforeBytes {
		t.Fatalf("invalid receipt: %+v", receipt)
	}
	for _, path := range []string{f.backup, f.active} {
		raw, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(raw, f.original) {
			t.Fatalf("backup or active input changed: %v", err)
		}
	}
	out.Reset()
	stderr.Reset()
	if code := runSubscriptionCompaction([]string{"verify", "--db", f.output, "--baseline", f.plan}, &out, &stderr); code != 0 {
		t.Fatalf("verify exit %d: %s", code, stderr.String())
	}
	cfg := sqlite.DefaultConfig(f.output)
	cfg.WALMode = false
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for q, want := range map[string]int{
		`SELECT COUNT(*) FROM subscription_payloads`:                                                     1,
		`SELECT COUNT(*) FROM subscription_entries`:                                                      1,
		`SELECT COUNT(*) FROM subscription_fetches`:                                                      2,
		`SELECT COUNT(*) FROM subscription_original_configs`:                                             0,
		`SELECT COUNT(*) FROM nodes`:                                                                     2,
		`SELECT COUNT(*) FROM node_sources WHERE node_logical_id='b'`:                                    0,
		`SELECT COUNT(*) FROM node_sources WHERE node_logical_id='a'`:                                    1,
		`SELECT COUNT(*) FROM node_source_history WHERE relation_state='unknown' AND cause='unresolved'`: 2,
		`SELECT COUNT(*) FROM node_connection_heads`:                                                     2,
		`SELECT COUNT(*) FROM node_connection_versions`:                                                  2,
		`SELECT COUNT(*) FROM subscription_current_state WHERE applied_fetch_id='partial-fetch' AND last_attempt_fetch_id='failed-fetch' AND last_attempt_status='failed'`:     1,
		`SELECT COUNT(*) FROM publication_payload_refs r JOIN subscription_payload_evidence e ON e.payload_id=r.payload_id WHERE e.content_digest='old-digest' AND e.pinned=1`: 1,
		`SELECT COUNT(*) FROM publications WHERE content=X'00112233' AND token_hash='synthetic-token'`:                                                                         1,
		`SELECT COUNT(*) FROM protected_test_asset WHERE value=X'667788'`:                                                                                                      1,
	} {
		var got int
		if err := db.QueryRow(q).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: got %d want %d: %v", q, got, want, err)
		}
	}
}

func TestSubscriptionCompactionCLIRejectsUnsafeInputsBeforeMutation(t *testing.T) {
	for _, branch := range []string{"authorization", "missing_offline", "missing_drain", "backup_writable", "backup_changed", "changed_snapshot", "existing_output", "scratch_alias", "hardlink", "symlink", "wrong_inode", "sidecar", "unknown_membership", "plan_tampered"} {
		t.Run(branch, func(t *testing.T) {
			f := makeCompactionCLIFixture(t)
			args := f.args()
			switch branch {
			case "authorization":
				f.auth.Offline = false
			case "missing_offline":
				args = args[:len(args)-2]
			case "missing_drain":
				args = args[:len(args)-1]
			case "backup_writable":
				if err := os.Chmod(f.backup, 0600); err != nil {
					t.Fatal(err)
				}
			case "backup_changed":
				if err := os.Chmod(f.backup, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(f.backup, []byte("invalid"), 0400); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(f.backup, 0400); err != nil {
					t.Fatal(err)
				}
			case "changed_snapshot":
				db, err := sqlite.Open(sqlite.Config{Path: f.scratch, ForeignKeys: true})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE nodes SET display_name='changed'`); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			case "existing_output":
				if err := os.WriteFile(f.output, []byte("do-not-overwrite"), 0600); err != nil {
					t.Fatal(err)
				}
			case "scratch_alias":
				f.auth.Scratch = f.auth.Active
			case "hardlink":
				if err := os.Link(f.scratch, f.scratch+".alias"); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				alias := filepath.Join(filepath.Dir(f.scratch), "alias.db")
				if err := os.Symlink(f.scratch, alias); err != nil {
					t.Fatal(err)
				}
				args[2] = alias
			case "wrong_inode":
				f.auth.Scratch.Inode++
			case "sidecar":
				if err := os.WriteFile(f.scratch+"-wal", nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown_membership":
				db, err := sqlite.Open(sqlite.Config{Path: f.scratch, ForeignKeys: true})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE node_sources SET last_seen_fetch_id='unknown'`); err != nil {
					t.Fatal(err)
				}
				db.Close()
			case "plan_tampered":
				if err := os.WriteFile(f.plan, []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			compactionCLIWriteJSON(t, f.authorization, f.auth)
			before, err := os.ReadFile(f.scratch)
			if err != nil {
				t.Fatal(err)
			}
			backupBefore, err := os.ReadFile(f.backup)
			if err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			if code := runSubscriptionCompaction(args, &out, &stderr); code == 0 || out.Len() != 0 {
				t.Fatalf("unsafe apply exit %d: %s", code, out.String())
			}
			after, err := os.ReadFile(f.scratch)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected apply changed scratch: %v", err)
			}
			after, err = os.ReadFile(f.backup)
			if err != nil || !bytes.Equal(backupBefore, after) {
				t.Fatalf("rejected apply changed backup: %v", err)
			}
			if branch == "existing_output" {
				raw, err := os.ReadFile(f.output)
				if err != nil || string(raw) != "do-not-overwrite" {
					t.Fatal("existing output changed")
				}
			} else {
				if _, err := os.Stat(f.output); !os.IsNotExist(err) {
					t.Fatal("unsafe apply produced output")
				}
			}
		})
	}
}
