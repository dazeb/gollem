package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type ToolRequestUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type ToolRequestUserInputQuestion struct {
	ID       string                       `json:"id"`
	Header   string                       `json:"header"`
	Question string                       `json:"question"`
	IsOther  bool                         `json:"isOther"`
	IsSecret bool                         `json:"isSecret"`
	Options  []ToolRequestUserInputOption `json:"options"`
}

// ToolRequestUserInputParams is the public structured request. The trailing
// fields preserve Gollem's v1 single-prompt request for existing clients.
type ToolRequestUserInputParams struct {
	ThreadID         string                         `json:"threadId"`
	TurnID           string                         `json:"turnId"`
	ItemID           string                         `json:"itemId"`
	Questions        []ToolRequestUserInputQuestion `json:"questions" jsonschema:"nonnullable=true"`
	AutoResolutionMS *uint64                        `json:"autoResolutionMs"`
	RequestID        string                         `json:"requestId,omitempty"`
	StartedAtMS      int64                          `json:"startedAtMs,omitempty"`
	Prompt           string                         `json:"prompt,omitempty"`
	Placeholder      string                         `json:"placeholder,omitempty"`
	Required         bool                           `json:"required,omitempty"`
	Options          []string                       `json:"options,omitempty"`
	Metadata         map[string]any                 `json:"metadata,omitempty"`
	Reason           string                         `json:"reason,omitempty"`
}

type ToolRequestUserInputAnswer struct {
	Answers []string `json:"answers" jsonschema:"nonnullable=true"`
}

func (a ToolRequestUserInputAnswer) MarshalJSON() ([]byte, error) {
	answers := a.Answers
	if answers == nil {
		answers = []string{}
	}
	return json.Marshal(struct {
		Answers []string `json:"answers"`
	}{Answers: answers})
}

func (a *ToolRequestUserInputAnswer) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode user input answer into nil receiver")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	for name := range payload {
		if name != "answers" {
			return fmt.Errorf("unknown user input answer field %q", name)
		}
	}
	answersRaw := payload["answers"]
	if len(answersRaw) == 0 || bytes.Equal(bytes.TrimSpace(answersRaw), []byte("null")) {
		return errors.New("user input answer requires answers array")
	}
	var answers []string
	if err := json.Unmarshal(answersRaw, &answers); err != nil {
		return fmt.Errorf("decode user input answers: %w", err)
	}
	*a = ToolRequestUserInputAnswer{Answers: answers}
	return nil
}

type ToolRequestUserInputResponse struct {
	Answers map[string]ToolRequestUserInputAnswer `json:"answers" jsonschema:"nonnullable=true"`
}

func (r ToolRequestUserInputResponse) MarshalJSON() ([]byte, error) {
	answers := r.Answers
	if answers == nil {
		answers = map[string]ToolRequestUserInputAnswer{}
	}
	return json.Marshal(struct {
		Answers map[string]ToolRequestUserInputAnswer `json:"answers"`
	}{Answers: answers})
}

func (r *ToolRequestUserInputResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode user input response into nil receiver")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	for name := range payload {
		if name != "answers" {
			return fmt.Errorf("unknown user input response field %q", name)
		}
	}
	answersRaw := payload["answers"]
	if len(answersRaw) == 0 || bytes.Equal(bytes.TrimSpace(answersRaw), []byte("null")) {
		return errors.New("user input response requires answers object")
	}
	var answers map[string]ToolRequestUserInputAnswer
	if err := json.Unmarshal(answersRaw, &answers); err != nil {
		return fmt.Errorf("decode user input response answers: %w", err)
	}
	*r = ToolRequestUserInputResponse{Answers: answers}
	return nil
}
