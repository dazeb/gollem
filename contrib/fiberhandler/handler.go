// Package fiberhandler provides an HTTP handler adapter that wraps a gollem agent
// for use with the Fiber web framework.
package fiberhandler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"

	"github.com/fugue-labs/gollem/core"
	"github.com/gofiber/fiber/v2"
)

// AgentRunner abstracts running a gollem agent so the handler does not depend
// on the generic Agent[T] type parameter.
type AgentRunner interface {
	Run(ctx context.Context, prompt string) (*core.RunResult[string], error)
	RunStream(ctx context.Context, prompt string) (*core.StreamResult[string], error)
}

// Request is the JSON request body accepted by the handler.
type Request struct {
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream,omitempty"`
}

// Response is the JSON response body returned by the handler.
type Response struct {
	Response string      `json:"response"`
	Usage    *core.Usage `json:"usage,omitempty"`
}

// Handler returns a fiber.Handler that runs the agent with the prompt
// from the request body. When stream is true it returns Server-Sent Events.
func Handler(runner AgentRunner) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req Request
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body: " + err.Error()})
		}
		if req.Prompt == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "prompt is required"})
		}

		if req.Stream {
			return handleStream(c, runner, req.Prompt)
		}

		result, err := runner.Run(c.UserContext(), req.Prompt)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "agent error: " + err.Error()})
		}

		return c.JSON(Response{
			Response: result.Output,
			Usage:    &result.Usage.Usage,
		})
	}
}

func handleStream(c *fiber.Ctx, runner AgentRunner, prompt string) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		stream, err := runner.RunStream(c.UserContext(), prompt)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			_ = w.Flush()
			return
		}
		defer stream.Close()

		for text, err := range stream.StreamText(true) {
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
				_ = w.Flush()
				return
			}
			data, _ := json.Marshal(map[string]string{"text": text})
			fmt.Fprintf(w, "data: %s\n\n", data)
			_ = w.Flush()
		}

		resp := stream.Response()
		if resp != nil {
			data, _ := json.Marshal(map[string]any{"usage": resp.Usage})
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
			_ = w.Flush()
		}
	})

	return nil
}

// AgentWrapper wraps a core.Agent[string] to satisfy the AgentRunner interface.
type AgentWrapper struct {
	Agent *core.Agent[string]
}

func (w *AgentWrapper) Run(ctx context.Context, prompt string) (*core.RunResult[string], error) {
	return w.Agent.Run(ctx, prompt)
}

func (w *AgentWrapper) RunStream(ctx context.Context, prompt string) (*core.StreamResult[string], error) {
	return w.Agent.RunStream(ctx, prompt)
}
