// Example multi-provider demonstrates running one typed agent across providers.
// It runs offline by default with provider-tagged TestModels and can use live
// providers when GOLLEM_USE_LIVE_MODELS=1 is set with the corresponding API keys.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
)

// Answer is the structured output type.
type Answer struct {
	Text       string `json:"text" jsonschema:"description=The answer text"`
	Confidence string `json:"confidence" jsonschema:"description=Confidence level: high, medium, or low,enum=high|medium|low"`
}

func main() {
	target := "all"
	if len(os.Args) > 1 {
		target = strings.ToLower(os.Args[1])
	}

	providers := selectProviders(target)
	prompt := "In one sentence, what is the speed of light in vacuum?"

	for _, provider := range providers {
		model, live, note := selectModel(provider)

		agent := core.NewAgent[Answer](model,
			core.WithSystemPrompt[Answer](
				"You are a physics assistant. Be concise and include confidence.",
			),
		)

		result, err := agent.Run(context.Background(), prompt)
		if err != nil {
			log.Fatalf("%s run failed: %v", provider, err)
		}

		mode := "demo"
		if live {
			mode = "live"
		}
		fmt.Printf("=== Provider: %s (%s) ===\n", provider, mode)
		fmt.Printf("Model: %s\n", model.ModelName())
		fmt.Printf("Answer: %s\n", result.Output.Text)
		fmt.Printf("Confidence: %s\n", result.Output.Confidence)
		fmt.Printf("Tokens: %d input, %d output\n", result.Usage.InputTokens, result.Usage.OutputTokens)
		if note != "" {
			fmt.Printf("Note: %s\n", note)
		}
		fmt.Println()
	}
}

func selectProviders(target string) []string {
	switch target {
	case "all":
		return []string{"anthropic", "openai"}
	case "anthropic", "openai":
		return []string{target}
	default:
		log.Fatalf("Unknown provider target %q (use anthropic, openai, or all)", target)
		return nil
	}
}

func selectModel(provider string) (core.Model, bool, string) {
	useLive := os.Getenv("GOLLEM_USE_LIVE_MODELS") == "1"

	if useLive {
		switch provider {
		case "anthropic":
			if os.Getenv("ANTHROPIC_API_KEY") != "" {
				return anthropic.New(), true, ""
			}
			return demoModelFor(provider), false, "ANTHROPIC_API_KEY not set; using demo model"
		case "openai":
			if os.Getenv("OPENAI_API_KEY") != "" {
				return openai.New(), true, ""
			}
			return demoModelFor(provider), false, "OPENAI_API_KEY not set; using demo model"
		}
	}

	return demoModelFor(provider), false, "set GOLLEM_USE_LIVE_MODELS=1 to call live providers"
}

func demoModelFor(provider string) core.Model {
	var response string
	switch provider {
	case "anthropic":
		response = `{"text":"The speed of light in vacuum is 299,792,458 meters per second.","confidence":"high"}`
	case "openai":
		response = `{"text":"In vacuum, light travels at exactly 299,792,458 m/s.","confidence":"high"}`
	default:
		response = `{"text":"The speed of light is about 3.00×10^8 m/s in vacuum.","confidence":"medium"}`
	}

	model := core.NewTestModel(core.ToolCallResponse("final_result", response))
	model.SetName(provider + "-demo")
	return model
}
