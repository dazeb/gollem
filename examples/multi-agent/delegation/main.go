// Example delegation demonstrates the AgentTool pattern where one agent
// delegates work to another agent by calling it as a tool. The outer
// "orchestrator" agent decides when to invoke the inner "specialist" agent,
// and the inner agent's output is returned as the tool result.
//
// This example uses TestModel so it compiles and runs without any API keys.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/core/orchestration"
)

// ResearchResult is the output type for the specialist research agent.
type ResearchResult struct {
	Topic       string   `json:"topic" jsonschema:"description=The research topic"`
	Summary     string   `json:"summary" jsonschema:"description=Summary of findings"`
	KeyFindings []string `json:"key_findings" jsonschema:"description=Key findings"`
	Sources     []string `json:"sources" jsonschema:"description=List of sources consulted"`
}

// RiskResult is the output type for the risk specialist agent.
type RiskResult struct {
	Level       string   `json:"level" jsonschema:"description=overall risk level,enum=low|medium|high"`
	Risks       []string `json:"risks" jsonschema:"description=Top risks"`
	Mitigations []string `json:"mitigations" jsonschema:"description=Recommended mitigations"`
}

// FinalReport is the output type for the orchestrator agent.
type FinalReport struct {
	Title            string   `json:"title" jsonschema:"description=Report title"`
	ExecutiveSummary string   `json:"executive_summary" jsonschema:"description=Executive summary"`
	Sections         []string `json:"sections" jsonschema:"description=Major report sections"`
	Risks            []string `json:"risks" jsonschema:"description=Key risks to track"`
	Recommendation   string   `json:"recommendation" jsonschema:"description=Final recommendation"`
}

func main() {
	// --- Inner Agent: Research Specialist ---
	// This agent handles research tasks delegated by the orchestrator.
	researchModel := core.NewTestModel(
		core.ToolCallResponse("final_result", `{
			"topic": "Go generics",
			"summary": "Go 1.18 introduced type parameters enabling generic programming for reusable libraries.",
			"key_findings": [
				"Type constraints are interfaces, which keeps the model consistent with core Go idioms.",
				"Type inference handles many call sites, reducing boilerplate.",
				"Standard library packages like slices and maps improved dramatically."
			],
			"sources": ["go.dev/doc/tutorial/generics", "go.dev/ref/spec#Type_parameters"]
		}`),
	)

	researchAgent := core.NewAgent[ResearchResult](researchModel,
		core.WithSystemPrompt[ResearchResult]("You are a research specialist. Investigate topics thoroughly and return structured findings."),
	)

	// --- Inner Agent: Risk Specialist ---
	riskModel := core.NewTestModel(
		core.ToolCallResponse("final_result", `{
			"level":"medium",
			"risks":[
				"Over-generalization can reduce readability in business code",
				"Teams may misuse constraints without clear style guidance"
			],
			"mitigations":[
				"Define linting/style rules for generics",
				"Adopt generics first in shared libraries and utilities"
			]
		}`),
	)
	riskAgent := core.NewAgent[RiskResult](riskModel,
		core.WithSystemPrompt[RiskResult]("You are a software architecture risk analyst. Return concrete risks and mitigations."),
	)

	// --- Outer Agent: Orchestrator ---
	// This agent decides what to research and composes the final report.
	orchestratorModel := core.NewTestModel(
		// First response: orchestrator calls the research agent tool.
		core.ToolCallResponse("research", `{"prompt":"Research Go generics: history, features, and best practices"}`),
		// Second response: orchestrator calls risk specialist.
		core.ToolCallResponse("assess_risks", `{"prompt":"Identify adoption risks for Go generics in production systems"}`),
		// Third response: orchestrator produces the final report.
		core.ToolCallResponse("final_result", `{
			"title": "Go Generics: Practical Adoption Brief",
			"executive_summary": "Generics are now mature enough for production when applied to reusable libraries with style guardrails.",
			"sections": [
				"Feature overview and language evolution",
				"High-impact use cases in modern Go codebases",
				"Adoption risks and mitigation strategy"
			],
			"risks": [
				"Readability regressions from over-abstraction",
				"Inconsistent constraint patterns across teams"
			],
			"recommendation": "Adopt generics in shared packages first, backed by clear linting and review standards."
		}`),
	)

	// Wrap the research agent as a tool using AgentTool.
	researchTool := orchestration.AgentTool("research", "Delegate research tasks to a specialist agent", researchAgent)
	riskTool := orchestration.AgentTool("assess_risks", "Delegate risk analysis to a specialist agent", riskAgent)

	orchestratorAgent := core.NewAgent[FinalReport](orchestratorModel,
		core.WithSystemPrompt[FinalReport]("You are a principal analyst. Gather specialist input, then produce a concise executive report."),
		core.WithTools[FinalReport](researchTool, riskTool),
	)

	// Run the orchestrator agent.
	result, err := orchestratorAgent.Run(context.Background(), "Write a report about Go generics")
	if err != nil {
		log.Fatal(err)
	}

	// Print the final report.
	fmt.Println("=== Final Report ===")
	fmt.Printf("Title: %s\n\n", result.Output.Title)
	fmt.Printf("Executive summary:\n%s\n\n", result.Output.ExecutiveSummary)
	fmt.Printf("Sections:\n")
	for _, section := range result.Output.Sections {
		fmt.Printf("  - %s\n", section)
	}
	fmt.Printf("\nRisks:\n")
	for _, risk := range result.Output.Risks {
		fmt.Printf("  - %s\n", risk)
	}
	fmt.Printf("\nRecommendation:\n%s\n\n", result.Output.Recommendation)
	fmt.Printf("Orchestrator usage: %d requests, %d tool calls\n",
		result.Usage.Requests, result.Usage.ToolCalls)
}
