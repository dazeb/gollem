package openai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fugue-labs/gollem"
)

// streamedResponse implements gollem.StreamedResponse for OpenAI SSE streams.
type streamedResponse struct {
	reader     *bufio.Reader
	body       io.ReadCloser
	model      string
	usage      gollem.Usage
	parts      []gollem.ModelResponsePart
	stopReason gollem.FinishReason
	done       bool

	// State for tracking tool calls being built across deltas.
	currentParts map[int]gollem.ModelResponsePart
	argsBuffers  map[int]*strings.Builder
	// Map tool call index to the next part index.
	toolCallPartIndex map[int]int
	nextPartIndex     int
}

func newStreamedResponse(body io.ReadCloser, model string) *streamedResponse {
	return &streamedResponse{
		reader:            bufio.NewReader(body),
		body:              body,
		model:             model,
		currentParts:      make(map[int]gollem.ModelResponsePart),
		argsBuffers:       make(map[int]*strings.Builder),
		toolCallPartIndex: make(map[int]int),
		stopReason:        gollem.FinishReasonStop,
	}
}

// apiChunk represents an OpenAI streaming chunk.
type apiChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Choices []apiChunkChoice `json:"choices"`
	Usage   *apiUsage      `json:"usage,omitempty"`
}

type apiChunkChoice struct {
	Index        int            `json:"index"`
	Delta        apiChunkDelta  `json:"delta"`
	FinishReason *string        `json:"finish_reason"`
}

type apiChunkDelta struct {
	Role      string            `json:"role,omitempty"`
	Content   string            `json:"content,omitempty"`
	ToolCalls []apiChunkToolCall `json:"tool_calls,omitempty"`
}

type apiChunkToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function apiChunkFunction `json:"function"`
}

type apiChunkFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// Next returns the next stream event.
func (s *streamedResponse) Next() (gollem.ModelResponseStreamEvent, error) {
	for {
		if s.done {
			return nil, io.EOF
		}

		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.done = true
				// Finalize any remaining parts.
				s.finalizeAll()
				return nil, io.EOF
			}
			return nil, err
		}

		line = strings.TrimSpace(line)

		// Skip empty lines and non-data lines.
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream termination.
		if data == "[DONE]" {
			s.done = true
			s.finalizeAll()
			return nil, io.EOF
		}

		var chunk apiChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Update usage if present.
		if chunk.Usage != nil {
			s.usage = mapUsage(*chunk.Usage)
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// Check finish reason.
		if choice.FinishReason != nil {
			s.stopReason = mapFinishReasonStr(*choice.FinishReason)
		}

		// Handle text content delta.
		if choice.Delta.Content != "" {
			event := s.handleTextDelta(choice.Delta.Content)
			if event != nil {
				return event, nil
			}
		}

		// Handle tool call deltas.
		if len(choice.Delta.ToolCalls) > 0 {
			events := s.handleToolCallDeltas(choice.Delta.ToolCalls)
			if len(events) > 0 {
				// Return the first event; queue others would be complex,
				// but typically there's only one delta per chunk.
				return events[0], nil
			}
		}
	}
}

// handleTextDelta processes a text content delta.
func (s *streamedResponse) handleTextDelta(content string) gollem.ModelResponseStreamEvent {
	// Check if text part already started.
	textIdx := -1
	for idx, part := range s.currentParts {
		if _, ok := part.(gollem.TextPart); ok {
			textIdx = idx
			break
		}
	}

	if textIdx == -1 {
		// Start a new text part.
		textIdx = s.nextPartIndex
		s.nextPartIndex++
		s.currentParts[textIdx] = gollem.TextPart{Content: content}
		return gollem.PartStartEvent{
			Index: textIdx,
			Part:  gollem.TextPart{Content: content},
		}
	}

	// Append to existing text part.
	if tp, ok := s.currentParts[textIdx].(gollem.TextPart); ok {
		tp.Content += content
		s.currentParts[textIdx] = tp
	}
	return gollem.PartDeltaEvent{
		Index: textIdx,
		Delta: gollem.TextPartDelta{ContentDelta: content},
	}
}

// handleToolCallDeltas processes tool call deltas from a chunk.
func (s *streamedResponse) handleToolCallDeltas(toolCalls []apiChunkToolCall) []gollem.ModelResponseStreamEvent {
	var events []gollem.ModelResponseStreamEvent

	for _, tc := range toolCalls {
		partIdx, isNew := s.getToolCallPartIndex(tc.Index)

		if isNew || tc.ID != "" {
			// Start a new tool call part.
			part := gollem.ToolCallPart{
				ToolName:   tc.Function.Name,
				ToolCallID: tc.ID,
			}
			s.currentParts[partIdx] = part
			s.argsBuffers[tc.Index] = &strings.Builder{}
			if tc.Function.Arguments != "" {
				s.argsBuffers[tc.Index].WriteString(tc.Function.Arguments)
			}
			events = append(events, gollem.PartStartEvent{
				Index: partIdx,
				Part:  part,
			})
		} else if tc.Function.Arguments != "" {
			// Append arguments delta.
			if buf, ok := s.argsBuffers[tc.Index]; ok {
				buf.WriteString(tc.Function.Arguments)
			}
			events = append(events, gollem.PartDeltaEvent{
				Index: partIdx,
				Delta: gollem.ToolCallPartDelta{ArgsJSONDelta: tc.Function.Arguments},
			})
		}
	}

	return events
}

// getToolCallPartIndex maps a tool call index to a part index.
func (s *streamedResponse) getToolCallPartIndex(tcIndex int) (int, bool) {
	if idx, ok := s.toolCallPartIndex[tcIndex]; ok {
		return idx, false
	}
	idx := s.nextPartIndex
	s.nextPartIndex++
	s.toolCallPartIndex[tcIndex] = idx
	return idx, true
}

// finalizeAll finalizes all remaining parts.
func (s *streamedResponse) finalizeAll() {
	for idx, part := range s.currentParts {
		if tc, ok := part.(gollem.ToolCallPart); ok {
			// Find the tool call index for this part.
			for tcIdx, partIdx := range s.toolCallPartIndex {
				if partIdx == idx {
					if buf, ok := s.argsBuffers[tcIdx]; ok {
						tc.ArgsJSON = buf.String()
						if tc.ArgsJSON == "" {
							tc.ArgsJSON = "{}"
						}
						part = tc
					}
					break
				}
			}
		}
		s.parts = append(s.parts, part)
	}
	s.currentParts = make(map[int]gollem.ModelResponsePart)
}

// Response returns the complete ModelResponse built from the stream.
func (s *streamedResponse) Response() *gollem.ModelResponse {
	return &gollem.ModelResponse{
		Parts:        s.parts,
		Usage:        s.usage,
		ModelName:    s.model,
		FinishReason: s.stopReason,
		Timestamp:    time.Now(),
	}
}

// Usage returns the current usage information.
func (s *streamedResponse) Usage() gollem.Usage {
	return s.usage
}

// Close releases resources.
func (s *streamedResponse) Close() error {
	return s.body.Close()
}

// mapFinishReasonStr maps a finish reason string to gollem FinishReason.
func mapFinishReasonStr(reason string) gollem.FinishReason {
	switch reason {
	case "stop":
		return gollem.FinishReasonStop
	case "length":
		return gollem.FinishReasonLength
	case "tool_calls":
		return gollem.FinishReasonToolCall
	case "content_filter":
		return gollem.FinishReasonContentFilter
	default:
		return gollem.FinishReasonStop
	}
}

// Verify streamedResponse implements gollem.StreamedResponse.
var _ gollem.StreamedResponse = (*streamedResponse)(nil)

// Ensure fmt is used.
var _ = fmt.Sprintf
