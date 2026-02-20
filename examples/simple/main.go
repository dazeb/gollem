// Example simple demonstrates basic Agent[CityInfo] usage with the Anthropic provider,
// showing structured output extraction from an LLM.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/fugue-labs/gollem"
	"github.com/fugue-labs/gollem/anthropic"
)

// CityInfo is the structured output type the agent will produce.
type CityInfo struct {
	Name       string  `json:"name" jsonschema:"description=City name"`
	Country    string  `json:"country" jsonschema:"description=Country the city is in"`
	Population int     `json:"population" jsonschema:"description=Approximate population"`
	Latitude   float64 `json:"latitude" jsonschema:"description=Latitude coordinate"`
	Longitude  float64 `json:"longitude" jsonschema:"description=Longitude coordinate"`
}

func main() {
	// Create a provider (reads ANTHROPIC_API_KEY from environment).
	model := anthropic.New()

	// Create an agent that returns structured CityInfo.
	agent := gollem.NewAgent[CityInfo](model,
		gollem.WithSystemPrompt[CityInfo]("You are a geography expert. Answer questions about cities with accurate data."),
	)

	// Run the agent with a prompt.
	result, err := agent.Run(context.Background(), "Tell me about Tokyo")
	if err != nil {
		log.Fatal(err)
	}

	// Access the structured output.
	fmt.Printf("City: %s\n", result.Output.Name)
	fmt.Printf("Country: %s\n", result.Output.Country)
	fmt.Printf("Population: %d\n", result.Output.Population)
	fmt.Printf("Location: %.4f, %.4f\n", result.Output.Latitude, result.Output.Longitude)
	fmt.Printf("\nToken usage: %d input, %d output\n",
		result.Usage.InputTokens, result.Usage.OutputTokens)
}
