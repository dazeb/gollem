package trace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

const SleepyEvidenceSchemaVersion = "gollem.sleepy.evidence.v1"

// SleepyEvidence is a trace-backed optimization evidence bundle intended for
// Sleepy's mutation/evolution loop. It keeps Gollem independent of Sleepy while
// giving Sleepy a stable substrate for ranking, replay lineage, drift, and
// evaluator-gaming checks.
type SleepyEvidence struct {
	SchemaVersion string                    `json:"schema_version"`
	GeneratedAt   time.Time                 `json:"generated_at"`
	BaselineRunID string                    `json:"baseline_run_id"`
	Candidates    []SleepyCandidateEvidence `json:"candidates"`
	Ranking       []SleepyCandidateRank     `json:"ranking"`
	Notes         []string                  `json:"notes,omitempty"`
}

// SleepyCandidateEvidence captures one candidate trace compared to a baseline.
type SleepyCandidateEvidence struct {
	CandidateID          string         `json:"candidate_id"`
	RunID                string         `json:"run_id"`
	Status               string         `json:"status"`
	Score                *float64       `json:"score,omitempty"`
	Passed               *bool          `json:"passed,omitempty"`
	Cost                 float64        `json:"cost,omitempty"`
	Tokens               int            `json:"tokens,omitempty"`
	DurationMillis       int64          `json:"duration_ms,omitempty"`
	Diff                 DiffResult     `json:"diff"`
	Replay               *ReplayState   `json:"replay,omitempty"`
	Replayable           bool           `json:"replayable"`
	ReplayError          string         `json:"replay_error,omitempty"`
	MutationEvidence     []string       `json:"mutation_evidence,omitempty"`
	TopologyEvidence     []string       `json:"topology_evidence,omitempty"`
	BehavioralDrift      []string       `json:"behavioral_drift,omitempty"`
	EvaluatorGamingFlags []string       `json:"evaluator_gaming_flags,omitempty"`
	Lineage              SleepyLineage  `json:"lineage"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

// SleepyLineage records replay/fork provenance for mutation replay.
type SleepyLineage struct {
	SourceTraceRunID string `json:"source_trace_run_id,omitempty"`
	SourceSnapshotID string `json:"source_snapshot_id,omitempty"`
	ParentRunID      string `json:"parent_run_id,omitempty"`
}

// SleepyCandidateRank is a deterministic ranking row for candidate selection.
type SleepyCandidateRank struct {
	CandidateID string   `json:"candidate_id"`
	RunID       string   `json:"run_id"`
	Rank        int      `json:"rank"`
	Score       *float64 `json:"score,omitempty"`
	Passed      *bool    `json:"passed,omitempty"`
	Cost        float64  `json:"cost,omitempty"`
	Tokens      int      `json:"tokens,omitempty"`
	Reason      string   `json:"reason"`
}

// BuildSleepyEvidence compares candidate traces against a baseline and returns
// a stable evidence bundle Sleepy can use as an optimization substrate.
func BuildSleepyEvidence(baseline *Artifact, candidates []*Artifact) (*SleepyEvidence, error) {
	if baseline == nil {
		return nil, errors.New("nil baseline trace")
	}
	evidence := &SleepyEvidence{
		SchemaVersion: SleepyEvidenceSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		BaselineRunID: displayRunID(baseline),
	}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		row := sleepyCandidateEvidence(baseline, candidate)
		evidence.Candidates = append(evidence.Candidates, row)
	}
	evidence.Ranking = rankSleepyCandidates(evidence.Candidates)
	if len(evidence.Candidates) == 0 {
		evidence.Notes = append(evidence.Notes, "no candidate traces supplied")
	}
	return evidence, nil
}

// WriteSleepyEvidence writes a human-readable JSON evidence bundle.
func WriteSleepyEvidence(w io.Writer, evidence *SleepyEvidence) error {
	if evidence == nil {
		return errors.New("nil sleepy evidence")
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(evidence)
}

// WriteSleepyEvidenceFile writes evidence to path. A path of "-" writes stdout.
func WriteSleepyEvidenceFile(path string, evidence *SleepyEvidence) error {
	if strings.TrimSpace(path) == "" || path == "-" {
		return WriteSleepyEvidence(os.Stdout, evidence)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteSleepyEvidence(f, evidence)
}

func sleepyCandidateEvidence(baseline, candidate *Artifact) SleepyCandidateEvidence {
	diff := Diff(baseline, candidate)
	replay, replayErr := BuildReplayState(candidate, ReplayOptions{Mode: "simulated"})
	row := SleepyCandidateEvidence{
		CandidateID:          sleepyCandidateID(candidate),
		RunID:                displayRunID(candidate),
		Status:               candidate.Summary.Status,
		Score:                evaluatorScore(candidate),
		Passed:               evaluatorPassed(candidate),
		Cost:                 costTotal(candidate.Summary.Cost),
		Tokens:               candidate.Summary.Usage.TotalTokens(),
		DurationMillis:       candidate.Summary.DurationMillis,
		Diff:                 diff,
		Replay:               replay,
		Replayable:           replayErr == nil,
		MutationEvidence:     diff.ArtifactDelta,
		TopologyEvidence:     diff.TopologyDelta,
		BehavioralDrift:      sleepyBehavioralDrift(diff),
		EvaluatorGamingFlags: sleepyEvaluatorGamingFlags(candidate, diff),
		Lineage:              sleepyLineage(candidate),
		Metadata:             cloneMetadata(candidate.Metadata),
	}
	if replayErr != nil {
		row.ReplayError = replayErr.Error()
	}
	return row
}

func rankSleepyCandidates(candidates []SleepyCandidateEvidence) []SleepyCandidateRank {
	ranked := append([]SleepyCandidateEvidence(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		left, right := ranked[i], ranked[j]
		leftPassed, rightPassed := boolScore(left.Passed), boolScore(right.Passed)
		if leftPassed != rightPassed {
			return leftPassed > rightPassed
		}
		if floatScore(left.Score) != floatScore(right.Score) {
			return floatScore(left.Score) > floatScore(right.Score)
		}
		if left.Tokens != right.Tokens {
			return left.Tokens < right.Tokens
		}
		if left.Cost != right.Cost {
			return left.Cost < right.Cost
		}
		return left.RunID < right.RunID
	})
	out := make([]SleepyCandidateRank, 0, len(ranked))
	for idx, candidate := range ranked {
		reason := "ranked by evaluator pass, score, tokens, and cost"
		if len(candidate.EvaluatorGamingFlags) > 0 {
			reason = "flagged: " + strings.Join(candidate.EvaluatorGamingFlags, "; ")
		}
		out = append(out, SleepyCandidateRank{
			CandidateID: candidate.CandidateID,
			RunID:       candidate.RunID,
			Rank:        idx + 1,
			Score:       candidate.Score,
			Passed:      candidate.Passed,
			Cost:        candidate.Cost,
			Tokens:      candidate.Tokens,
			Reason:      reason,
		})
	}
	return out
}

func sleepyCandidateID(artifact *Artifact) string {
	for _, key := range []string{"sleepy_candidate_id", "candidate_id", "mutation_id"} {
		if value := metadataString(artifact.Metadata, key); value != "" {
			return value
		}
	}
	return displayRunID(artifact)
}

func evaluatorScore(artifact *Artifact) *float64 {
	if artifact == nil || artifact.Summary.Evaluator == nil {
		return nil
	}
	return artifact.Summary.Evaluator.Score
}

func evaluatorPassed(artifact *Artifact) *bool {
	if artifact == nil || artifact.Summary.Evaluator == nil {
		return nil
	}
	return artifact.Summary.Evaluator.Passed
}

func sleepyBehavioralDrift(diff DiffResult) []string {
	if !diff.SemanticDelta.Changed {
		return nil
	}
	return append([]string(nil), diff.SemanticDelta.Notes...)
}

func sleepyEvaluatorGamingFlags(candidate *Artifact, diff DiffResult) []string {
	var flags []string
	passed := evaluatorPassed(candidate)
	score := evaluatorScore(candidate)
	if passed != nil && *passed && candidate.Summary.Status != "succeeded" {
		flags = append(flags, "evaluator passed but run did not succeed")
	}
	if score != nil && *score > 0 && diff.RetryErrorDelta.Failures > 0 {
		flags = append(flags, "positive evaluator score despite runtime failures")
	}
	if diff.SemanticDelta.FinalOutputChanged && diff.EvaluatorDelta != nil && diff.EvaluatorDelta.ScoreDelta != nil && *diff.EvaluatorDelta.ScoreDelta > 0 {
		flags = append(flags, "score improved while final output changed")
	}
	return flags
}

func sleepyLineage(artifact *Artifact) SleepyLineage {
	if artifact == nil {
		return SleepyLineage{}
	}
	return SleepyLineage{
		SourceTraceRunID: metadataString(artifact.Metadata, "resume_source_trace_run_id"),
		SourceSnapshotID: metadataString(artifact.Metadata, "resume_source_snapshot_id"),
		ParentRunID:      metadataString(artifact.Metadata, "resume_parent_run_id"),
	}
}

func boolScore(value *bool) int {
	if value == nil || !*value {
		return 0
	}
	return 1
}

func floatScore(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(metadata[key]))
}
