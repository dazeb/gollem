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
)

// Claims are the capabilities covered by a deterministic conformance fixture.
// A driver must not advertise one of these capabilities in Slang unless it has
// a matching deterministic fixture and this verification passes.
type Claims struct {
	ToolCalls       bool
	Streaming       bool
	Usage           bool
	Cancellation    bool
	PartialStream   bool
	MalformedStream bool
}

// Expectations declares the normalized outputs a deterministic fixture
// produces. ToolName is required when Claims.ToolCalls is true. StreamText is
// required when Claims.Streaming is true.
type Expectations struct {
	ResponseText string
	ToolName     string
	StreamText   string
	PartialText  string
}

// Driver binds a provider model to the common capability claims and expected
// normalized results that its deterministic fixture produces.
type Driver struct {
	Name              string
	Model             core.Model
	Claims            Claims
	Expectations      Expectations
	CancellationReady <-chan struct{}
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
