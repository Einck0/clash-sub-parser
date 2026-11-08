# Production repair rollout — BLOCKED, rolled back

## Authority and outcome
User explicitly authorized deployment (“CSP任务成功了，帮我部署上线”), followed by urgent outage investigation/recovery appends1712/1713. Run pi_run_20261108_160953_2990699 / change010-export-adaptation / default epoch1. Product deployment attempted, NOT successfully delivered. New productb71159a migrated28 but DB-backed endpoints timed out. Rolled back olda166cb5 and schema27; subsequent DB/export requests still timed out, so FULL BUSINESS RECOVERY IS NOT VERIFIED. No standalone history cleanup or VACUUM executed. Existing225dirty protected. No local project compilation/testing, main unchanged.

## Valid release and preparation evidence (not production success)
Productb71159afc3d11824a4bd3946e40139c9595526a9, not docs028e7b1. Dev Actions42455516482 checks/runtime PASS. Nonforce preleasea166cb5->b71159a; Actions42463830766 all3jobsSUCCESS checks128678672753/runtime128679321894/promotion128679437569. Rolling Release assets source/image/binary verified. Imagee832a180a202823449becf21d54d906cb9cfd0c793c8e9c4cdad748e68403240/OCIb71159a/binarycb0756326348af43a2477afa9d838df0c91deb3b251bcf1521c5c039200a350d. Independent readinessa6fc57fa-ccfd-4c9b-8c23-a5456018c11f PASS concerned prerequisites only; actual production reviewer645242a2-7918-4630-9212-835c3c54e8e1 BLOCKED.

Prior development run2774365 FINISHED/PASS0, activefailures[] with rejected reportc1f728bc exit2 (pending harness image incorrectly reportedPASS). Retained historical rejection; later actual cloud/runtime/reviewer gates valid. This does not excuse production failure or relabel prior isolated results as live proof.

## Backups and actual change
Preserved existing Compose/projectclash-sub-parser/serviceapp, UID10001, csp-v1-data:/data, loopback17000/18080, full env privately at /tmp/csp-rollout-2990699/prepare/private-env-file0600. This file is ACTIVE runtime/rollback dependency, do not delete. Old immutable image77f26749... cached; executor additionally created dormant rollback image protection container (retained, no service started there).
- Online backup /data/backups/csp-v1-pre-repair-2990699.db:2185449472bytes, SHA8b9907a377311cc3e1e7bc136e07df51f3dd1e525774f85113d811ca05679922,integrityok/FK0/schema27.
- Quiescent backup /data/backups/csp-v1-cutover-repair-2990699.db:2187079680bytes,SHAfee6b7ad3c136edd64b014506043ae30f1375539cf0b2635c311b39f300552b2,0600/10001:10001,integrityok/FK0/schema27,orphan0/override-backfill0.
- New containercb8f76b8cd5c... migrated28. Live bounded verification reported10core tables unchanged but DB-backed requests timed out; liveness200 was insufficient.

## Precise incident timeline (UTC+8)
-17:07:42 stop initiated;17:07:43.078 old stopped.
-17:07:43->17:07:58.100 backup copy+SHA15.099s. Script timer ended before later checks.
-Additional integrity/FK/baseline checks aggregate211.9s, uninstrumented individually. No invented per-check duration.
-17:11:30.575 new container started;17:12:49 user observed unhealthy/not listening.
-17:16:17.577 startup Listen log, first healthy bounded17:16:18–17:16:42. Initial non-listening interval514.5s (~8m34.5s). Log is listen-intent/start evidence, not successful business request proof.
-17:23:22 master health20024.5ms; publichealth20085ms, ready timeout8s. Later clash/surge DB requests also timed out.
-17:39:23 master stopped new after confirming no concurrent cutover/backup process, migration completed, backup valid, capacity. Failed DB main/WAL/SHM moved TOGETHER into /var/lib/docker/volumes/csp-v1-data/_data/failed-repair-2990699-20261108-173924. No WAL deleted. Existing validated quiescent backup copied back600/10001:10001 (~10s), no redundant full integrity scan.
-17:39:35 old image recreated via same Compose/env.17:40:02healthy/health2001.6ms. This12s stop->container-start is NOT complete recovery duration.
-17:54–17:56 master bounded export verification:loopback/sub/qx timed out60.059s; public/sub/surge returned HTTP error after60.04s. DB-backed business not recovered.
-17:57:04–06 temporary disable-probe-coordinator override applied; health200,subscriptions/QX timed out8s.
-17:58:11 additional disable-subscription-scheduler applied; health200,subscriptions timed out8s. No further restarts pursued. Current containera22c125ad9616ccc444941e5a3f68a9c006daddc07495cff936ce04344737b6d, oldimage77f26749/schema27, command serve --disable-probe-coordinator --disable-subscription-scheduler. Temporary override /tmp/csp-rollout-2990699/recovery/disable-coordinator.yml IS ACTIVE runtime dependency. Automatic probing/refresh paused; manual refresh/probe not executed. Health/auth status200 does NOT prove DB/export recovery.

## Cause, uncertainty and responsibility
Confirmed: single application DB connection MaxOpenConns1; long SQLite read I/O, independent read-only metadata connection fast; in-memory health200 vs DB requests timeouts. Expensive notice/current inventory predicates over millions of entries and periodic sweep plausible/high-inference contributors. No SQL goroutine stack was captured, so exact running SQL and causal share not proven. Pausing both automatic workers did NOT restore access, weakening any claim the coordinator alone caused failure. Do not present speculative dedicated-pool/materialization fix as established solution. Exact migration-vs-other-startup allocation of287s is uninstrumented, not proven index build.

MASTER RESPONSIBILITY: required exhaustive checks during service downtime, lacked per-stage budget/timers and prompt business monitoring, accepted unsupported~0.5s migration estimate and small synthetic evidence insufficient for real scale. Executor additionally ran unbounded tracing (~510s) delaying visibility; orchestration owns supervision. Historical preflight PASS and container healthy never absolve failed actual gate. No further repeated restarts, production full scans, schema/data patching or destructive cleanup authorized as shortcut.

## Gates and remaining work
- [x] P01 Product/prelease/cloud/release identity, historical failure lineage and protected baseline.
- [x] P02 Env/capacity/online backup, independent readiness and quiescent backup.
- [x] P03 Attempt actual deployment and capture production failure evidence.
- [x] P04 Preserve failed DB/WAL, roll back image/data; apply explicit temporary scheduler pause, record limitations.
- [ ] P05 Restore verified DB-backed business availability, diagnose exact expensive query at real scale, fix via cloud-first development, independent full production review and successful release. BLOCKED; not silently accepted risk.

Need separate bounded engineering continuation for real-scale SQL/connection lifetime diagnosis and reproducible validation without touching production data in tests. Do not perform production historical deletions/VACUUM to disguise query defects. Resume reviewer must cite all active failure IDs from live aggregate. Prelease rolling release currently points failed-to-deploy productb71159a, while actual production olda166cb5; no claim release publication equals deployment.

## Evidence and preservation
Private /tmp/csp-rollout-2990699/{prepare,release,deploy,incident,verify,rollback-verify,recovery,review}; secrets/raw exports0600 outside Git. Failed DB/WAL, online+quiescent backups and old image preserved. No restore rehearsal was claimed before actual rollback; actual file restore recorded but not full business recovery. Raw failures/timeouts remain. No blind production integrity/FK rescans after urgent appends. Final report must BLOCKED, not successful deployment.
