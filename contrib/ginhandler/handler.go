// Package ginhandler provides an HTTP handler adapter that wraps a gollem agent
// for use with the gin web framework.
package ginhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fugue-labs/gollem/core"
	"github.com/gin-gonic/gin"
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

// Handler returns a gin.HandlerFunc that runs the agent with the prompt
// from the request body. When stream is true it returns Server-Sent Events.
func Handler(runner AgentRunner) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}
		if req.Prompt == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
			return
		}

		if req.Stream {
			handleStream(c, runner, req.Prompt)
			return
		}

		result, err := runner.Run(c.Request.Context(), req.Prompt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "agent error: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, Response{
			Response: result.Output,
			Usage:    &result.Usage.Usage,
		})
	}
}

func handleStream(c *gin.Context, runner AgentRunner, prompt string) {
	stream, err := runner.RunStream(c.Request.Context(), prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent stream error: " + err.Error()})
		return
	}
	defer stream.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	w := c.Writer

	for text, err := range stream.StreamText(true) {
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			w.Flush()
			return
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
