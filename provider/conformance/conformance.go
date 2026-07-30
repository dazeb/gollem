// Package conformance provides provider-neutral checks for model-driver tests.
//
// Drivers supply their own deterministic protocol fixture and then pass the
// resulting core.Model to Verify. This keeps wire-format details with each
// adapter while making capability claims observable at Gollem's model boundary.
package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/modelutil"
)

// Claims are the capabilities covered by a deterministic conformance fixture.
// A driver must not advertise one of these capabilities in Slang unless it has
// a matching deterministic fixture and this verification passes.
type Claims struct {
	ToolCalls           bool
	Streaming           bool
	Usage               bool
	Cancellation        bool
	PartialStream       bool
	MalformedStream     bool
	DisconnectStream    bool
	Retryability        bool
	RequestTimeout      bool
	StreamTimeout       bool
	ReasoningVisibility bool
}

// Expectations declares the normalized outputs a deterministic fixture
// produces. ToolName is required when Claims.ToolCalls is true. StreamText is
// required when Claims.Streaming is true.
type Expectations struct {
	ResponseText      string
	ToolName          string
	StreamText        string
	PartialText       string
	DisconnectText    string
	RetryText         string
	StreamTimeoutText string
	ReasoningText     string
}

// Driver binds a provider model to the common capability claims and expected
// normalized results that its deterministic fixture produces.
type Driver struct {
	Name                string
	Model               core.Model
	Claims              Claims
	Expectations        Expectations
	CancellationReady   <-chan struct{}
	RequestTimeoutReady <-chan struct{}
	ReasoningModel      core.Model
}

// Verify exercises the claimed common model surface through core.Model. It is
// intentionally independent of a provider's HTTP or RPC wire format.
func Verify(ctx context.Context, driver Driver) error {
	if strings.TrimSpace(driver.Name) == "" {
		return errors.New("provider conformance: driver name is required")
	}
	if driver.Model == nil {
		return fmt.Errorf("provider conformance: %s model is required", driver.Name)
	}
	if driver.Claims.ToolCalls && strings.TrimSpace(driver.Expectations.ToolName) == "" {
		return fmt.Errorf("provider conformance: %s tool-capable fixture must expect a tool call", driver.Name)
	}
	if driver.Claims.Streaming && strings.TrimSpace(driver.Expectations.StreamText) == "" {
		return fmt.Errorf("provider conformance: %s streaming fixture must expect stream text", driver.Name)
	}
	if driver.Claims.Cancellation && driver.CancellationReady == nil {
		return fmt.Errorf("provider conformance: %s cancellation-capable fixture must signal request start", driver.Name)
	}
	if driver.Claims.PartialStream && strings.TrimSpace(driver.Expectations.PartialText) == "" {
		return fmt.Errorf("provider conformance: %s partial-stream fixture must expect partial text", driver.Name)
	}
	if driver.Claims.DisconnectStream && strings.TrimSpace(driver.Expectations.DisconnectText) == "" {
		return fmt.Errorf("provider conformance: %s disconnect-stream fixture must expect partial text", driver.Name)
	}
	if driver.Claims.Retryability && strings.TrimSpace(driver.Expectations.RetryText) == "" {
		return fmt.Errorf("provider conformance: %s retry fixture must expect retry text", driver.Name)
	}
	if driver.Claims.RequestTimeout && driver.RequestTimeoutReady == nil {
		return fmt.Errorf("provider conformance: %s timeout-capable fixture must signal request start", driver.Name)
	}
	if driver.Claims.StreamTimeout && strings.TrimSpace(driver.Expectations.StreamTimeoutText) == "" {
		return fmt.Errorf("provider conformance: %s stream-timeout fixture must expect partial text", driver.Name)
	}
	if driver.Claims.ReasoningVisibility && driver.ReasoningModel == nil {
		return fmt.Errorf("provider conformance: %s reasoning-capable fixture must supply a reasoning model", driver.Name)
	}
	if driver.Claims.ReasoningVisibility && strings.TrimSpace(driver.Expectations.ReasoningText) == "" {
		return fmt.Errorf("provider conformance: %s reasoning fixture must expect reasoning text", driver.Name)
	}

	params := &core.ModelRequestParameters{AllowTextOutput: true}
	if driver.Claims.ToolCalls {
		params.FunctionTools = []core.ToolDefinition{{
			Name:        "conformance_echo",
			Description: "Return the supplied value for deterministic driver testing.",
			ParametersSchema: core.Schema{
				"type": "object",
				"properties": map[string]any{
					"value": core.Schema{"type": "string"},
				},
				"required": []string{"value"},
			},
		}}
	}
	response, err := driver.Model.Request(ctx, conformanceMessages(), nil, params)
	if err != nil {
		return fmt.Errorf("provider conformance: %s request: %w", driver.Name, err)
	}
	if response == nil {
		return fmt.Errorf("provider conformance: %s request returned a nil response", driver.Name)
	}
	if err := verifyResponse(driver, response, false); err != nil {
		return err
	}
	if driver.Claims.Cancellation {
		if err := verifyCancellation(ctx, driver); err != nil {
			return err
		}
	}
	if driver.Claims.RequestTimeout {
		if err := verifyRequestTimeout(ctx, driver); err != nil {
			return err
		}
	}
	if driver.Claims.StreamTimeout {
		if err := verifyStreamTimeout(ctx, driver); err != nil {
			return err
		}
	}
	if driver.Claims.Retryability {
		if err := verifyRetryability(ctx, driver); err != nil {
			return err
		}
	}
	if driver.Claims.ReasoningVisibility {
		if err := verifyReasoningVisibility(ctx, driver); err != nil {
			return err
		}
	}

	if !driver.Claims.Streaming {
		return nil
	}
	stream, err := driver.Model.RequestStream(ctx, conformanceMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return fmt.Errorf("provider conformance: %s stream request: %w", driver.Name, err)
	}
	defer stream.Close()
	for {
		_, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("provider conformance: %s stream: %w", driver.Name, err)
		}
	}
	if err := verifyResponse(driver, stream.Response(), true); err != nil {
		return err
	}
	if driver.Claims.PartialStream {
		if err := verifyPartialStream(ctx, driver); err != nil {
			return err
		}
	}
	if driver.Claims.MalformedStream {
		if err := verifyMalformedStream(ctx, driver); err != nil {
			return err
		}
	}
	if driver.Claims.DisconnectStream {
		if err := verifyDisconnectStream(ctx, driver); err != nil {
			return err
		}
	}
	return nil
}

func verifyCancellation(ctx context.Context, driver Driver) error {
	requestContext, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := driver.Model.Request(requestContext, cancellationMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
		result <- err
	}()

	select {
	case <-driver.CancellationReady:
		cancel()
	case <-ctx.Done():
		return fmt.Errorf("provider conformance: %s cancellation request did not start: %w", driver.Name, ctx.Err())
	case <-time.After(time.Second):
		return fmt.Errorf("provider conformance: %s cancellation request did not start", driver.Name)
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			return fmt.Errorf("provider conformance: %s cancellation error, want context canceled: %w", driver.Name, err)
		}
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("provider conformance: %s cancellation request did not finish", driver.Name)
	}
}

func verifyRequestTimeout(ctx context.Context, driver Driver) error {
	requestContext, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := driver.Model.Request(requestContext, timeoutMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
		result <- err
	}()

	select {
	case <-driver.RequestTimeoutReady:
	case <-ctx.Done():
		return fmt.Errorf("provider conformance: %s timeout request did not start: %w", driver.Name, ctx.Err())
	case <-time.After(time.Second):
		return fmt.Errorf("provider conformance: %s timeout request did not start", driver.Name)
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("provider conformance: %s timeout error, want context deadline exceeded: %w", driver.Name, err)
		}
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("provider conformance: %s timeout request did not finish", driver.Name)
	}
}

func verifyStreamTimeout(ctx context.Context, driver Driver) error {
	requestContext, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	stream, err := driver.Model.RequestStream(requestContext, streamTimeoutMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return fmt.Errorf("provider conformance: %s stream-timeout request: %w", driver.Name, err)
	}
	defer stream.Close()
	for {
		_, err := stream.Next()
		if err == nil {
			continue
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("provider conformance: %s stream-timeout error, want context deadline exceeded: %w", driver.Name, err)
		}
		break
	}
	if got := stream.Response().TextContent(); got != driver.Expectations.StreamTimeoutText {
		return fmt.Errorf("provider conformance: %s stream-timeout text = %q, want %q", driver.Name, got, driver.Expectations.StreamTimeoutText)
	}
	return nil
}

func verifyPartialStream(ctx context.Context, driver Driver) error {
	stream, err := driver.Model.RequestStream(ctx, partialStreamMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return fmt.Errorf("provider conformance: %s partial stream request: %w", driver.Name, err)
	}
	defer stream.Close()
	for {
		_, err := stream.Next()
		if err == nil {
			continue
		}
		var incomplete *core.StreamIncompleteError
		if !errors.As(err, &incomplete) {
			return fmt.Errorf("provider conformance: %s partial stream error = %w, want StreamIncompleteError", driver.Name, err)
		}
		break
	}
	if got := stream.Response().TextContent(); got != driver.Expectations.PartialText {
		return fmt.Errorf("provider conformance: %s partial text = %q, want %q", driver.Name, got, driver.Expectations.PartialText)
	}
	return nil
}

func verifyMalformedStream(ctx context.Context, driver Driver) error {
	stream, err := driver.Model.RequestStream(ctx, malformedStreamMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return fmt.Errorf("provider conformance: %s malformed stream request: %w", driver.Name, err)
	}
	defer stream.Close()
	for {
		_, err := stream.Next()
		if err == nil {
			continue
		}
		var protocol *core.StreamProtocolError
		if !errors.As(err, &protocol) {
			return fmt.Errorf("provider conformance: %s malformed stream error = %w, want StreamProtocolError", driver.Name, err)
		}
		return nil
	}
}

func verifyDisconnectStream(ctx context.Context, driver Driver) error {
	stream, err := driver.Model.RequestStream(ctx, disconnectStreamMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return fmt.Errorf("provider conformance: %s disconnect stream request: %w", driver.Name, err)
	}
	defer stream.Close()
	for {
		_, err := stream.Next()
		if err == nil {
			continue
		}
		var transport *core.StreamTransportError
		if !errors.As(err, &transport) {
			return fmt.Errorf("provider conformance: %s disconnect stream error = %w, want StreamTransportError", driver.Name, err)
		}
		break
	}
	if got := stream.Response().TextContent(); got != driver.Expectations.DisconnectText {
		return fmt.Errorf("provider conformance: %s disconnect text = %q, want %q", driver.Name, got, driver.Expectations.DisconnectText)
	}
	return nil
}

func verifyRetryability(ctx context.Context, driver Driver) error {
	retryingModel := modelutil.NewRetryModel(driver.Model, modelutil.RetryConfig{
		MaxRetries:     1,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		BackoffFactor:  1,
		Jitter:         false,
		MinRemaining:   0,
	})
	response, err := retryingModel.Request(ctx, retryMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return fmt.Errorf("provider conformance: %s retry request: %w", driver.Name, err)
	}
	if response == nil {
		return fmt.Errorf("provider conformance: %s retry request returned a nil response", driver.Name)
	}
	if got := response.TextContent(); got != driver.Expectations.RetryText {
		return fmt.Errorf("provider conformance: %s retry text = %q, want %q", driver.Name, got, driver.Expectations.RetryText)
	}
	return nil
}

func verifyReasoningVisibility(ctx context.Context, driver Driver) error {
	stream, err := driver.ReasoningModel.RequestStream(ctx, reasoningMessages(), nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return fmt.Errorf("provider conformance: %s reasoning stream request: %w", driver.Name, err)
	}
	defer stream.Close()

	var (
		started bool
		deltas  strings.Builder
	)
	for {
		event, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("provider conformance: %s reasoning stream: %w", driver.Name, err)
		}
		switch value := event.(type) {
		case core.PartStartEvent:
			if part, ok := value.Part.(core.ThinkingPart); ok {
				started = true
				deltas.WriteString(part.Content)
			}
		case core.PartDeltaEvent:
			if delta, ok := value.Delta.(core.ThinkingPartDelta); ok {
				deltas.WriteString(delta.ContentDelta)
			}
		}
	}
	if !started {
		return fmt.Errorf("provider conformance: %s reasoning stream did not emit a ThinkingPart start", driver.Name)
	}
	if got := deltas.String(); got != driver.Expectations.ReasoningText {
		return fmt.Errorf("provider conformance: %s reasoning deltas = %q, want %q", driver.Name, got, driver.Expectations.ReasoningText)
	}
	for _, part := range stream.Response().Parts {
		if thinking, ok := part.(core.ThinkingPart); ok && thinking.Content == driver.Expectations.ReasoningText {
			return nil
		}
	}
	return fmt.Errorf("provider conformance: %s final response did not retain reasoning text", driver.Name)
}

func verifyResponse(driver Driver, response *core.ModelResponse, streaming bool) error {
	if response == nil {
		return fmt.Errorf("provider conformance: %s returned a nil response", driver.Name)
	}
	wantText := driver.Expectations.ResponseText
	if streaming {
		wantText = driver.Expectations.StreamText
	}
	if got := response.TextContent(); got != wantText {
		return fmt.Errorf("provider conformance: %s text = %q, want %q", driver.Name, got, wantText)
	}
	if driver.Claims.ToolCalls && !streaming && !hasToolCall(response.Parts, driver.Expectations.ToolName) {
		return fmt.Errorf("provider conformance: %s response did not contain tool %q", driver.Name, driver.Expectations.ToolName)
	}
	if driver.Claims.Usage && response.Usage.InputTokens+response.Usage.OutputTokens == 0 {
		return fmt.Errorf("provider conformance: %s response did not report usage", driver.Name)
	}
	return nil
}

func hasToolCall(parts []core.ModelResponsePart, name string) bool {
	for _, part := range parts {
		if call, ok := part.(core.ToolCallPart); ok && call.ToolName == name {
			return true
		}
	}
	return false
}

func conformanceMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "run conformance"}}},
	}
}

func cancellationMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "cancel conformance"}}},
	}
}

func partialStreamMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "partial stream conformance"}}},
	}
}

func malformedStreamMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "malformed stream conformance"}}},
	}
}

func disconnectStreamMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "disconnect stream conformance"}}},
	}
}

func retryMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "retry conformance"}}},
	}
}

func timeoutMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "timeout conformance"}}},
	}
}

func streamTimeoutMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "stream timeout conformance"}}},
	}
}

func reasoningMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "reasoning conformance"}}},
	}
}
