// Example tools demonstrates multiple FuncTool calls in one run.
// It runs offline by default with TestModel and can use Anthropic when
// GOLLEM_USE_LIVE_MODELS=1 is set.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/anthropic"
)

// CityParams defines a tool input with a city name.
type CityParams struct {
	City string `json:"city" jsonschema:"description=The city to query"`
}

// WeatherPlan is the structured output type.
type WeatherPlan struct {
	City        string   `json:"city"`
	Temperature int      `json:"temperature"`
	Conditions  string   `json:"conditions"`
	AirQuality  string   `json:"air_quality"`
	Highlights  []string `json:"highlights"`
	Plan        string   `json:"plan"`
}

func main() {
	model, live := selectModel()

	weatherTool := core.FuncTool[CityParams](
		"get_weather",
		"Get weather snapshot for a city",
		func(_ context.Context, params CityParams) (string, error) {
			return params.City + " weather: 64°F, breezy, sunny intervals", nil
		},
	)

	airQualityTool := core.FuncTool[CityParams](
		"get_air_quality",
		"Get air quality snapshot for a city",
		func(_ context.Context, params CityParams) (string, error) {
			return params.City + " AQI: 42 (Good)", nil
		},
	)

	agent := core.NewAgent[WeatherPlan](model,
		core.WithSystemPrompt[WeatherPlan](
			"You are a city concierge. Use tools, then produce a practical outdoor plan with highlights.",
		),
		core.WithTools[WeatherPlan](weatherTool, airQualityTool),
	)

	result, err := agent.Run(context.Background(), "Plan a fun afternoon in San Francisco based on current conditions.")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== City Outdoor Plan ===")
	fmt.Printf("City: %s\n", result.Output.City)
	fmt.Printf("Weather: %d°F, %s\n", result.Output.Temperature, result.Output.Conditions)
	fmt.Printf("Air quality: %s\n", result.Output.AirQuality)
	fmt.Printf("Highlights: %v\n", result.Output.Highlights)
	fmt.Printf("Plan: %s\n", result.Output.Plan)
	fmt.Printf("\nRun stats: %d requests, %d tool calls\n",
		result.Usage.Requests, result.Usage.ToolCalls)
	if !live {
		fmt.Println("(offline demo mode: set GOLLEM_USE_LIVE_MODELS=1 to call Anthropic)")
	}
}

func selectModel() (core.Model, bool) {
	if os.Getenv("GOLLEM_USE_LIVE_MODELS") == "1" {
		return anthropic.New(), true
	}

	return core.NewTestModel(
		core.ToolCallResponse("get_weather", `{"city":"San Francisco"}`),
		core.ToolCallResponse("get_air_quality", `{"city":"San Francisco"}`),
		core.ToolCallResponse("final_result", `{
			"city":"San Francisco",
			"temperature":64,
			"conditions":"breezy with sunny intervals",
			"air_quality":"AQI 42 (Good)",
			"highlights":["Crissy Field walk","Ferry Building snacks","Sunset at Lands End"],
			"plan":"Start with a waterfront walk, grab lunch at the Ferry Building, and finish with sunset views."
		}`),
	), false
}
