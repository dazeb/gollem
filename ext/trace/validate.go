package trace

import (
	"errors"
	"fmt"
	"strings"
)

// ValidateArtifact checks structural invariants for canonical trace artifacts.
func ValidateArtifact(artifact *Artifact) error {
	if artifact == nil {
		return errors.New("nil trace artifact")
	}
	if artifact.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unexpected schema version %q", artifact.SchemaVersion)
	}
	runID := strings.TrimSpace(artifact.Run.ID)
	if runID == "" && artifact.Trace != nil {
		runID = strings.TrimSpace(artifact.Trace.RunID)
	}
	if runID == "" {
		return errors.New("trace is missing run id")
	}
	if artifact.Run.ID != "" && artifact.Trace != nil && artifact.Trace.RunID != "" && artifact.Run.ID != artifact.Trace.RunID {
		return fmt.Errorf("trace run id mismatch: run metadata %q != embedded trace %q", artifact.Run.ID, artifact.Trace.RunID)
	}
	if artifact.Summary.Status != "" && !validTraceStatus(artifact.Summary.Status) {
		return fmt.Errorf("invalid summary status %q", artifact.Summary.Status)
	}
	if err := validateTraceEvents(runID, artifact.Events); err != nil {
		return err
	}
	if err := validateTraceSnapshots(artifact.Snapshots); err != nil {
		return err
	}
	if artifact.Summary.Status != "running" {
		if err := ValidateReplay(artifact); err != nil {
			return err
		}
	} else if err := validateRunningReplayPolicies(artifact.Events); err != nil {
		return err
	}
	return nil
}

func validateTraceEvents(runID string, events []Event) error {
	agentIDs := map[string]bool{runID: true}
	seenIDs := make(map[string]bool, len(events))
	for _, event := range events {
		if strings.TrimSpace(event.AgentID) != "" {
			agentIDs[event.AgentID] = true
		}
	}
	for i, event := range events {
		wantSeq := i + 1
		if event.Seq != wantSeq {
			return fmt.Errorf("event sequence at index %d = %d, want %d", i, event.Seq, wantSeq)
		}
		if strings.TrimSpace(event.ID) == "" {
			return fmt.Errorf("event %03d is missing id", event.Seq)
		}
		if seenIDs[event.ID] {
			return fmt.Errorf("event %03d duplicates id %q", event.Seq, event.ID)
		}
		seenIDs[event.ID] = true
		if strings.TrimSpace(event.Kind) == "" {
			return fmt.Errorf("event %03d is missing kind", event.Seq)
		}
		if !knownTraceEventKind(event.Kind) {
			return fmt.Errorf("event %03d has unknown kind %q", event.Seq, event.Kind)
		}
		if strings.TrimSpace(event.ReplayPolicy) == "" {
			return fmt.Errorf("event %03d is missing replay policy", event.Seq)
		}
		if want := expectedReplayPolicy(event.Kind); want != "" && event.ReplayPolicy != want {
			return fmt.Errorf("event %03d replay policy = %q, want %q", event.Seq, event.ReplayPolicy, want)
		}
		if event.Step < 0 {
			return fmt.Errorf("event %03d has negative step %d", event.Seq, event.Step)
		}
		if event.DurationMillis < 0 {
			return fmt.Errorf("event %03d has negative duration %d", event.Seq, event.DurationMillis)
		}
		if event.CausalParentID != "" && !agentIDs[event.CausalParentID] {
			return fmt.Errorf("event %03d causal parent %q is not present in run lineage", event.Seq, event.CausalParentID)
		}
		if err := validateTraceEventPayload(event); err != nil {
			return err
		}
	}
	return nil
}

func validateTraceSnapshots(records []SnapshotRecord) error {
	seen := make(map[string]bool, len(records))
	for i, record := range records {
		if strings.TrimSpace(record.ID) == "" {
			return fmt.Errorf("snapshot %d is missing id", i+1)
		}
		if seen[record.ID] {
			return fmt.Errorf("snapshot %d duplicates id %q", i+1, record.ID)
		}
		seen[record.ID] = true
		if record.Step < 0 {
			return fmt.Errorf("snapshot %s has negative step %d", record.ID, record.Step)
		}
		if record.Snapshot == nil {
			return fmt.Errorf("snapshot %s is missing payload", record.ID)
		}
		if _, err := DecodeSnapshotRecord(record); err != nil {
			return fmt.Errorf("snapshot %s cannot be decoded: %w", record.ID, err)
		}
	}
	return nil
}

func validateRunningReplayPolicies(events []Event) error {
	for _, event := range events {
		if strings.TrimSpace(event.ReplayPolicy) == "" {
			return fmt.Errorf("event %03d is missing replay policy", event.Seq)
		}
	}
	return nil
}

func validateTraceEventPayload(event Event) error {
	switch event.Kind {
	case "model.requested", "model.responded", "model.failed":
		if event.RequestID == "" && event.Step == 0 {
			return fmt.Errorf("event %03d %s needs request id or step", event.Seq, event.Kind)
		}
	case "tool.called", "tool.completed", "tool.failed":
		if event.RequestID == "" && firstPayloadString(event, "tool_call_id") == "" && firstPayloadString(event, "tool_name") == "" {
			return fmt.Errorf("event %03d %s needs tool identity", event.Seq, event.Kind)
		}
	case "approval.requested", "approval.resolved", "deferred.requested", "deferred.resolved":
		if event.RequestID == "" && firstPayloadString(event, "tool_call_id") == "" {
			return fmt.Errorf("event %03d %s needs tool call id", event.Seq, event.Kind)
		}
	}
	return nil
}

func validTraceStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "waiting", "running", "aborted":
		return true
	default:
		return false
	}
}

func knownTraceEventKind(kind string) bool {
	switch kind {
	case "run.started", "run.completed", "run.failed",
		"turn.started", "turn.completed",
		"model.requested", "model.delta", "model.responded", "model.failed",
		"guardrail.evaluated",
		"tool.called", "tool.completed", "tool.failed",
		"approval.requested", "approval.resolved",
		"deferred.requested", "deferred.resolved",
		"wait.started", "wait.resolved",
		"retry.scheduled", "checkpoint.created", "snapshot.created",
		"topology.transitioned", "evaluator.completed",
		"artifact.changed", "error.raised":
		return true
	default:
		return strings.HasPrefix(kind, "trace.") || strings.HasPrefix(kind, "custom.")
	}
}

func expectedReplayPolicy(kind string) string {
	switch kind {
	case "model.requested", "model.delta", "model.responded", "model.failed",
		"guardrail.evaluated",
		"tool.called", "tool.completed", "tool.failed",
		"approval.requested", "approval.resolved",
		"deferred.requested", "deferred.resolved",
		"retry.scheduled", "topology.transitioned", "evaluator.completed",
		"artifact.changed", "error.raised":
		return "recorded"
	case "checkpoint.created":
		return "checkpoint"
	case "snapshot.created":
		return "snapshot"
	case "run.started", "run.completed", "run.failed", "turn.started", "turn.completed", "wait.started", "wait.resolved":
		return "inspect"
	default:
		return ""
	}
}
