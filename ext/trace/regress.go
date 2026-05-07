package trace

import (
	"encoding/json"
	"fmt"
	"io"
)

// RegressionOptions configures trace-backed regression checks.
type RegressionOptions struct {
	MaxCostDelta  *float64 `json:"max_cost_delta,omitempty"`
	MaxTokenDelta *int     `json:"max_token_delta,omitempty"`
	RequireStatus string   `json:"require_status,omitempty"`
}

// RegressionReport is a portable summary of baseline-vs-variant trace checks.
type RegressionReport struct {
	BaselineID string           `json:"baseline_id"`
	Passed     bool             `json:"passed"`
	Cases      []RegressionCase `json:"cases"`
}

// RegressionCase describes one variant compared to the baseline trace.
type RegressionCase struct {
	VariantID string     `json:"variant_id"`
	Passed    bool       `json:"passed"`
	Failures  []string   `json:"failures,omitempty"`
	Diff      DiffResult `json:"diff"`
}

// Regress compares variant traces against a baseline and evaluates thresholds.
func Regress(baseline *Artifact, variants []*Artifact, opts RegressionOptions) RegressionReport {
	report := RegressionReport{
		BaselineID: displayRunID(baseline),
		Passed:     true,
		Cases:      make([]RegressionCase, 0, len(variants)),
	}
	for _, variant := range variants {
		diff := Diff(baseline, variant)
		c := RegressionCase{
			VariantID: diff.VariantID,
			Passed:    true,
			Diff:      diff,
		}
		if opts.RequireStatus != "" && diff.VariantStatus != opts.RequireStatus {
			c.Failures = append(c.Failures, fmt.Sprintf("status %q != required %q", diff.VariantStatus, opts.RequireStatus))
		}
		if opts.MaxTokenDelta != nil && diff.UsageDelta.TotalTokens > *opts.MaxTokenDelta {
			c.Failures = append(c.Failures, fmt.Sprintf("token delta %+d exceeds %+d", diff.UsageDelta.TotalTokens, *opts.MaxTokenDelta))
		}
		if opts.MaxCostDelta != nil && diff.CostDelta > *opts.MaxCostDelta {
			c.Failures = append(c.Failures, fmt.Sprintf("cost delta %+0.6f exceeds %+0.6f", diff.CostDelta, *opts.MaxCostDelta))
		}
		if len(c.Failures) > 0 {
			c.Passed = false
			report.Passed = false
		}
		report.Cases = append(report.Cases, c)
	}
	return report
}

// WriteRegressionReport writes a human-readable regression report.
func WriteRegressionReport(w io.Writer, report RegressionReport) error {
	status := "passed"
	if !report.Passed {
		status = "failed"
	}
	fmt.Fprintf(w, "Trace regression: %s\n", status)
	fmt.Fprintf(w, "baseline: %s\n", report.BaselineID)
	for _, c := range report.Cases {
		caseStatus := "passed"
		if !c.Passed {
			caseStatus = "failed"
		}
		fmt.Fprintf(w, "\nvariant: %s (%s)\n", c.VariantID, caseStatus)
		if len(c.Failures) > 0 {
			fmt.Fprintln(w, "failures:")
			for _, failure := range c.Failures {
				fmt.Fprintf(w, "  %s\n", failure)
			}
		}
		if c.Diff.FirstDivergence == nil {
			fmt.Fprintln(w, "first divergence: none")
		} else {
			fmt.Fprintf(w, "first divergence: event %d\n", c.Diff.FirstDivergence.Index+1)
		}
		fmt.Fprintf(w, "status: %s -> %s\n", c.Diff.BaselineStatus, c.Diff.VariantStatus)
		fmt.Fprintf(w, "tokens: %+d total (%+d in, %+d out)\n", c.Diff.UsageDelta.TotalTokens, c.Diff.UsageDelta.InputTokens, c.Diff.UsageDelta.OutputTokens)
		fmt.Fprintf(w, "cost: %+0.6f\n", c.Diff.CostDelta)
		if c.Diff.EvaluatorDelta != nil && c.Diff.EvaluatorDelta.ScoreDelta != nil {
			fmt.Fprintf(w, "evaluator score: %+0.4f\n", *c.Diff.EvaluatorDelta.ScoreDelta)
		}
	}
	return nil
}

// WriteRegressionReportJSON writes a JSON regression report.
func WriteRegressionReportJSON(w io.Writer, report RegressionReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
