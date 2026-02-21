// Example streaming demonstrates real-time token streaming.
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

func main() {
	model, live := selectModel()

	agent := core.NewAgent[string](model,
		core.WithSystemPrompt[string]("You are a cinematic storyteller. Keep scenes vivid and concise."),
	)

	stream, err := agent.RunStream(context.Background(), "Write a tiny story about a robot learning to paint dawn skies.")
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	fmt.Println("=== Streaming Output (delta mode) ===")
	chunks := 0
	for text, err := range stream.StreamText(true) {
		if err != nil {
			log.Fatal(err)
		}
		chunks++
		fmt.Print(text)
	}
	fmt.Println("\n=== End Stream ===")

	resp, err := stream.GetOutput()
	if err != nil {
		log.Fatal(err)
	}

	finalText := resp.TextContent()
	fmt.Printf("\nChunks streamed: %d\n", chunks)
	fmt.Printf("Final length: %d chars\n", len(finalText))
	fmt.Printf("Model: %s\n", resp.ModelName)
	fmt.Printf("Tokens: %d input, %d output\n",
		resp.Usage.InputTokens, resp.Usage.OutputTokens)
	if !live {
		fmt.Println("(offline demo mode: set GOLLEM_USE_LIVE_MODELS=1 to call Anthropic)")
	}
}

func selectModel() (core.Model, bool) {
	if os.Getenv("GOLLEM_USE_LIVE_MODELS") == "1" {
		return anthropic.New(), true
	}

	return core.NewTestModel(core.TextResponse(
		"At 5:03 a.m., Unit-9 mixed cobalt with apricot.\n" +
			"Its first sunrise looked clumsy, then brave.\n" +
			"By noon, the studio windows were glowing back.",
	)), false
}
