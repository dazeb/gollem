package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ThreadItem retains one validated public v2 thread item. It remains separate
// from Gollem's live item notifications and durable timeline records.
type ThreadItem struct {
	raw json.RawMessage
}

func (i ThreadItem) MarshalJSON() ([]byte, error) {
	if len(i.raw) == 0 {
		return nil, errors.New("thread item has no value")
	}
	return validateThreadItemJSON(i.raw)
}

func (i *ThreadItem) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode thread item into nil receiver")
	}
	canonical, err := validateThreadItemJSON(data)
	if err != nil {
		return err
	}
	i.raw = canonical
	return nil
}

var threadItemFields = []string{
	"type",
	"id",
	"clientId",
	"content",
	"fragments",
	"text",
	"phase",
	"memoryCitation",
	"summary",
	"command",
	"cwd",
	"processId",
	"source",
	"status",
	"commandActions",
	"aggregatedOutput",
	"exitCode",
	"durationMs",
	"changes",
	"server",
	"tool",
	"arguments",
	"appContext",
	"mcpAppResourceUri",
	"pluginId",
	"result",
	"error",
	"namespace",
	"contentItems",
	"success",
	"senderThreadId",
	"receiverThreadIds",
	"prompt",
	"model",
	"reasoningEffort",
	"agentsStates",
	"kind",
	"agentThreadId",
	"agentPath",
	"query",
	"action",
	"path",
	"revisedPrompt",
	"savedPath",
	"review",
}

func validateThreadItemJSON(data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(data, "thread item", threadItemFields...)
	if err != nil {
		return nil, err
	}
	itemType, err := decodeRequiredThreadItemValue[string](payload, "thread item", "type")
	if err != nil {
		return nil, err
	}

	switch itemType {
	case "userMessage":
		return validateUserMessageThreadItem(payload, itemType)
	case "hookPrompt":
		return validateHookPromptThreadItem(payload, itemType)
	case "agentMessage":
		return validateAgentMessageThreadItem(payload, itemType)
	case "plan", "enteredReviewMode", "exitedReviewMode":
		return validateTextThreadItem(payload, itemType)
	case "reasoning":
		return validateReasoningThreadItem(payload, itemType)
	case "commandExecution":
		return validateCommandExecutionThreadItem(payload, itemType)
	case "fileChange":
		return validateFileChangeThreadItem(payload, itemType)
	case "mcpToolCall":
		return validateMcpToolCallThreadItem(payload, itemType)
	case "dynamicToolCall":
		return validateDynamicToolCallThreadItem(payload, itemType)
	case "collabAgentToolCall":
		return validateCollabAgentToolCallThreadItem(payload, itemType)
	case "subAgentActivity":
		return validateSubAgentActivityThreadItem(payload, itemType)
	case "webSearch":
		return validateWebSearchThreadItem(payload, itemType)
	case "imageView":
		return validateImageViewThreadItem(payload, itemType)
	case "sleep":
		return validateSleepThreadItem(payload, itemType)
	case "imageGeneration":
		return validateImageGenerationThreadItem(payload, itemType)
	case "contextCompaction":
		return validateContextCompactionThreadItem(payload, itemType)
	default:
		return nil, fmt.Errorf("unknown thread item type %q", itemType)
	}
}

func validateUserMessageThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "user-message thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "clientId", "content"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	clientID, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "clientId")
	if err != nil {
		return nil, err
	}
	content, err := decodeRequiredThreadItemArray[UserInput](payload, objectName, "content")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type     string      `json:"type"`
		ID       string      `json:"id"`
		ClientID *string     `json:"clientId"`
		Content  []UserInput `json:"content"`
	}{Type: itemType, ID: id, ClientID: clientID, Content: content})
}

func validateHookPromptThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "hook-prompt thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "fragments"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	fragments, err := decodeRequiredThreadItemArray[HookPromptFragment](payload, objectName, "fragments")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type      string               `json:"type"`
		ID        string               `json:"id"`
		Fragments []HookPromptFragment `json:"fragments"`
	}{Type: itemType, ID: id, Fragments: fragments})
}

func validateAgentMessageThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "agent-message thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "text", "phase", "memoryCitation"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	text, err := decodeRequiredThreadItemValue[string](payload, objectName, "text")
	if err != nil {
		return nil, err
	}
	phase, err := decodeRequiredNullableThreadItemValue[MessagePhase](payload, objectName, "phase")
	if err != nil {
		return nil, err
	}
	memoryCitation, err := decodeRequiredNullableThreadItemMemoryCitation(payload, objectName, "memoryCitation")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type           string          `json:"type"`
		ID             string          `json:"id"`
		Text           string          `json:"text"`
		Phase          *MessagePhase   `json:"phase"`
		MemoryCitation *MemoryCitation `json:"memoryCitation"`
	}{Type: itemType, ID: id, Text: text, Phase: phase, MemoryCitation: memoryCitation})
}

func validateTextThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	fieldName := "text"
	if itemType == "enteredReviewMode" || itemType == "exitedReviewMode" {
		fieldName = "review"
	}
	objectName := itemType + " thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", fieldName); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	value, err := decodeRequiredThreadItemValue[string](payload, objectName, fieldName)
	if err != nil {
		return nil, err
	}
	if fieldName == "review" {
		return json.Marshal(struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			Review string `json:"review"`
		}{Type: itemType, ID: id, Review: value})
	}
	return json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Text string `json:"text"`
	}{Type: itemType, ID: id, Text: value})
}

func validateReasoningThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "reasoning thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "summary", "content"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	summary, err := decodeRequiredNonNullStringArray(payload, objectName, "summary")
	if err != nil {
		return nil, err
	}
	content, err := decodeRequiredNonNullStringArray(payload, objectName, "content")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type    string   `json:"type"`
		ID      string   `json:"id"`
		Summary []string `json:"summary"`
		Content []string `json:"content"`
	}{Type: itemType, ID: id, Summary: summary, Content: content})
}

func validateCommandExecutionThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "command-execution thread item"
	if err := rejectThreadItemFields(
		payload,
		objectName,
		"type", "id", "command", "cwd", "processId", "source", "status",
		"commandActions", "aggregatedOutput", "exitCode", "durationMs",
	); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	command, err := decodeRequiredThreadItemValue[string](payload, objectName, "command")
	if err != nil {
		return nil, err
	}
	cwd, err := decodeRequiredThreadItemValue[LegacyAppPathString](payload, objectName, "cwd")
	if err != nil {
		return nil, err
	}
	processID, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "processId")
	if err != nil {
		return nil, err
	}
	source, err := decodeRequiredThreadItemValue[CommandExecutionSource](payload, objectName, "source")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredClosedThreadItemString(
		payload, objectName, "status",
		CommandExecutionStatusInProgress, CommandExecutionStatusCompleted,
		CommandExecutionStatusFailed, CommandExecutionStatusDeclined,
	)
	if err != nil {
		return nil, err
	}
	commandActions, err := decodeRequiredThreadItemArray[CommandAction](payload, objectName, "commandActions")
	if err != nil {
		return nil, err
	}
	aggregatedOutput, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "aggregatedOutput")
	if err != nil {
		return nil, err
	}
	exitCode, err := decodeRequiredNullableThreadItemValue[int32](payload, objectName, "exitCode")
	if err != nil {
		return nil, err
	}
	durationMS, err := decodeRequiredNullableThreadItemValue[int64](payload, objectName, "durationMs")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type             string                 `json:"type"`
		ID               string                 `json:"id"`
		Command          string                 `json:"command"`
		CWD              LegacyAppPathString    `json:"cwd"`
		ProcessID        *string                `json:"processId"`
		Source           CommandExecutionSource `json:"source"`
		Status           CommandExecutionStatus `json:"status"`
		CommandActions   []CommandAction        `json:"commandActions"`
		AggregatedOutput *string                `json:"aggregatedOutput"`
		ExitCode         *int32                 `json:"exitCode"`
		DurationMS       *int64                 `json:"durationMs"`
	}{
		Type: itemType, ID: id, Command: command, CWD: cwd, ProcessID: processID,
		Source: source, Status: status, CommandActions: commandActions,
		AggregatedOutput: aggregatedOutput, ExitCode: exitCode, DurationMS: durationMS,
	})
}

func validateFileChangeThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "file-change thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "changes", "status"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	changes, err := decodeRequiredThreadItemFileChanges(payload, objectName, "changes")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredClosedThreadItemString(
		payload, objectName, "status",
		PatchApplyStatusInProgress, PatchApplyStatusCompleted,
		PatchApplyStatusFailed, PatchApplyStatusDeclined,
	)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type    string                       `json:"type"`
		ID      string                       `json:"id"`
		Changes []strictThreadItemFileChange `json:"changes"`
		Status  PatchApplyStatus             `json:"status"`
	}{Type: itemType, ID: id, Changes: changes, Status: status})
}

func validateMcpToolCallThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "MCP tool-call thread item"
	if err := rejectThreadItemFields(
		payload,
		objectName,
		"type", "id", "server", "tool", "status", "arguments", "appContext",
		"mcpAppResourceUri", "pluginId", "result", "error", "durationMs",
	); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	server, err := decodeRequiredThreadItemValue[string](payload, objectName, "server")
	if err != nil {
		return nil, err
	}
	tool, err := decodeRequiredThreadItemValue[string](payload, objectName, "tool")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredClosedThreadItemString(
		payload, objectName, "status",
		McpToolCallStatusInProgress, McpToolCallStatusCompleted, McpToolCallStatusFailed,
	)
	if err != nil {
		return nil, err
	}
	arguments, err := decodeRequiredThreadItemJSONValue(payload, objectName, "arguments")
	if err != nil {
		return nil, err
	}
	appContext, err := decodeRequiredNullableThreadItemValue[McpToolCallAppContext](payload, objectName, "appContext")
	if err != nil {
		return nil, err
	}
	mcpAppResourceURI, err := decodeOptionalResponseItemValue[string](payload, objectName, "mcpAppResourceUri")
	if err != nil {
		return nil, err
	}
	pluginID, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "pluginId")
	if err != nil {
		return nil, err
	}
	result, err := decodeRequiredNullableThreadItemValue[McpToolCallResult](payload, objectName, "result")
	if err != nil {
		return nil, err
	}
	callError, err := decodeRequiredNullableThreadItemMcpError(payload, objectName, "error")
	if err != nil {
		return nil, err
	}
	durationMS, err := decodeRequiredNullableThreadItemValue[int64](payload, objectName, "durationMs")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type              string                    `json:"type"`
		ID                string                    `json:"id"`
		Server            string                    `json:"server"`
		Tool              string                    `json:"tool"`
		Status            McpToolCallStatus         `json:"status"`
		Arguments         JsonValue                 `json:"arguments"`
		AppContext        *McpToolCallAppContext    `json:"appContext"`
		MCPAppResourceURI *string                   `json:"mcpAppResourceUri,omitempty"`
		PluginID          *string                   `json:"pluginId"`
		Result            *McpToolCallResult        `json:"result"`
		Error             *strictThreadItemMcpError `json:"error"`
		DurationMS        *int64                    `json:"durationMs"`
	}{
		Type: itemType, ID: id, Server: server, Tool: tool, Status: status,
		Arguments: arguments, AppContext: appContext, MCPAppResourceURI: mcpAppResourceURI,
		PluginID: pluginID, Result: result, Error: callError, DurationMS: durationMS,
	})
}

func validateDynamicToolCallThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "dynamic tool-call thread item"
	if err := rejectThreadItemFields(
		payload, objectName,
		"type", "id", "namespace", "tool", "arguments", "status", "contentItems", "success", "durationMs",
	); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	namespace, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "namespace")
	if err != nil {
		return nil, err
	}
	tool, err := decodeRequiredThreadItemValue[string](payload, objectName, "tool")
	if err != nil {
		return nil, err
	}
	arguments, err := decodeRequiredThreadItemJSONValue(payload, objectName, "arguments")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredClosedThreadItemString(
		payload, objectName, "status",
		DynamicToolCallStatusInProgress, DynamicToolCallStatusCompleted, DynamicToolCallStatusFailed,
	)
	if err != nil {
		return nil, err
	}
	contentItems, err := decodeRequiredNullableThreadItemArray[DynamicToolCallOutputContentItem](payload, objectName, "contentItems")
	if err != nil {
		return nil, err
	}
	success, err := decodeRequiredNullableThreadItemValue[bool](payload, objectName, "success")
	if err != nil {
		return nil, err
	}
	durationMS, err := decodeRequiredNullableThreadItemValue[int64](payload, objectName, "durationMs")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type         string                              `json:"type"`
		ID           string                              `json:"id"`
		Namespace    *string                             `json:"namespace"`
		Tool         string                              `json:"tool"`
		Arguments    JsonValue                           `json:"arguments"`
		Status       DynamicToolCallStatus               `json:"status"`
		ContentItems *[]DynamicToolCallOutputContentItem `json:"contentItems"`
		Success      *bool                               `json:"success"`
		DurationMS   *int64                              `json:"durationMs"`
	}{
		Type: itemType, ID: id, Namespace: namespace, Tool: tool, Arguments: arguments,
		Status: status, ContentItems: contentItems, Success: success, DurationMS: durationMS,
	})
}

func validateCollabAgentToolCallThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "collab-agent tool-call thread item"
	if err := rejectThreadItemFields(
		payload, objectName,
		"type", "id", "tool", "status", "senderThreadId", "receiverThreadIds",
		"prompt", "model", "reasoningEffort", "agentsStates",
	); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	tool, err := decodeRequiredThreadItemValue[CollabAgentTool](payload, objectName, "tool")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredThreadItemValue[CollabAgentToolCallStatus](payload, objectName, "status")
	if err != nil {
		return nil, err
	}
	senderThreadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "senderThreadId")
	if err != nil {
		return nil, err
	}
	receiverThreadIDs, err := decodeRequiredNonNullStringArray(payload, objectName, "receiverThreadIds")
	if err != nil {
		return nil, err
	}
	prompt, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "prompt")
	if err != nil {
		return nil, err
	}
	model, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "model")
	if err != nil {
		return nil, err
	}
	reasoningEffort, err := decodeRequiredNullableThreadItemValue[ReasoningEffort](payload, objectName, "reasoningEffort")
	if err != nil {
		return nil, err
	}
	agentsStates, err := decodeRequiredThreadItemAgentStates(payload, objectName, "agentsStates")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type              string                      `json:"type"`
		ID                string                      `json:"id"`
		Tool              CollabAgentTool             `json:"tool"`
		Status            CollabAgentToolCallStatus   `json:"status"`
		SenderThreadID    string                      `json:"senderThreadId"`
		ReceiverThreadIDs []string                    `json:"receiverThreadIds"`
		Prompt            *string                     `json:"prompt"`
		Model             *string                     `json:"model"`
		ReasoningEffort   *ReasoningEffort            `json:"reasoningEffort"`
		AgentsStates      map[string]CollabAgentState `json:"agentsStates"`
	}{
		Type: itemType, ID: id, Tool: tool, Status: status, SenderThreadID: senderThreadID,
		ReceiverThreadIDs: receiverThreadIDs, Prompt: prompt, Model: model,
		ReasoningEffort: reasoningEffort, AgentsStates: agentsStates,
	})
}

func validateSubAgentActivityThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "subagent-activity thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "kind", "agentThreadId", "agentPath"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	kind, err := decodeRequiredThreadItemValue[SubAgentActivityKind](payload, objectName, "kind")
	if err != nil {
		return nil, err
	}
	agentThreadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "agentThreadId")
	if err != nil {
		return nil, err
	}
	agentPath, err := decodeRequiredThreadItemValue[string](payload, objectName, "agentPath")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type          string               `json:"type"`
		ID            string               `json:"id"`
		Kind          SubAgentActivityKind `json:"kind"`
		AgentThreadID string               `json:"agentThreadId"`
		AgentPath     string               `json:"agentPath"`
	}{Type: itemType, ID: id, Kind: kind, AgentThreadID: agentThreadID, AgentPath: agentPath})
}

func validateWebSearchThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "web-search thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "query", "action"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	query, err := decodeRequiredThreadItemValue[string](payload, objectName, "query")
	if err != nil {
		return nil, err
	}
	action, err := decodeRequiredNullableThreadItemValue[WebSearchAction](payload, objectName, "action")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type   string           `json:"type"`
		ID     string           `json:"id"`
		Query  string           `json:"query"`
		Action *WebSearchAction `json:"action"`
	}{Type: itemType, ID: id, Query: query, Action: action})
}

func validateImageViewThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "image-view thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "path"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	path, err := decodeRequiredThreadItemValue[LegacyAppPathString](payload, objectName, "path")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type string              `json:"type"`
		ID   string              `json:"id"`
		Path LegacyAppPathString `json:"path"`
	}{Type: itemType, ID: id, Path: path})
}

func validateSleepThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "sleep thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "durationMs"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	durationMS, err := decodeRequiredThreadItemValue[uint64](payload, objectName, "durationMs")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		DurationMS uint64 `json:"durationMs"`
	}{Type: itemType, ID: id, DurationMS: durationMS})
}

func validateImageGenerationThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "image-generation thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id", "status", "revisedPrompt", "result", "savedPath"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredThreadItemValue[string](payload, objectName, "status")
	if err != nil {
		return nil, err
	}
	revisedPrompt, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "revisedPrompt")
	if err != nil {
		return nil, err
	}
	result, err := decodeRequiredThreadItemValue[string](payload, objectName, "result")
	if err != nil {
		return nil, err
	}
	savedPath, err := decodeOptionalResponseItemValue[AbsolutePathBuf](payload, objectName, "savedPath")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type          string           `json:"type"`
		ID            string           `json:"id"`
		Status        string           `json:"status"`
		RevisedPrompt *string          `json:"revisedPrompt"`
		Result        string           `json:"result"`
		SavedPath     *AbsolutePathBuf `json:"savedPath,omitempty"`
	}{Type: itemType, ID: id, Status: status, RevisedPrompt: revisedPrompt, Result: result, SavedPath: savedPath})
}

func validateContextCompactionThreadItem(payload map[string]json.RawMessage, itemType string) (json.RawMessage, error) {
	const objectName = "context-compaction thread item"
	if err := rejectThreadItemFields(payload, objectName, "type", "id"); err != nil {
		return nil, err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{Type: itemType, ID: id})
}

type strictThreadItemFileChange struct {
	Path string
	Kind strictThreadItemPatchChangeKind
	Diff string
}

func (c strictThreadItemFileChange) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Path string                          `json:"path"`
		Kind strictThreadItemPatchChangeKind `json:"kind"`
		Diff string                          `json:"diff"`
	}{Path: c.Path, Kind: c.Kind, Diff: c.Diff})
}

type strictThreadItemPatchChangeKind struct {
	Type     string
	MovePath *string
}

func (k strictThreadItemPatchChangeKind) MarshalJSON() ([]byte, error) {
	switch k.Type {
	case "add", "delete":
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: k.Type})
	case "update":
		return json.Marshal(struct {
			Type     string  `json:"type"`
			MovePath *string `json:"move_path"`
		}{Type: k.Type, MovePath: k.MovePath})
	default:
		return nil, fmt.Errorf("unknown thread-item patch change kind %q", k.Type)
	}
}

type strictThreadItemMcpError struct {
	Message string `json:"message"`
}

func decodeRequiredThreadItemArray[T any](payload map[string]json.RawMessage, objectName, fieldName string) ([]T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]T, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: value cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return values, nil
}

func decodeRequiredNullableThreadItemArray[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*[]T, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if isJSONNull(raw) {
		return nil, nil
	}
	values, err := decodeRequiredThreadItemArray[T](payload, objectName, fieldName)
	if err != nil {
		return nil, err
	}
	return &values, nil
}

func decodeRequiredClosedThreadItemString[T ~string](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
	allowed ...T,
) (T, error) {
	value, err := decodeRequiredThreadItemValue[string](payload, objectName, fieldName)
	if err != nil {
		var zero T
		return zero, err
	}
	parsed := T(value)
	for _, candidate := range allowed {
		if parsed == candidate {
			return parsed, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("unknown %s %s %q", objectName, fieldName, value)
}

func decodeRequiredThreadItemJSONValue(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (JsonValue, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return JsonValue{}, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	var value JsonValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return JsonValue{}, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

func decodeRequiredNullableThreadItemMemoryCitation(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*MemoryCitation, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if isJSONNull(raw) {
		return nil, nil
	}
	citationPayload, err := decodeExactThreadItemObject(raw, "thread-item memory citation", "entries", "threadIds")
	if err != nil {
		return nil, err
	}
	var citation MemoryCitation
	if err := json.Unmarshal(raw, &citation); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	threadIDs, err := decodeRequiredNonNullStringArray(citationPayload, "thread-item memory citation", "threadIds")
	if err != nil {
		return nil, err
	}
	citation.ThreadIDs = threadIDs
	return &citation, nil
}

func decodeRequiredThreadItemFileChanges(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) ([]strictThreadItemFileChange, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	changes := make([]strictThreadItemFileChange, len(elements))
	for index, element := range elements {
		change, err := decodeStrictThreadItemFileChange(element)
		if err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
		changes[index] = change
	}
	return changes, nil
}

func decodeStrictThreadItemFileChange(data []byte) (strictThreadItemFileChange, error) {
	payload, err := decodeExactThreadItemObject(data, "thread-item file update change", "path", "kind", "diff")
	if err != nil {
		return strictThreadItemFileChange{}, err
	}
	path, err := decodeRequiredThreadItemValue[string](payload, "thread-item file update change", "path")
	if err != nil {
		return strictThreadItemFileChange{}, err
	}
	kindRaw, ok := payload["kind"]
	if !ok || isJSONNull(kindRaw) {
		return strictThreadItemFileChange{}, errors.New("thread-item file update change requires kind")
	}
	kind, err := decodeStrictThreadItemPatchChangeKind(kindRaw)
	if err != nil {
		return strictThreadItemFileChange{}, err
	}
	diff, err := decodeRequiredThreadItemValue[string](payload, "thread-item file update change", "diff")
	if err != nil {
		return strictThreadItemFileChange{}, err
	}
	return strictThreadItemFileChange{Path: path, Kind: kind, Diff: diff}, nil
}

func decodeStrictThreadItemPatchChangeKind(data []byte) (strictThreadItemPatchChangeKind, error) {
	payload, err := decodeExactThreadItemObject(data, "thread-item patch change kind", "type", "move_path")
	if err != nil {
		return strictThreadItemPatchChangeKind{}, err
	}
	kind, err := decodeRequiredThreadItemValue[string](payload, "thread-item patch change kind", "type")
	if err != nil {
		return strictThreadItemPatchChangeKind{}, err
	}
	switch kind {
	case "add", "delete":
		if err := rejectThreadItemFields(payload, "thread-item "+kind+" patch change kind", "type"); err != nil {
			return strictThreadItemPatchChangeKind{}, err
		}
		return strictThreadItemPatchChangeKind{Type: kind}, nil
	case "update":
		movePath, err := decodeRequiredNullableThreadItemValue[string](payload, "thread-item update patch change kind", "move_path")
		if err != nil {
			return strictThreadItemPatchChangeKind{}, err
		}
		return strictThreadItemPatchChangeKind{Type: kind, MovePath: movePath}, nil
	default:
		return strictThreadItemPatchChangeKind{}, fmt.Errorf("unknown thread-item patch change kind %q", kind)
	}
}

func decodeRequiredNullableThreadItemMcpError(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*strictThreadItemMcpError, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if isJSONNull(raw) {
		return nil, nil
	}
	errorPayload, err := decodeExactThreadItemObject(raw, "thread-item MCP tool-call error", "message")
	if err != nil {
		return nil, err
	}
	message, err := decodeRequiredThreadItemValue[string](errorPayload, "thread-item MCP tool-call error", "message")
	if err != nil {
		return nil, err
	}
	return &strictThreadItemMcpError{Message: message}, nil
}

func decodeRequiredThreadItemAgentStates(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (map[string]CollabAgentState, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	states := make(map[string]CollabAgentState, len(fields))
	for name, stateRaw := range fields {
		if isJSONNull(stateRaw) {
			return nil, fmt.Errorf("decode %s %s[%q]: value cannot be null", objectName, fieldName, name)
		}
		var state CollabAgentState
		if err := json.Unmarshal(stateRaw, &state); err != nil {
			return nil, fmt.Errorf("decode %s %s[%q]: %w", objectName, fieldName, name, err)
		}
		states[name] = state
	}
	return states, nil
}
