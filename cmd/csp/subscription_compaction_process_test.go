package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Run inside the permission-capable end-to-end spine, before the successful
// conversion. Prove the same real guard refuses a live external descriptor and
// leaves both scratch and independent backup byte-identical on rejection.
func assertCompactionRejectsRealOpenFile(t *testing.T, f compactionCLIFixture) {
	t.Helper()
	held, err := os.Open(f.scratch)
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sleep", "60")
	child.ExtraFiles = []*os.File{held}
	if err := child.Start(); err != nil {
		held.Close()
		t.Fatalf("cannot start open-file fixture: %v", err)
	}
	if err := held.Close(); err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		t.Fatal(err)
	}
	// No test skip or success stub: inherited fd3 exists from process creation,
	// so no polling/delay is needed to establish that the cut is really open.
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	pid, err := checkOpenDatabaseFiles(f.scratch)
	if err == nil || pid != child.Process.Pid {
		t.Fatalf("real open-file guard: got pid %d, err %v; want child %d", pid, err, child.Process.Pid)
	}
	var out, stderr bytes.Buffer
	if code := runSubscriptionCompaction(f.args(), &out, &stderr); code == 0 || out.Len() != 0 || !strings.Contains(stderr.String(), "visible process has cut open") {
		t.Fatalf("open scratch admitted or wrong rejection: exit %d stdout=%s stderr=%s", code, out.String(), stderr.String())
	}
	for _, path := range []string{f.scratch, f.backup} {
		raw, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(raw, f.original) {
			t.Fatalf("open-file rejection changed cut: %v", err)
		}
	}
	if _, err := os.Lstat(f.output); !os.IsNotExist(err) {
		t.Fatal("open-file rejection produced output")
	}
}
