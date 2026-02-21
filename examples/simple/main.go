// Example simple demonstrates typed structured output with Agent[CityInfo].
// It runs offline by default with TestModel and can use Vertex AI when
// GOLLEM_USE_LIVE_MODELS=1 is set.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/vertexai"
)

// CityInfo is the structured output type the agent will produce.
type CityInfo struct {
	Name              string   `json:"name" jsonschema:"description=City name"`
	Country           string   `json:"country" jsonschema:"description=Country the city is in"`
	Population        int      `json:"population" jsonschema:"description=Approximate population"`
	FavoriteFood      string   `json:"favorite_food" jsonschema:"description=A popular local dish"`
	LeastFavoriteFood string   `json:"least_favorite_food" jsonschema:"description=A less popular local dish"`
	Latitude          float64  `json:"latitude" jsonschema:"description=Latitude coordinate"`
	Longitude         float64  `json:"longitude" jsonschema:"description=Longitude coordinate"`
	Landmarks         []string `json:"landmarks" jsonschema:"description=Famous landmarks in the city"`
}

func main() {
	model, live := selectModel()

	agent := core.NewAgent[CityInfo](model,
		core.WithSystemPrompt[CityInfo](
			"You are a geography analyst. Return compact, factual city snapshots with useful highlights.",
		),
	)

	prompt := "Give me a compact briefing for Tokyo: location, population, iconic landmarks, and food culture."
	result, err := agent.Run(context.Background(), prompt)
	if err != nil {
		log.Fatal(err)
	}

	printBriefing(result.Output)
	fmt.Printf("\nModel: %s\n", model.ModelName())
	fmt.Printf("Usage: %d input tokens, %d output tokens\n",
		result.Usage.InputTokens, result.Usage.OutputTokens)
	if !live {
		fmt.Println("(offline demo mode: set GOLLEM_USE_LIVE_MODELS=1 to call Vertex AI)")
	}
}

func selectModel() (core.Model, bool) {
	if os.Getenv("GOLLEM_USE_LIVE_MODELS") == "1" {
		return vertexai.New(
			vertexai.WithModel("gemini-3-flash-preview"),
			vertexai.WithLocation("global"),
		), true
	}

	return core.NewTestModel(
		core.ToolCallResponse("final_result", `{
			"name":"Tokyo",
			"country":"Japan",
			"population":13960000,
			"favorite_food":"Sushi",
			"least_favorite_food":"Natto (fermented soybeans)",
			"latitude":35.6762,
			"longitude":139.6503,
			"landmarks":["Shibuya Crossing","Tokyo Skytree","Senso-ji Temple"]
		}`),
	), false
}

func printBriefing(city CityInfo) {
	fmt.Println("=== City Briefing ===")
	fmt.Printf("City: %s, %s\n", city.Name, city.Country)
	fmt.Printf("Population: %d\n", city.Population)
	fmt.Printf("Coordinates: %.4f, %.4f (%s hemisphere)\n",
		city.Latitude, city.Longitude, hemisphere(city.Latitude))
	fmt.Printf("Favorite food: %s\n", city.FavoriteFood)
	fmt.Printf("Least favorite food: %s\n", city.LeastFavoriteFood)
	fmt.Printf("Landmarks: %s\n", strings.Join(city.Landmarks, ", "))
}

func hemisphere(lat float64) string {
	if lat < 0 {
		return "southern"
	}
	return "northern"
}
