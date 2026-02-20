// Package eval provides a systematic evaluation framework for testing agent quality.
//
// It supports datasets of test cases, multiple evaluators (including LLM-as-judge),
// and aggregated reporting. Use it to measure and track agent performance across
// different scenarios.
//
// Usage:
//
//	dataset := eval.Dataset[string]{
//	    Name: "greeting-quality",
//	    Cases: []eval.Case[string]{
//	        {Name: "hello", Prompt: "Say hello", Expected: "Hello!"},
//	        {Name: "goodbye", Prompt: "Say goodbye", Expected: "Goodbye!"},
//	    },
//	}
//
//	runner := eval.NewRunner(agent, eval.Contains())
//	report, err := runner.Run(ctx, dataset)
package eval
