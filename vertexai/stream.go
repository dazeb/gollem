package vertexai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fugue-labs/gollem"
)

// streamedResponse implements gollem.StreamedResponse for Vertex AI Gemini SSE streams.
type streamedResponse struct {
	reader     *bufio.Reader
	body       io.ReadCloser
	model      string
	usage      gollem.Usage
	parts      []gollem.ModelResponsePart
	stopReason gollem.FinishReason
	done       bool

	// State for tracking current parts being built.
	currentParts  map[int]gollem.ModelResponsePart
	nextPartIndex int
}

func newStreamedResponse(body io.ReadCloser, model string) *streamedResponse {
	return &streamedResponse{
		reader:       bufio.NewReader(body),
		body:         body,
		model:        model,
		currentParts: make(map[int]gollem.ModelResponsePart),
		stopReason:   gollem.FinishReasonStop,
	}
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

		var resp geminiResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

		// Update usage.
		if resp.UsageMetadata.PromptTokenCount > 0 || resp.UsageMetadata.CandidatesTokenCount > 0 {
			s.usage = mapUsage(resp.UsageMetadata)
		}

		if len(resp.Candidates) == 0 {
			continue
		}

		candidate := resp.Candidates[0]

		// Check finish reason.
		if candidate.FinishReason != "" {
			s.stopReason = mapFinishReasonStr(candidate.FinishReason)
		}

		// Process parts in this chunk.
		for _, p := range candidate.Content.Parts {
			if p.Text != "" {
				event := s.handleTextDelta(p.Text)
				if event != nil {
					return event, nil
				}
			}
			if p.FunctionCall != nil {
				event := s.handleFunctionCall(p.FunctionCall)
				if event != nil {
					return event, nil
				}
			}
		}
	}
}

// handleTextDelta processes a text delta from a streaming chunk.
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
		textIdx = s.nextPartIndex
		s.nextPartIndex++
		s.currentParts[textIdx] = gollem.TextPart{Content: content}
		return gollem.PartStartEvent{
			Index: textIdx,
			Part:  gollem.TextPart{Content: content},
		}
	}

	if tp, ok := s.currentParts[textIdx].(gollem.TextPart); ok {
		tp.Content += content
		s.currentParts[textIdx] = tp
	}
	return gollem.PartDeltaEvent{
		Index: textIdx,
		Delta: gollem.TextPartDelta{ContentDelta: content},
	}
}

// handleFunctionCall processes a function call from a streaming chunk.
func (s *streamedResponse) handleFunctionCall(fc *geminiFunctionCall) gollem.ModelResponseStreamEvent {
	idx := s.nextPartIndex
	s.nextPartIndex++

	argsJSON := "{}"
	if fc.Args != nil {
		b, _ := json.Marshal(fc.Args)
		argsJSON = string(b)
	}

	part := gollem.ToolCallPart{
		ToolName:   fc.Name,
		ArgsJSON:   argsJSON,
		ToolCallID: fc.Name,
	}
	s.currentParts[idx] = part

	return gollem.PartStartEvent{
		Index: idx,
		Part:  part,
	}
}

// finalizeAll moves current parts into the finalized parts list.
func (s *streamedResponse) finalizeAll() {
	for _, part := range s.currentParts {
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

// mapFinishReasonStr maps a Gemini finish reason string to gollem FinishReason.
func mapFinishReasonStr(reason string) gollem.FinishReason {
	switch reason {
	case "STOP":
		return gollem.FinishReasonStop
	case "MAX_TOKENS":
		return gollem.FinishReasonLength
	case "SAFETY", "RECITATION":
		return gollem.FinishReasonContentFilter
	default:
		return gollem.FinishReasonStop
	}
}

// Verify streamedResponse implements gollem.StreamedResponse.
var _ gollem.StreamedResponse = (*streamedResponse)(nil)

// Ensure fmt is used.
var _ = fmt.Sprintf
