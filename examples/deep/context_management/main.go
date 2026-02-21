// Example context_management demonstrates the deep package's three-tier
// context compression for long-running agents. The ContextManager automatically
// offloads large tool results, compresses tool call inputs, and summarizes
// older conversation turns to stay within token limits.
//
// This example uses TestModel so it compiles and runs without any API keys.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/deep"
)

// AnalysisResult is the structured output type.
type AnalysisResult struct {
	Summary    string `json:"summary" jsonschema:"description=Summary of the analysis"`
	ItemCount  int    `json:"item_count" jsonschema:"description=Number of items analyzed"`
	Conclusion string `json:"conclusion" jsonschema:"description=Final conclusion"`
}

func main() {
	// Create a TestModel for demonstrating the context manager.
	model := core.NewTestModel(
		// First response: model calls a tool that returns a large result.
		core.ToolCallResponse("analyze_data", `{"dataset":"sales_q4"}`),
		// Second response: model produces the final result.
		core.ToolCallResponse("final_result", `{
			"summary": "Q4 sales data analyzed across 1200 records with segment-level trends",
			"item_count": 1200,
			"conclusion": "Revenue increased 15% quarter-over-quarter, with strongest gains in enterprise plans"
		}`),
	)

	// Create a tool that simulates returning a large result.
	analyzeTool := core.FuncTool[struct {
		Dataset string `json:"dataset" jsonschema:"description=Name of the dataset to analyze"`
	}](
		"analyze_data",
		"Analyze a dataset and return findings",
		func(_ context.Context, params struct {
			Dataset string `json:"dataset" jsonschema:"description=Name of the dataset to analyze"`
		}) (string, error) {
			// Build a deliberately large payload so tier-1 offloading is exercised.
			var b strings.Builder
			b.WriteString("Executive summary for ")
			b.WriteString(params.Dataset)
			b.WriteString(":\n")
			for i := 1; i <= 30; i++ {
				fmt.Fprintf(&b, "Segment %02d: revenue +%d%%, margin +%d%%, churn -%d%%\n", i, 10+i%7, 3+i%4, i%5)
			}
			return b.String(), nil
		},
	)

	// Set up the ContextManager with a filesystem-backed store.
	storeDir := filepath.Join(os.TempDir(), "gollem-context-example")
	store, err := deep.NewFileStore(storeDir)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Cleanup()

	cm := deep.NewContextManager(model,
		deep.WithMaxContextTokens(240),
		deep.WithOffloadThreshold(40),
		deep.WithCompressionThreshold(0.70),
		deep.WithContextStore(store),
	)

	// Create an agent with the context manager as a history processor.
	// The ContextManager will automatically compress context when it
	// approaches the token limit.
	agent := core.NewAgent[AnalysisResult](model,
		core.WithSystemPrompt[AnalysisResult]("You are a data analyst. Analyze datasets and provide summaries."),
		core.WithTools[AnalysisResult](analyzeTool),
		core.WithHistoryProcessor[AnalysisResult](cm.AsHistoryProcessor()),
	)

	result, err := agent.Run(context.Background(), "Analyze the Q4 sales data and summarize the findings")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Analysis Result ===")
	fmt.Printf("Summary: %s\n", result.Output.Summary)
	fmt.Printf("Items analyzed: %d\n", result.Output.ItemCount)
	fmt.Printf("Conclusion: %s\n", result.Output.Conclusion)
	fmt.Printf("Requests: %d, Tool calls: %d\n", result.Usage.Requests, result.Usage.ToolCalls)
	files, err := os.ReadDir(storeDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Offloaded context files: %d (%s)\n", len(files), storeDir)

	// Demonstrate the LongRunAgent, which bundles context management
	// and optional planning into a single wrapper.
	fmt.Println("\n=== LongRunAgent Demo ===")

	longModel := core.NewTestModel(
		core.ToolCallResponse("final_result", `{
			"summary": "Comprehensive analysis complete with anomaly scan and remediation plan",
			"item_count": 5000,
			"conclusion": "Core systems healthy; recommend proactive scaling for peak traffic windows"
		}`),
	)

	lra := deep.NewLongRunAgent[AnalysisResult](longModel,
		deep.WithContextWindow[AnalysisResult](100000),
		deep.WithPlanningEnabled[AnalysisResult](),
		deep.WithLongRunAgentOptions[AnalysisResult](
			core.WithSystemPrompt[AnalysisResult]("You are a thorough analyst. Plan your work, then execute."),
		),
	)

	longResult, err := lra.Run(context.Background(), "Perform a comprehensive system health check")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Summary: %s\n", longResult.Output.Summary)
	fmt.Printf("Conclusion: %s\n", longResult.Output.Conclusion)
}
