//go:build tuistory

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestTraceViewTuistoryEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("tuistory"); err != nil {
		t.Skip("tuistory is not installed")
	}

	root := repoRootForTest(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "gollem")
	tracePath := filepath.Join(tmp, "view.trace.json")
	runCmd(t, root, "go", "build", "-o", bin, "./cmd/gollem")
	runCmd(t, root, bin, "run", "--provider", "test", "--no-code-mode", "--trace-out", tracePath, "tuistory trace view")

	session := fmt.Sprintf("gollem-trace-%d", time.Now().UnixNano())
	defer func() { _ = exec.Command("tuistory", "-s", session, "close").Run() }()

	runTuistory(t, "launch", strconv.Quote(bin)+" trace view "+strconv.Quote(tracePath), "-s", session, "--cols", "100", "--rows", "28")
	runTuistory(t, "-s", session, "wait", "model.requested", "--timeout", "15000")
	initial := runTuistory(t, "-s", session, "snapshot", "--trim")
	assertContains(t, initial, "model.requested", "model.responded", "n/p:step")

	runTuistory(t, "-s", session, "type", "g")
	top := runTuistory(t, "-s", session, "snapshot", "--trim")
	assertContains(t, top, "run.started")

	runTuistory(t, "-s", session, "type", "c")
	checkpoint := runTuistory(t, "-s", session, "snapshot", "--trim")
	assertContains(t, checkpoint, "snapshot.created")

	runTuistory(t, "-s", session, "type", "q")

	compareSession := fmt.Sprintf("gollem-trace-compare-%d", time.Now().UnixNano())
	defer func() { _ = exec.Command("tuistory", "-s", compareSession, "close").Run() }()

	runTuistory(t, "launch", strconv.Quote(bin)+" trace view "+strconv.Quote(tracePath)+" "+strconv.Quote(tracePath), "-s", compareSession, "--cols", "110", "--rows", "28")
	runTuistory(t, "-s", compareSession, "wait", "diff", "--timeout", "15000")
	compare := runTuistory(t, "-s", compareSession, "snapshot", "--trim")
	assertContains(t, compare, "diff", "first divergence", "d:diverge")

	runTuistory(t, "-s", compareSession, "type", "q")
}

func runTuistory(t *testing.T, args ...string) string {
	t.Helper()
	return runCmd(t, repoRootForTest(t), "tuistory", args...)
}
