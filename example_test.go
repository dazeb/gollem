package gollem_test

import (
	"context"
	"fmt"

	"github.com/trevorprater/gollem"
)

func ExampleNewAgent() {
	model := gollem.NewTestModel(gollem.TextResponse("Hello!"))
	agent := gollem.NewAgent[string](model,
		gollem.WithSystemPrompt[string]("You are helpful."),
	)

	result, err := agent.Run(context.Background(), "Hi")
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Output)
	// Output: Hello!
}

func ExampleFuncTool() {
	type WeatherParams struct {
		City string `json:"city" description:"City name"`
	}

	tool := gollem.FuncTool[WeatherParams]("get_weather", "Get weather for a city",
		func(_ context.Context, params WeatherParams) (string, error) {
			return "Sunny in " + params.City, nil
		},
	)

	fmt.Println(tool.Definition.Name)
	fmt.Println(tool.Definition.Description)
	// Output:
	// get_weather
	// Get weather for a city
}

func ExampleAgent_Run() {
	type Result struct {
		Answer string `json:"answer"`
	}

	model := gollem.NewTestModel(
		gollem.ToolCallResponse("final_result", `{"answer":"42"}`),
	)
	agent := gollem.NewAgent[Result](model)

	result, err := agent.Run(context.Background(), "What is the answer?")
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Output.Answer)
	// Output: 42
}

func ExampleAgent_RunStream() {
	model := gollem.NewTestModel(gollem.TextResponse("streaming response"))
	agent := gollem.NewAgent[string](model)

	stream, err := agent.RunStream(context.Background(), "Hello")
	if err != nil {
		panic(err)
	}
	defer stream.Close()

	resp, err := stream.GetOutput()
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.TextContent())
	// Output: streaming response
}
