package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// CompactionAuthorization is a private, explicit operator grant for a single
// cut, not a pathname-based guess. Dev/inode identities must be collected after
// preparing the independent files. The tool cannot prove cross-namespace drain:
// the operator must stop all external writers/consumers before granting it.
// Consumer closure is a separate operator assertion backed by independent
// validation; a flag or manifest cannot itself prove the consumer contract.
// It never stops a service, creates a backup, or switches the active database.
type CompactionAuthorization struct {
	Version          int                    `json:"version"`
	Scratch          CompactionFileIdentity `json:"scratch"`
	Backup           CompactionFileIdentity `json:"backup"`
	Active           CompactionFileIdentity `json:"active"`
	Output           string                 `json:"output"`
	PlanSHA256       string                 `json:"plan_sha256"`
	BackupSHA256     string                 `json:"backup_sha256"`
	Offline          bool                   `json:"offline"`
	ConsumersDrained bool                   `json:"consumers_drained"`
	ConsumersClosed  bool                   `json:"consumers_closed"`
}
type CompactionFileIdentity struct {
	Path   string `json:"path"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

func runSubscriptionCompactionApply(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compact-subscription-storage apply", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var dbPath, backupPath, planPath, outputPath, authPath string
	var offline, drained, closed bool
	var timeout time.Duration
	fs.StringVar(&dbPath, "db", "", "explicit authorized scratch database")
	fs.StringVar(&backupPath, "backup", "", "independent read-only sealed backup")
	fs.StringVar(&planPath, "plan", "", "original read-only plan")
	fs.StringVar(&outputPath, "output", "", "nonexistent output in private scratch directory")
	fs.StringVar(&authPath, "authorization", "", "private cut-specific authorization manifest")
	fs.BoolVar(&offline, "offline", false, "assert all services accessing this cut are stopped")
	fs.BoolVar(&drained, "consumers-drained", false, "assert all external version consumers are drained")
	fs.BoolVar(&closed, "consumers-closed", false, "assert the independently validated version consumer contract applies to this cut")
	fs.DurationVar(&timeout, "timeout", 30*time.Minute, "bounded operation duration (maximum 2h)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if dbPath == "" || backupPath == "" || planPath == "" || outputPath == "" || authPath == "" || !offline || !drained || !closed || fs.NArg() != 0 || timeout <= 0 || timeout > 2*time.Hour {
		fmt.Fprintln(stderr, "compaction apply: maintenance_required: explicit scratch/backup/plan/output/authorization, --offline, --consumers-drained and --consumers-closed required")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	receipt, err := applySubscriptionCompaction(ctx, dbPath, backupPath, planPath, outputPath, authPath)
	if err != nil {
		// Driver errors may contain credentials. Keep full errors out of terminals.
		fmt.Fprintln(stderr, "compaction apply: maintenance_required: "+err.Error())
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		return 1
	}
	return 0
}

type compactionApplyReceipt struct {
	Status           string                                  `json:"status"`
	BeforeBytes      int64                                   `json:"before_bytes"`
	OutputBytes      int64                                   `json:"output_bytes"`
	BackupSHA256     string                                  `json:"backup_sha256"`
	Verification     *domain.SubscriptionStorageVerification `json:"verification"`
	NamespaceWarning string                                  `json:"namespace_warning"`
}

func applySubscriptionCompaction(ctx context.Context, dbPath, backupPath, planPath, outputPath, authPath string) (*compactionApplyReceipt, error) {
	fail := func(reason string) (*compactionApplyReceipt, error) { return nil, errors.New(reason) }
	var auth CompactionAuthorization
	authRaw, err := readCompactionPrivateFile(authPath)
	if err != nil || json.Unmarshal(authRaw, &auth) != nil || auth.Version != 1 || !auth.Offline || !auth.ConsumersDrained || !auth.ConsumersClosed {
		return fail("invalid private operator authorization")
	}
	planRaw, err := readCompactionPrivateFile(planPath)
	if err != nil || compactionBytesHash(planRaw) != auth.PlanSHA256 {
		return fail("plan authorization digest mismatch")
	}
	var baseline domain.SubscriptionStoragePlan
	if json.Unmarshal(planRaw, &baseline) != nil || !baseline.ApplySupported {
		return fail("unsupported or unresolved plan")
	}
	identities := []CompactionFileIdentity{auth.Scratch, auth.Backup, auth.Active}
	paths := []string{dbPath, backupPath, auth.Active.Path}
	infos := make([]os.FileInfo, 0, 3)
	for i, path := range paths {
		canonical, fi, err := compactionCanonicalFile(path)
		if err != nil || canonical != identities[i].Path {
			return fail("file identity or canonical path rejected")
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || st.Dev != identities[i].Device || st.Ino != identities[i].Inode || st.Nlink != 1 {
			return fail("file device/inode or hardlink rejected")
		}
		if i < 2 && (fi.Mode().Perm()&0077 != 0 || st.Uid != uint32(os.Geteuid())) {
			return fail("scratch and backup must be owned private files")
		}
		for _, prev := range infos {
			if os.SameFile(fi, prev) {
				return fail("scratch/backup/active alias rejected")
			}
		}
		infos = append(infos, fi)
	}
	if infos[1].Mode().Perm()&0222 != 0 {
		return fail("backup must be sealed read-only")
	}
	backupDir, err := os.Stat(filepath.Dir(auth.Backup.Path))
	if err != nil || !backupDir.IsDir() || backupDir.Mode().Perm() != 0700 {
		return fail("backup parent must be private 0700 directory")
	}
	scratch := auth.Scratch.Path
	dir := filepath.Dir(scratch)
	canonicalDir, err := filepath.EvalSymlinks(dir)
	dirInfo, dirErr := os.Stat(dir)
	if err != nil || dirErr != nil || canonicalDir != dir || !dirInfo.IsDir() || dirInfo.Mode().Perm() != 0700 {
		return fail("scratch workspace must be canonical private 0700 directory")
	}
	if strings.HasPrefix(scratch, "/data/") || strings.HasPrefix(scratch, "/var/lib/docker/") || dir == filepath.Dir(auth.Active.Path) {
		return fail("active data location cannot be a scratch workspace")
	}
	if !filepath.IsAbs(outputPath) || filepath.Clean(outputPath) != outputPath || filepath.Dir(outputPath) != dir || auth.Output != outputPath {
		return fail("output must be explicitly authorized inside scratch workspace")
	}
	for _, path := range []string{scratch, auth.Backup.Path, outputPath} {
		if path == outputPath {
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				return fail("output must not exist")
			}
		}
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if _, err := os.Lstat(path + suffix); !os.IsNotExist(err) {
				return fail("self-contained cut required; sidecar exists or cannot be inspected")
			}
		}
	}
	if pid, err := checkConcurrentService(scratch); err != nil || pid > 0 {
		return fail("concurrent service detected or inspection failed")
	}
	// O_EXCL lock is deliberately not auto-recovered after a crash. An operator
	// must inspect a retained failed scratch rather than blindly replay it.
	lock, err := os.OpenFile(scratch+".maintenance.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fail("scratch maintenance ownership unavailable")
	}
	defer lock.Close()
	defer os.Remove(scratch + ".maintenance.lock")
	if _, err := fmt.Fprintln(lock, os.Getpid()); err != nil {
		return fail("cannot record maintenance ownership")
	}
	if err := lock.Sync(); err != nil {
		return fail("cannot sync maintenance ownership")
	}
	for _, path := range []string{scratch, auth.Backup.Path} {
		if pid, err := checkOpenDatabaseFiles(path); err != nil || pid > 0 {
			// Preserve fail-closed behavior while separating an unavailable
			// permission prerequisite from an actual open-file observation.
			// Never copy raw procfs errors/paths to terminal output.
			if errors.Is(err, os.ErrPermission) {
				return fail("process inspection permission denied; full visible procfs access required")
			}
			return fail("visible process has cut open or process inspection failed")
		}
	}
	beforeHash, err := compactionFileHash(auth.Backup.Path)
	if err != nil || beforeHash != auth.BackupSHA256 {
		return fail("sealed backup digest mismatch")
	}
	scratchHash, err := compactionFileHash(scratch)
	if err != nil || scratchHash != beforeHash {
		return fail("scratch must be an independent byte-identical backup copy")
	}
	// Rollback journal, migration indexes/temp and new output can coexist. This
	// conservative bound does not promise a compact target size in advance.
	var space syscall.Statfs_t
	if err := syscall.Statfs(dir, &space); err != nil {
		return fail("capacity inspection failed")
	}
	need := uint64(infos[0].Size())*3 + 64*1024*1024
	if space.Bavail*uint64(space.Bsize) < need {
		return fail("insufficient scratch/output migration capacity")
	}
	backup, err := sqlite.OpenReadOnly(auth.Backup.Path)
	if err != nil {
		return fail("cannot inspect backup")
	}
	_, _, integrityErr := verifySQLiteIntegrity(ctx, backup)
	backupPlan, inspectErr := sqlite.InspectSubscriptionCompaction(ctx, backup)
	closeErr := backup.Close()
	if integrityErr != nil || inspectErr != nil || closeErr != nil || backupPlan.SnapshotDigest != baseline.SnapshotDigest {
		return fail("backup integrity or planned snapshot mismatch")
	}
	cfg := sqlite.DefaultConfig(scratch)
	cfg.WALMode = false
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := sqlite.Open(cfg)
	if err != nil {
		return fail("cannot open scratch")
	}
	defer db.Close()
	// Exclusive mode retains the SQLite file lock between the runner's separate
	// transactions on this sole connection. No online startup path is invoked.
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=DELETE; PRAGMA locking_mode=EXCLUSIVE; BEGIN EXCLUSIVE; COMMIT;`); err != nil {
		return fail("exclusive scratch lock unavailable")
	}
	for i, path := range paths {
		_, fi, err := compactionCanonicalFile(path)
		if err != nil || !os.SameFile(fi, infos[i]) {
			return fail("cut identity changed before conversion")
		}
	}
	prepared, decisions, err := sqlite.PrepareOfflineSubscriptionCompaction(ctx, db, &baseline)
	if err != nil {
		return fail("scratch preparation rejected; backup untouched")
	}
	if _, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, prepared, decisions, auth.ConsumersClosed); err != nil {
		return fail("scratch transformation rejected; backup untouched")
	}
	verification, err := sqlite.VerifySubscriptionCompaction(ctx, db, &baseline)
	if err != nil || !verification.Verified {
		return fail("scratch protected-content verification failed")
	}
	// SQLite creates only inside our new private staging directory. Link installs
	// the completed output atomically without overwriting any competing target.
	stage, err := os.MkdirTemp(dir, ".compaction-output-")
	if err != nil {
		return fail("cannot reserve private output stage")
	}
	staged := filepath.Join(stage, "candidate.db")
	defer func() { _ = os.Remove(staged); _ = os.Remove(stage) }()
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, staged); err != nil {
		return fail("physical compaction failed; no output approved")
	}
	if err := os.Chmod(staged, 0600); err != nil {
		return fail("cannot secure compact output")
	}
	candidate, err := sqlite.OpenReadOnly(staged)
	if err != nil {
		return fail("cannot reopen compact output")
	}
	verification, err = sqlite.VerifySubscriptionCompaction(ctx, candidate, &baseline)
	closeErr = candidate.Close()
	if err != nil || closeErr != nil || !verification.Verified {
		return fail("compact output verification failed")
	}
	finalBackupHash, err := compactionFileHash(auth.Backup.Path)
	_, finalBackupInfo, identityErr := compactionCanonicalFile(auth.Backup.Path)
	if err != nil || identityErr != nil || !os.SameFile(finalBackupInfo, infos[1]) || finalBackupHash != beforeHash {
		return fail("backup changed during conversion")
	}
	if err := syncCompactionFile(staged); err != nil {
		return fail("compact output sync failed")
	}
	if err := os.Link(staged, outputPath); err != nil {
		return fail("output installation refused; target may already exist")
	}
	if err := os.Remove(staged); err != nil {
		return fail("output stage unlink failed; inspect retained output")
	}
	parent, err := os.Open(dir)
	if err != nil {
		return fail("output directory open failed")
	}
	err = parent.Sync()
	closeErr = parent.Close()
	if err != nil || closeErr != nil {
		return fail("output directory sync failed; inspect retained output")
	}
	outInfo, err := os.Stat(outputPath)
	if err != nil {
		return fail("output stat failed")
	}
	return &compactionApplyReceipt{Status: "VERIFIED_OFFLINE_OUTPUT", BeforeBytes: infos[0].Size(), OutputBytes: outInfo.Size(), BackupSHA256: beforeHash, Verification: verification, NamespaceWarning: NamespaceScopeNotice}, nil
}

func compactionCanonicalFile(path string) (string, os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", nil, errors.New("plain absolute path required")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return "", nil, errors.New("symlink rejected")
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if !fi.Mode().IsRegular() {
		return "", nil, errors.New("regular file required")
	}
	return canonical, fi, nil
}
func readCompactionPrivateFile(path string) ([]byte, error) {
	_, fi, err := compactionCanonicalFile(path)
	if err != nil {
		return nil, err
	}
	if fi.Mode().Perm()&0077 != 0 || fi.Size() > 8*1024*1024 {
		return nil, errors.New("private bounded manifest required")
	}
	return os.ReadFile(path)
}
func compactionBytesHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func compactionFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func syncCompactionFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
