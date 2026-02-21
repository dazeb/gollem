// Example evaluation demonstrates quality measurement with datasets and
// multiple evaluator styles. It runs offline using TestModel.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/eval"
)

func main() {
	model := core.NewTestModel(
		core.TextResponse("Paris"),
		core.TextResponse("Tokyo"),
		core.TextResponse("Rio de Janeiro"), // intentionally wrong for Brasilia
		core.TextResponse("Ottawa"),
	)

	agent := core.NewAgent[string](model,
		core.WithSystemPrompt[string]("You are a geography quiz assistant. Return just the city name."),
	)

	dataset := eval.Dataset[string]{
		Name: "world-capitals",
		Cases: []eval.Case[string]{
			{Name: "france", Prompt: "What is the capital of France?", Expected: "Paris"},
			{Name: "japan", Prompt: "What is the capital of Japan?", Expected: "Tokyo"},
			{Name: "brazil", Prompt: "What is the capital of Brazil?", Expected: "Brasilia"},
			{Name: "canada", Prompt: "What is the capital of Canada?", Expected: "Ottawa"},
		},
	}

	fmt.Println("=== Exact Match ===")
	exactRunner := eval.NewRunner(agent, eval.ExactMatch[string]()).WithPassScore(0.75)
	exactReport, err := exactRunner.Run(context.Background(), dataset)
	if err != nil {
		log.Fatal(err)
	}
	printReport(exactReport)

	fmt.Println("\n=== Contains ===")
	model.Reset()
	containsRunner := eval.NewRunner(agent, eval.Contains()).WithPassScore(0.75)
	containsReport, err := containsRunner.Run(context.Background(), dataset)
	if err != nil {
		log.Fatal(err)
	}
	printReport(containsReport)

	fmt.Println("\n=== Custom (starts-with) ===")
	model.Reset()
	customEval := eval.Custom[string](func(_ context.Context, output, expected string) (*eval.Score, error) {
		if expected == "" {
			return &eval.Score{Value: 0.0, Reason: "empty expected value"}, nil
		}
		if strings.HasPrefix(strings.ToLower(output), strings.ToLower(string(expected[0]))) {
			return &eval.Score{Value: 1.0, Reason: "starts with the expected letter"}, nil
		}
		return &eval.Score{Value: 0.0, Reason: "different starting letter"}, nil
	})
	customRunner := eval.NewRunner(agent, customEval).WithPassScore(0.75)
	customReport, err := customRunner.Run(context.Background(), dataset)
	if err != nil {
		log.Fatal(err)
	}
	printReport(customReport)
}

func printReport(report *eval.Report) {
	fmt.Printf("Dataset: %s\n", report.DatasetName)
	fmt.Printf("Cases: %d | Passed: %d | Failed: %d | Avg score: %.2f\n",
		report.TotalCases, report.PassedCases, report.FailedCases, report.AvgScore)

	for _, cr := range report.Results {
		if cr.Error != nil {
			fmt.Printf("  - %-8s error: %v\n", cr.CaseName, cr.Error)
			continue
		}
		score := 0.0
		reason := "n/a"
		if len(cr.Scores) > 0 {
			score = cr.Scores[0].Value
			reason = cr.Scores[0].Reason
		}
		fmt.Printf("  - %-8s output=%-15v score=%.2f (%s)\n", cr.CaseName, cr.Output, score, reason)
	}
}
