// Package chi provides an HTTP handler adapter that wraps a gollem agent
// for use with go-chi/chi (or any net/http-compatible router).
package chi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fugue-labs/gollem/core"
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

// Handler returns an http.HandlerFunc that runs the agent with the prompt
// from the request body. When stream is true it returns Server-Sent Events.
func Handler(runner AgentRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Prompt == "" {
			http.Error(w, "prompt is required", http.StatusBadRequest)
			return
		}

		if req.Stream {
			handleStream(w, r, runner, req.Prompt)
			return
		}

		result, err := runner.Run(r.Context(), req.Prompt)
		if err != nil {
			http.Error(w, "agent error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		resp := Response{
			Response: result.Output,
			Usage:    &result.Usage.Usage,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func handleStream(w http.ResponseWriter, r *http.Request, runner AgentRunner, prompt string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	stream, err := runner.RunStream(r.Context(), prompt)
	if err != nil {
		http.Error(w, "agent stream error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for text, err := range stream.StreamText(true) {
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			flusher.Flush()
			return
		}
		data, _ := json.Marshal(map[string]string{"text": text})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Send final usage event.
	resp := stream.Response()
	if resp != nil {
		data, _ := json.Marshal(map[string]any{"usage": resp.Usage})
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
		flusher.Flush()
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
