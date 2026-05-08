package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

const traceCLIE2ECommandTimeout = 2 * time.Minute

func TestTraceCLI14HourReplayDemoPath(t *testing.T) {
	root := repoRootForTest(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "gollem")
	runCmd(t, root, "go", "build", "-o", bin, "./cmd/gollem")

	baseTrace := filepath.Join(tmp, "base.trace.json")
	baseStream := filepath.Join(tmp, "base.trace.jsonl")
	forkSnapshot := filepath.Join(tmp, "fork.snapshot.json")
	forkTrace := filepath.Join(tmp, "fork.trace.json")
	continuedForkTrace := filepath.Join(tmp, "continued-fork.trace.json")
	liveReexecTrace := filepath.Join(tmp, "live-reexec.trace.json")
	sleepyEvidence := filepath.Join(tmp, "sleepy-evidence.json")
	evaluatedTrace := filepath.Join(tmp, "evaluated.trace.json")
	redactedTrace := filepath.Join(tmp, "redacted.trace.json")
	compactTrace := filepath.Join(tmp, "compact.trace.json")

	runCmd(t, root, bin, "run", "--provider", "test", "--no-code-mode", "--trace-out", baseTrace, "--trace-stream", baseStream, "hello secret-e2e")
	streamData, err := os.ReadFile(baseStream)
	if err != nil {
		t.Fatalf("read trace stream: %v", err)
	}
	assertContains(t, string(streamData), `"kind":"run.started"`, `"kind":"run.completed"`)
	inspect := runCmd(t, root, bin, "trace", "inspect", baseTrace, "--events", "20")
	assertContains(t, inspect, "schema: gollem.trace.v1", "snapshot.created", "requests: 1")

	replay := runCmd(t, root, bin, "trace", "replay", baseTrace, "--mode", "strict")
	assertContains(t, replay, "strict replay validation: ok", "model.requested", "model.responded")

	runCmd(t, root, bin, "trace", "fork", baseTrace,
		"--from-kind", "model.responded",
		"--system-prompt", "branch system",
		"--append-user", "try a branch",
		"--out", forkSnapshot,
	)
	runCmd(t, root, bin, "run", "--provider", "test", "--no-code-mode", "--resume-snapshot", forkSnapshot, "--trace-out", forkTrace, "continue")
	runCmd(t, root, bin, "trace", "fork", baseTrace,
		"--from-kind", "model.responded",
		"--append-user", "try a one-command branch",
		"--continue",
		"--provider", "test",
		"--run-arg", "--no-code-mode",
		"--out", continuedForkTrace,
	)
	liveReexec := runCmd(t, root, bin, "trace", "replay", baseTrace,
		"--mode", "live-reexec",
		"--from-kind", "model.responded",
		"--planner-prompt", "try a live re-exec planner",
		"--append-user", "try a live replay branch",
		"--provider", "test",
		"--run-arg", "--no-code-mode",
		"--out", liveReexecTrace,
	)
	assertContains(t, liveReexec, "live-reexec runtime-boundary replay", "live re-execution: completed")

	baseArtifact, err := traceutil.ReadFile(baseTrace)
	if err != nil {
		t.Fatalf("read base trace: %v", err)
	}
	forkArtifact, err := traceutil.ReadFile(forkTrace)
	if err != nil {
		t.Fatalf("read fork trace: %v", err)
	}
	if forkArtifact.Run.ID == baseArtifact.Run.ID {
		t.Fatalf("expected fork trace to have a distinct run id, got %q", forkArtifact.Run.ID)
	}
	if got := metadataString(forkArtifact.Metadata, "resume_trace_policy"); got != "fresh_segment" {
		t.Fatalf("resume trace policy = %q, want fresh_segment", got)
	}
	if got := metadataString(forkArtifact.Metadata, "resume_source_trace_run_id"); got != baseArtifact.Run.ID {
		t.Fatalf("resume source trace run id = %q, want %q", got, baseArtifact.Run.ID)
	}
	if got := metadataString(forkArtifact.Metadata, "resume_source_snapshot_id"); !strings.HasPrefix(got, "synthetic_step_") {
		t.Fatalf("resume source snapshot id = %q, want synthetic boundary snapshot", got)
	}
	if forkArtifact.Summary.Requests != 1 {
		t.Fatalf("expected fork trace to contain only fresh segment requests, got %d", forkArtifact.Summary.Requests)
	}
	if forkArtifact.Trace == nil || len(forkArtifact.Trace.Requests) != 1 {
		t.Fatalf("expected one embedded request in fork trace, got %+v", forkArtifact.Trace)
	}
	if forkArtifact.Trace.Requests[0].Sequence != 1 {
		t.Fatalf("expected fresh fork trace request sequence to restart at 1, got %d", forkArtifact.Trace.Requests[0].Sequence)
	}
	if forkArtifact.Trace.Requests[0].MessageCount <= 1 {
		t.Fatalf("expected resumed request to include restored conversation history, got %d messages", forkArtifact.Trace.Requests[0].MessageCount)
	}
	continuedArtifact, err := traceutil.ReadFile(continuedForkTrace)
	if err != nil {
		t.Fatalf("read continued fork trace: %v", err)
	}
	if got := metadataString(continuedArtifact.Metadata, "resume_trace_policy"); got != "fresh_segment" {
		t.Fatalf("continued fork resume trace policy = %q, want fresh_segment", got)
	}
	if continuedArtifact.Run.ID == baseArtifact.Run.ID {
		t.Fatalf("expected continued fork trace to have a distinct run id, got %q", continuedArtifact.Run.ID)
	}
	liveReexecArtifact, err := traceutil.ReadFile(liveReexecTrace)
	if err != nil {
		t.Fatalf("read live reexec trace: %v", err)
	}
	if got := metadataString(liveReexecArtifact.Metadata, "resume_trace_policy"); got != "fresh_segment" {
		t.Fatalf("live reexec resume trace policy = %q, want fresh_segment", got)
	}
	if liveReexecArtifact.Run.ID == baseArtifact.Run.ID {
		t.Fatalf("expected live reexec trace to have a distinct run id, got %q", liveReexecArtifact.Run.ID)
	}

	diffJSON := runCmd(t, root, bin, "trace", "diff", baseTrace, forkTrace, "--format", "json")
	var diff traceutil.DiffResult
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("decode diff JSON: %v\n%s", err, diffJSON)
	}
	if diff.FirstDivergence == nil {
		t.Fatalf("expected fork to diverge:\n%s", diffJSON)
	}
	if len(diff.Narrative) == 0 {
		t.Fatalf("expected diff narrative:\n%s", diffJSON)
	}
	regressJSON := runCmd(t, root, bin, "trace", "regress", baseTrace, forkTrace, "--require-status", "succeeded", "--format", "json")
	var regress traceutil.RegressionReport
	if err := json.Unmarshal([]byte(regressJSON), &regress); err != nil {
		t.Fatalf("decode regress JSON: %v\n%s", err, regressJSON)
	}
	if !regress.Passed || len(regress.Cases) != 1 {
		t.Fatalf("unexpected regression report:\n%s", regressJSON)
	}
	runCmd(t, root, bin, "trace", "evaluate", forkTrace, "--evaluator", "status-succeeded", "--out", evaluatedTrace)
	evaluatedArtifact, err := traceutil.ReadFile(evaluatedTrace)
	if err != nil {
		t.Fatalf("read evaluated trace: %v", err)
	}
	if evaluatedArtifact.Summary.Evaluator == nil || evaluatedArtifact.Summary.Evaluator.Passed == nil || !*evaluatedArtifact.Summary.Evaluator.Passed {
		t.Fatalf("unexpected evaluated summary: %+v", evaluatedArtifact.Summary.Evaluator)
	}
	runCmd(t, root, bin, "trace", "sleepy", baseTrace, forkTrace, "--out", sleepyEvidence)
	sleepyData, err := os.ReadFile(sleepyEvidence)
	if err != nil {
		t.Fatalf("read sleepy evidence: %v", err)
	}
	assertContains(t, string(sleepyData), "gollem.sleepy.evidence.v1", "ranking", "replayable")

	runCmd(t, root, bin, "trace", "redact", baseTrace, "--pattern", "secret-e2e", "--drop-trace", "--out", redactedTrace)
	redactedData, err := os.ReadFile(redactedTrace)
	if err != nil {
		t.Fatalf("read redacted trace: %v", err)
	}
	if strings.Contains(string(redactedData), "secret-e2e") {
		t.Fatalf("redacted trace still contains secret:\n%s", string(redactedData))
	}
	runCmd(t, root, bin, "trace", "validate", redactedTrace)

	runCmd(t, root, bin, "trace", "compact", baseTrace, "--payload-limit", "64", "--keep-snapshots", "1", "--out", compactTrace)
	validate := runCmd(t, root, bin, "trace", "validate", compactTrace)
	assertContains(t, validate, "ok: gollem.trace.v1")
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func runCmd(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), traceCLIE2ECommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOLLEM_TOP_LEVEL_PERSONALITY=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("%s %s timed out\nstdout:\n%s\nstderr:\n%s", name, strings.Join(args, " "), stdout.String(), stderr.String())
		}
		t.Fatalf("%s %s failed: %v\nstdout:\n%s\nstderr:\n%s", name, strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String() + stderr.String()
}

func assertContains(t *testing.T, value string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(value, want) {
			t.Fatalf("output missing %q:\n%s", want, value)
		}
	}
}
