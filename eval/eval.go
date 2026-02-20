package eval

import (
	"context"
	"time"

	"github.com/trevorprater/gollem"
)

// Case represents a single evaluation test case.
type Case[T any] struct {
	Name     string
	Prompt   string
	Expected T
	Metadata map[string]any
}

// Dataset is a collection of evaluation cases.
type Dataset[T any] struct {
	Name  string
	Cases []Case[T]
}

// Evaluator scores an agent's output against expected results.
type Evaluator[T any] interface {
	Evaluate(ctx context.Context, output T, expected T) (*Score, error)
}

// Score represents an evaluation result.
type Score struct {
	Value   float64        // 0.0 to 1.0
	Reason  string
	Details map[string]any
}

// CaseResult contains the result of a single case.
type CaseResult struct {
	CaseName string
	Scores   []Score
	Output   any
	Duration time.Duration
	Usage    gollem.RunUsage
	Error    error
}

// Report contains evaluation results.
type Report struct {
	DatasetName string
	TotalCases  int
	PassedCases int
	FailedCases int
	AvgScore    float64
	Results     []CaseResult
}

// Runner executes evaluation datasets against agents.
type Runner[T any] struct {
	agent      *gollem.Agent[T]
	evaluators []Evaluator[T]
	passScore  float64
}

// NewRunner creates an evaluation runner.
func NewRunner[T any](agent *gollem.Agent[T], evaluators ...Evaluator[T]) *Runner[T] {
	return &Runner[T]{
		agent:      agent,
		evaluators: evaluators,
		passScore:  0.5,
	}
}

// WithPassScore sets the minimum average score to consider a case "passed" (default: 0.5).
func (r *Runner[T]) WithPassScore(score float64) *Runner[T] {
	r.passScore = score
	return r
}

// Run executes all cases and returns results.
func (r *Runner[T]) Run(ctx context.Context, dataset Dataset[T]) (*Report, error) {
	report := &Report{
		DatasetName: dataset.Name,
		TotalCases:  len(dataset.Cases),
		Results:     make([]CaseResult, 0, len(dataset.Cases)),
	}

	var totalScore float64
	var totalEvals int

	for _, tc := range dataset.Cases {
		start := time.Now()
		cr := CaseResult{
			CaseName: tc.Name,
		}

		result, err := r.agent.Run(ctx, tc.Prompt)
		cr.Duration = time.Since(start)
		if err != nil {
			cr.Error = err
			report.FailedCases++
			report.Results = append(report.Results, cr)
			continue
		}

		cr.Output = result.Output
		cr.Usage = result.Usage

		// Run evaluators.
		var caseTotal float64
		for _, evaluator := range r.evaluators {
			score, evalErr := evaluator.Evaluate(ctx, result.Output, tc.Expected)
			if evalErr != nil {
				cr.Error = evalErr
				break
			}
			cr.Scores = append(cr.Scores, *score)
			caseTotal += score.Value
			totalScore += score.Value
			totalEvals++
		}

		if len(cr.Scores) > 0 {
			avgCaseScore := caseTotal / float64(len(cr.Scores))
			if avgCaseScore >= r.passScore {
				report.PassedCases++
			} else {
				report.FailedCases++
			}
		} else if cr.Error != nil {
			report.FailedCases++
		}

		report.Results = append(report.Results, cr)
	}

	if totalEvals > 0 {
		report.AvgScore = totalScore / float64(totalEvals)
	}

	return report, nil
}
