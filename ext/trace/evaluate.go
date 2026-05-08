package trace

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// EvaluateOptions selects a built-in trace evaluator.
type EvaluateOptions struct {
	Evaluator string
	Expected  string
}

// EvaluateTrace runs a named trace-level evaluator and appends replayable
// evaluator evidence to the artifact.
func EvaluateTrace(artifact *Artifact, opts EvaluateOptions) (*Artifact, error) {
	if artifact == nil {
		return nil, errors.New("nil trace artifact")
	}
	name := strings.TrimSpace(opts.Evaluator)
	if name == "" {
		return nil, errors.New("trace evaluator name is required")
	}
	out, err := cloneArtifact(artifact)
	if err != nil {
		return nil, err
	}
	summary, err := runTraceEvaluator(out, name, opts.Expected)
	if err != nil {
		return nil, err
	}
	out.Summary.Evaluator = summary
	out.Events = core.NormalizeTraceEvents(append(out.Events, Event{
		Kind:         "evaluator.completed",
		Timestamp:    traceEvaluatorTimestamp(out),
		AgentID:      displayRunID(out),
		ReplayPolicy: "recorded",
		Payload: compactMap(map[string]any{
			"name":    summary.Name,
			"score":   summary.Score,
			"passed":  summary.Passed,
			"results": summary.Results,
		}),
	}))
	return out, nil
}

func runTraceEvaluator(artifact *Artifact, name, expected string) (*EvaluatorSummary, error) {
	normalized := strings.TrimSpace(strings.ToLower(name))
	score := 0.0
	passed := false
	results := map[string]any{"evaluator": name}
	switch normalized {
	case "status-succeeded", "succeeded":
		passed = artifact.Summary.Status == "succeeded" || artifact.Summary.Success
		score = traceBoolScore(passed)
		results["status"] = artifact.Summary.Status
	case "no-errors":
		passed = artifact.Summary.Error == "" && retryErrorDelta(nil, artifact.Events).ErrorsRaised == 0 && retryErrorDelta(nil, artifact.Events).Failures == 0
		score = traceBoolScore(passed)
		results["error"] = artifact.Summary.Error
	case "contains-output", "final-output-contains":
		if expected == "" {
			return nil, fmt.Errorf("evaluator %q requires --expected", name)
		}
		output := lastOutputText(artifact)
		passed = strings.Contains(output, expected)
		score = traceBoolScore(passed)
		results["expected"] = expected
		results["output"] = truncateLine(output, 240)
	case "exact-output", "final-output-exact":
		if expected == "" {
			return nil, fmt.Errorf("evaluator %q requires --expected", name)
		}
		output := lastOutputText(artifact)
		passed = output == expected
		score = traceBoolScore(passed)
		results["expected"] = expected
		results["output"] = truncateLine(output, 240)
	default:
		if status, ok := strings.CutPrefix(normalized, "status:"); ok && strings.TrimSpace(status) != "" {
			want := strings.TrimSpace(status)
			passed = artifact.Summary.Status == want
			score = traceBoolScore(passed)
			results["expected_status"] = want
			results["status"] = artifact.Summary.Status
			break
		}
		return nil, fmt.Errorf("unknown trace evaluator %q (supported: status-succeeded, no-errors, contains-output, exact-output, status:<value>)", name)
	}
	return &EvaluatorSummary{
		Name:    name,
		Score:   &score,
		Passed:  &passed,
		Results: results,
	}, nil
}

func cloneArtifact(artifact *Artifact) (*Artifact, error) {
	data, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf("clone trace artifact: %w", err)
	}
	var out Artifact
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("clone trace artifact: %w", err)
	}
	return &out, nil
}

func traceEvaluatorTimestamp(artifact *Artifact) time.Time {
	if artifact.Run.EndedAt.IsZero() {
		return time.Now()
	}
	return artifact.Run.EndedAt
}

func traceBoolScore(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
