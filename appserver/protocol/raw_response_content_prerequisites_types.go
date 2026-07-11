package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

type AgentMessageInputContent struct {
	raw json.RawMessage
}

func (c AgentMessageInputContent) MarshalJSON() ([]byte, error) {
	if len(c.raw) == 0 {
		return nil, errors.New("agent message input content has no value")
	}
	return validateRawResponseContentJSON(c.raw, "agent message input content", agentMessageInputContentVariants)
}

func (c *AgentMessageInputContent) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode agent message input content into nil receiver")
	}
	canonical, err := validateRawResponseContentJSON(data, "agent message input content", agentMessageInputContentVariants)
	if err != nil {
		return err
	}
	c.raw = canonical
	return nil
}

var agentMessageInputContentVariants = []rawResponseContentVariant{
	{contentType: "input_text", field: "text"},
	{contentType: "encrypted_content", field: "encrypted_content"},
}

type ReasoningItemContent struct {
	raw json.RawMessage
}

func (c ReasoningItemContent) MarshalJSON() ([]byte, error) {
	if len(c.raw) == 0 {
		return nil, errors.New("reasoning item content has no value")
	}
	return validateRawResponseContentJSON(c.raw, "reasoning item content", reasoningItemContentVariants)
}

func (c *ReasoningItemContent) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode reasoning item content into nil receiver")
	}
	canonical, err := validateRawResponseContentJSON(data, "reasoning item content", reasoningItemContentVariants)
	if err != nil {
		return err
	}
	c.raw = canonical
	return nil
}

var reasoningItemContentVariants = []rawResponseContentVariant{
	{contentType: "reasoning_text", field: "text"},
	{contentType: "text", field: "text"},
}

type ReasoningItemReasoningSummary struct {
	raw json.RawMessage
}

func (s ReasoningItemReasoningSummary) MarshalJSON() ([]byte, error) {
	if len(s.raw) == 0 {
		return nil, errors.New("reasoning item summary has no value")
	}
	return validateRawResponseContentJSON(s.raw, "reasoning item summary", reasoningItemSummaryVariants)
}

func (s *ReasoningItemReasoningSummary) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode reasoning item summary into nil receiver")
	}
	canonical, err := validateRawResponseContentJSON(data, "reasoning item summary", reasoningItemSummaryVariants)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

var reasoningItemSummaryVariants = []rawResponseContentVariant{
	{contentType: "summary_text", field: "text"},
}

type rawResponseContentVariant struct {
	contentType string
	field       string
}

func validateRawResponseContentJSON(data []byte, objectName string, variants []rawResponseContentVariant) (json.RawMessage, error) {
	allowedFields := []string{"type"}
	for _, variant := range variants {
		if !containsRawResponseField(allowedFields, variant.field) {
			allowedFields = append(allowedFields, variant.field)
		}
	}
	payload, err := decodeExactThreadItemObject(data, objectName, allowedFields...)
	if err != nil {
		return nil, err
	}
	contentType, err := decodeRequiredThreadItemValue[string](payload, objectName, "type")
	if err != nil {
		return nil, err
	}
	for _, variant := range variants {
		if contentType != variant.contentType {
			continue
		}
		if err := rejectThreadItemFields(payload, objectName+" "+contentType, "type", variant.field); err != nil {
			return nil, err
		}
		value, err := decodeRequiredThreadItemValue[string](payload, objectName+" "+contentType, variant.field)
		if err != nil {
			return nil, err
		}
		if variant.field == "encrypted_content" {
			return json.Marshal(struct {
				Type             string `json:"type"`
				EncryptedContent string `json:"encrypted_content"`
			}{Type: contentType, EncryptedContent: value})
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: contentType, Text: value})
	}
	return nil, fmt.Errorf("unknown %s type %q", objectName, contentType)
}

func containsRawResponseField(fields []string, candidate string) bool {
	for _, field := range fields {
		if field == candidate {
			return true
		}
	}
	return false
}
