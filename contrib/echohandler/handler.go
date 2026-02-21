// Package echohandler provides an HTTP handler adapter that wraps a gollem agent
// for use with the Echo web framework.
package echohandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fugue-labs/gollem/core"
	"github.com/labstack/echo/v4"
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

// Handler returns an echo.HandlerFunc that runs the agent with the prompt
// from the request body. When stream is true it returns Server-Sent Events.
func Handler(runner AgentRunner) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req Request
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		}
		if req.Prompt == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		}

		if req.Stream {
			return handleStream(c, runner, req.Prompt)
		}

		result, err := runner.Run(c.Request().Context(), req.Prompt)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "agent error: " + err.Error()})
		}

		return c.JSON(http.StatusOK, Response{
			Response: result.Output,
			Usage:    &result.Usage.Usage,
		})
	}
}

func handleStream(c echo.Context, runner AgentRunner, prompt string) error {
	stream, err := runner.RunStream(c.Request().Context(), prompt)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "agent stream error: " + err.Error()})
	}
	defer stream.Close()

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for text, err := range stream.StreamText(true) {
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			w.Flush()
			return nil
		}
		data, _ := json.Marshal(map[string]string{"text": text})
		fmt.Fprintf(w, "data: %s\n\n", data)
		w.Flush()
	}

	resp := stream.Response()
	if resp != nil {
		data, _ := json.Marshal(map[string]any{"usage": resp.Usage})
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
		w.Flush()
	}

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
