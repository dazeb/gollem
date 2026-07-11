package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ResponseItem retains one validated public model response item. It remains
// independent from provider payloads and Gollem's durable timeline records.
type ResponseItem struct {
	raw json.RawMessage
}

func (i ResponseItem) MarshalJSON() ([]byte, error) {
	if len(i.raw) == 0 {
		return nil, errors.New("response item has no value")
	}
	return validateResponseItemJSON(i.raw)
}

func (i *ResponseItem) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode response item into nil receiver")
	}
	canonical, err := validateResponseItemJSON(data)
	if err != nil {
		return err
	}
	i.raw = canonical
	return nil
}

var responseItemFields = []string{
	"type",
	"id",
	"role",
	"content",
	"phase",
	"internal_chat_message_metadata_passthrough",
	"author",
	"recipient",
	"summary",
	"encrypted_content",
	"call_id",
	"status",
	"action",
	"name",
	"namespace",
	"arguments",
	"execution",
	"output",
	"input",
	"tools",
	"revised_prompt",
	"result",
}

const responseItemMetadataField = "internal_chat_message_metadata_passthrough"

func validateResponseItemJSON(data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(data, "response item", responseItemFields...)
	if err != nil {
		return nil, err
	}
	itemType, err := decodeRequiredThreadItemValue[string](payload, "response item", "type")
	if err != nil {
		return nil, err
	}

	switch itemType {
	case "message":
		return validateMessageResponseItem(payload, itemType)
	case "agent_message":
		return validateAgentMessageResponseItem(payload, itemType)
	case "reasoning":
		return validateReasoningResponseItem(payload, itemType)
	case "local_shell_call":
		return validateLocalShellCallResponseItem(payload, itemType)
	case "function_call":
		return validateFunctionCallResponseItem(payload, itemType)
	case "tool_search_call":
		return validateToolSearchCallResponseItem(payload, itemType)
	case "function_call_output":
		return validateFunctionCallOutputResponseItem(payload, itemType)
	case "custom_tool_call":
		return validateCustomToolCallResponseItem(payload, itemType)
	case "custom_tool_call_output":
		return validateCustomToolCallOutputResponseItem(payload, itemType)
	case "tool_search_output":
		return validateToolSearchOutputResponseItem(payload, itemType)
	case "web_search_call":
		return validateWebSearchCallResponseItem(payload, itemType)
	case "image_generation_call":
		return validateImageGenerationCallResponseItem(payload, itemType)
	case "compaction":
		return validateCompactionResponseItem(payload, itemType)
	case "compaction_trigger", "other":
		if err := rejectThreadItemFields(payload, itemType+" response item", "type"); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: itemType})
	case "context_compaction":
		return validateContextCompactionResponseItem(payload, itemType)
	default:
		return nil, fmt.Errorf("unknown response item type %q", itemType)
	}
}

type responseItemCommon struct {
	ID       *string
	Metadata *InternalChatMessageMetadataPassthrough
}

func decodeResponseItemCommon(payload map[string]json.RawMessage, objectName string) (responseItemCommon, error) {
	id, err := decodeOptionalResponseItemValue[string](payload, objectName, "id")
	if err != nil {
		return responseItemCommon{}, err
	}
	metadata, err := decodeOptionalResponseItemValue[InternalChatMessageMetadataPassthrough](
		payload,
		objectName,
		responseItemMetadataField,
	)
	if err != nil {
		return responseItemCommon{}, err
	}
	return responseItemCommon{ID: id, Metadata: metadata}, nil
}

func validateMessageResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "message response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "role", "content", "phase", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	role, err := decodeRequiredThreadItemValue[string](payload, objectName, "role")
	if err != nil {
		return nil, err
	}
	content, err := decodeRequiredThreadItemValue[[]ContentItem](payload, objectName, "content")
	if err != nil {
		return nil, err
	}
	phase, err := decodeOptionalResponseItemValue[MessagePhase](payload, objectName, "phase")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type     string                                  `json:"type"`
		ID       *string                                 `json:"id,omitempty"`
		Role     string                                  `json:"role"`
		Content  []ContentItem                           `json:"content"`
		Phase    *MessagePhase                           `json:"phase,omitempty"`
		Metadata *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{Type: itemType, ID: common.ID, Role: role, Content: content, Phase: phase, Metadata: common.Metadata})
}

func validateAgentMessageResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "agent-message response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "author", "recipient", "content", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	author, err := decodeRequiredThreadItemValue[string](payload, objectName, "author")
	if err != nil {
		return nil, err
	}
	recipient, err := decodeRequiredThreadItemValue[string](payload, objectName, "recipient")
	if err != nil {
		return nil, err
	}
	content, err := decodeRequiredThreadItemValue[[]AgentMessageInputContent](payload, objectName, "content")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type      string                                  `json:"type"`
		ID        *string                                 `json:"id,omitempty"`
		Author    string                                  `json:"author"`
		Recipient string                                  `json:"recipient"`
		Content   []AgentMessageInputContent              `json:"content"`
		Metadata  *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{Type: itemType, ID: common.ID, Author: author, Recipient: recipient, Content: content, Metadata: common.Metadata})
}

func validateReasoningResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "reasoning response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "summary", "content", "encrypted_content", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	summary, err := decodeRequiredThreadItemValue[[]ReasoningItemReasoningSummary](payload, objectName, "summary")
	if err != nil {
		return nil, err
	}
	content, err := decodeOptionalResponseItemValue[[]ReasoningItemContent](payload, objectName, "content")
	if err != nil {
		return nil, err
	}
	encryptedContent, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "encrypted_content")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type             string                                  `json:"type"`
		ID               *string                                 `json:"id,omitempty"`
		Summary          []ReasoningItemReasoningSummary         `json:"summary"`
		Content          *[]ReasoningItemContent                 `json:"content,omitempty"`
		EncryptedContent *string                                 `json:"encrypted_content"`
		Metadata         *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{
		Type: itemType, ID: common.ID, Summary: summary, Content: content,
		EncryptedContent: encryptedContent, Metadata: common.Metadata,
	})
}

func validateLocalShellCallResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "local-shell-call response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "call_id", "status", "action", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	callID, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "call_id")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredThreadItemValue[LocalShellStatus](payload, objectName, "status")
	if err != nil {
		return nil, err
	}
	action, err := decodeRequiredThreadItemValue[LocalShellAction](payload, objectName, "action")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type     string                                  `json:"type"`
		ID       *string                                 `json:"id,omitempty"`
		CallID   *string                                 `json:"call_id"`
		Status   LocalShellStatus                        `json:"status"`
		Action   LocalShellAction                        `json:"action"`
		Metadata *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{Type: itemType, ID: common.ID, CallID: callID, Status: status, Action: action, Metadata: common.Metadata})
}

func validateFunctionCallResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "function-call response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "name", "namespace", "arguments", "call_id", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return nil, err
	}
	namespace, err := decodeOptionalResponseItemValue[string](payload, objectName, "namespace")
	if err != nil {
		return nil, err
	}
	arguments, err := decodeRequiredThreadItemValue[string](payload, objectName, "arguments")
	if err != nil {
		return nil, err
	}
	callID, err := decodeRequiredThreadItemValue[string](payload, objectName, "call_id")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type      string                                  `json:"type"`
		ID        *string                                 `json:"id,omitempty"`
		Name      string                                  `json:"name"`
		Namespace *string                                 `json:"namespace,omitempty"`
		Arguments string                                  `json:"arguments"`
		CallID    string                                  `json:"call_id"`
		Metadata  *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{
		Type: itemType, ID: common.ID, Name: name, Namespace: namespace,
		Arguments: arguments, CallID: callID, Metadata: common.Metadata,
	})
}

func validateToolSearchCallResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "tool-search-call response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "call_id", "status", "execution", "arguments", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	callID, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "call_id")
	if err != nil {
		return nil, err
	}
	status, err := decodeOptionalResponseItemValue[string](payload, objectName, "status")
	if err != nil {
		return nil, err
	}
	execution, err := decodeRequiredThreadItemValue[string](payload, objectName, "execution")
	if err != nil {
		return nil, err
	}
	arguments, err := decodeRequiredResponseItemJSON(payload, objectName, "arguments")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type      string                                  `json:"type"`
		ID        *string                                 `json:"id,omitempty"`
		CallID    *string                                 `json:"call_id"`
		Status    *string                                 `json:"status,omitempty"`
		Execution string                                  `json:"execution"`
		Arguments json.RawMessage                         `json:"arguments"`
		Metadata  *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{
		Type: itemType, ID: common.ID, CallID: callID, Status: status,
		Execution: execution, Arguments: arguments, Metadata: common.Metadata,
	})
}

func validateFunctionCallOutputResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "function-call-output response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "call_id", "output", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	callID, err := decodeRequiredThreadItemValue[string](payload, objectName, "call_id")
	if err != nil {
		return nil, err
	}
	output, err := decodeRequiredThreadItemValue[FunctionCallOutputBody](payload, objectName, "output")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type     string                                  `json:"type"`
		ID       *string                                 `json:"id,omitempty"`
		CallID   string                                  `json:"call_id"`
		Output   FunctionCallOutputBody                  `json:"output"`
		Metadata *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{Type: itemType, ID: common.ID, CallID: callID, Output: output, Metadata: common.Metadata})
}

func validateCustomToolCallResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "custom-tool-call response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "status", "call_id", "name", "namespace", "input", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	status, err := decodeOptionalResponseItemValue[string](payload, objectName, "status")
	if err != nil {
		return nil, err
	}
	callID, err := decodeRequiredThreadItemValue[string](payload, objectName, "call_id")
	if err != nil {
		return nil, err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return nil, err
	}
	namespace, err := decodeOptionalResponseItemValue[string](payload, objectName, "namespace")
	if err != nil {
		return nil, err
	}
	input, err := decodeRequiredThreadItemValue[string](payload, objectName, "input")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type      string                                  `json:"type"`
		ID        *string                                 `json:"id,omitempty"`
		Status    *string                                 `json:"status,omitempty"`
		CallID    string                                  `json:"call_id"`
		Name      string                                  `json:"name"`
		Namespace *string                                 `json:"namespace,omitempty"`
		Input     string                                  `json:"input"`
		Metadata  *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{
		Type: itemType, ID: common.ID, Status: status, CallID: callID, Name: name,
		Namespace: namespace, Input: input, Metadata: common.Metadata,
	})
}

func validateCustomToolCallOutputResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "custom-tool-call-output response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "call_id", "name", "output", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	callID, err := decodeRequiredThreadItemValue[string](payload, objectName, "call_id")
	if err != nil {
		return nil, err
	}
	name, err := decodeOptionalResponseItemValue[string](payload, objectName, "name")
	if err != nil {
		return nil, err
	}
	output, err := decodeRequiredThreadItemValue[FunctionCallOutputBody](payload, objectName, "output")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type     string                                  `json:"type"`
		ID       *string                                 `json:"id,omitempty"`
		CallID   string                                  `json:"call_id"`
		Name     *string                                 `json:"name,omitempty"`
		Output   FunctionCallOutputBody                  `json:"output"`
		Metadata *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{Type: itemType, ID: common.ID, CallID: callID, Name: name, Output: output, Metadata: common.Metadata})
}

func validateToolSearchOutputResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "tool-search-output response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "call_id", "status", "execution", "tools", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	callID, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "call_id")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredThreadItemValue[string](payload, objectName, "status")
	if err != nil {
		return nil, err
	}
	execution, err := decodeRequiredThreadItemValue[string](payload, objectName, "execution")
	if err != nil {
		return nil, err
	}
	tools, err := decodeRequiredResponseItemJSONArray(payload, objectName, "tools")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type      string                                  `json:"type"`
		ID        *string                                 `json:"id,omitempty"`
		CallID    *string                                 `json:"call_id"`
		Status    string                                  `json:"status"`
		Execution string                                  `json:"execution"`
		Tools     []json.RawMessage                       `json:"tools"`
		Metadata  *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{
		Type: itemType, ID: common.ID, CallID: callID, Status: status,
		Execution: execution, Tools: tools, Metadata: common.Metadata,
	})
}

func validateWebSearchCallResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "web-search-call response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "status", "action", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	status, err := decodeOptionalResponseItemValue[string](payload, objectName, "status")
	if err != nil {
		return nil, err
	}
	action, err := decodeOptionalResponseItemValue[WebSearchAction](payload, objectName, "action")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type     string                                  `json:"type"`
		ID       *string                                 `json:"id,omitempty"`
		Status   *string                                 `json:"status,omitempty"`
		Action   *WebSearchAction                        `json:"action,omitempty"`
		Metadata *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{Type: itemType, ID: common.ID, Status: status, Action: action, Metadata: common.Metadata})
}

func validateImageGenerationCallResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "image-generation-call response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "status", "revised_prompt", "result", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredThreadItemValue[string](payload, objectName, "status")
	if err != nil {
		return nil, err
	}
	revisedPrompt, err := decodeOptionalResponseItemValue[string](payload, objectName, "revised_prompt")
	if err != nil {
		return nil, err
	}
	result, err := decodeRequiredThreadItemValue[string](payload, objectName, "result")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type          string                                  `json:"type"`
		ID            *string                                 `json:"id,omitempty"`
		Status        string                                  `json:"status"`
		RevisedPrompt *string                                 `json:"revised_prompt,omitempty"`
		Result        string                                  `json:"result"`
		Metadata      *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{
		Type: itemType, ID: common.ID, Status: status, RevisedPrompt: revisedPrompt,
		Result: result, Metadata: common.Metadata,
	})
}

func validateCompactionResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "compaction response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "encrypted_content", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	encryptedContent, err := decodeRequiredThreadItemValue[string](payload, objectName, "encrypted_content")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type             string                                  `json:"type"`
		ID               *string                                 `json:"id,omitempty"`
		EncryptedContent string                                  `json:"encrypted_content"`
		Metadata         *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{Type: itemType, ID: common.ID, EncryptedContent: encryptedContent, Metadata: common.Metadata})
}

func validateContextCompactionResponseItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "context-compaction response item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "encrypted_content", responseItemMetadataField); err != nil {
		return nil, err
	}
	common, err := decodeResponseItemCommon(payload, objectName)
	if err != nil {
		return nil, err
	}
	encryptedContent, err := decodeOptionalResponseItemValue[string](payload, objectName, "encrypted_content")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type             string                                  `json:"type"`
		ID               *string                                 `json:"id,omitempty"`
		EncryptedContent *string                                 `json:"encrypted_content,omitempty"`
		Metadata         *InternalChatMessageMetadataPassthrough `json:"internal_chat_message_metadata_passthrough,omitempty"`
	}{Type: itemType, ID: common.ID, EncryptedContent: encryptedContent, Metadata: common.Metadata})
}

func decodeOptionalResponseItemValue[T any](payload map[string]json.RawMessage, objectName, fieldName string) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s %s cannot be null", objectName, fieldName)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func decodeRequiredResponseItemJSON(payload map[string]json.RawMessage, objectName, fieldName string) (json.RawMessage, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	canonical, err := canonicalizeResponseItemJSONValue(raw)
	if err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return canonical, nil
}

func decodeRequiredResponseItemJSONArray(payload map[string]json.RawMessage, objectName, fieldName string) ([]json.RawMessage, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	canonical, err := canonicalizeResponseItemJSONValue(raw)
	if err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(canonical, &values); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return values, nil
}

func canonicalizeResponseItemJSONValue(data []byte) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("trailing JSON value: %w", err)
	}
	canonical, err := json.Marshal(value)
	return canonical, err
}
