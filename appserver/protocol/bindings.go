package protocol

// WireTypeBinding maps one method to concrete exported parameter and result
// definitions. Multiple Params entries represent polymorphic payloads.
type WireTypeBinding struct {
	Method  string   `json:"method"`
	Surface Surface  `json:"surface"`
	Params  []string `json:"params,omitempty"`
	Result  []string `json:"result,omitempty"`
}

// ItemPayloadBinding maps a durable timeline item kind to the exported type
// carried in TimelineItem.Payload.
type ItemPayloadBinding struct {
	Kind string `json:"kind"`
	Type string `json:"type"`
}

var wireTypeBindings = []WireTypeBinding{
	{Method: "approval/respond", Surface: SurfaceGollemExtension, Params: []string{"ApprovalRespondParams"}, Result: []string{"ApprovalRespondResult"}},
	{Method: "command/exec/outputDelta", Surface: SurfaceServerNotification, Params: []string{"CommandExecOutputDeltaNotification"}},
	{Method: "command/exec/resize", Surface: SurfaceClientRequest, Params: []string{"CommandExecResizeParams"}, Result: []string{"CommandExecResizeResponse"}},
	{Method: "command/exec/terminate", Surface: SurfaceClientRequest, Params: []string{"CommandExecTerminateParams"}, Result: []string{"CommandExecTerminateResponse"}},
	{Method: "command/exec/write", Surface: SurfaceClientRequest, Params: []string{"CommandExecWriteParams"}, Result: []string{"CommandExecWriteResponse"}},
	{Method: "daemon/restart", Surface: SurfaceGollemExtension, Params: []string{"DaemonShutdownParams"}, Result: []string{"DaemonStopResult"}},
	{Method: "daemon/start", Surface: SurfaceGollemExtension, Result: []string{"DaemonStartResult"}},
	{Method: "daemon/status", Surface: SurfaceGollemExtension, Result: []string{"DaemonStatus"}},
	{Method: "daemon/stop", Surface: SurfaceGollemExtension, Params: []string{"DaemonShutdownParams"}, Result: []string{"DaemonStopResult"}},
	{Method: "daemon/version", Surface: SurfaceGollemExtension, Result: []string{"DaemonVersion"}},
	{Method: "initialize", Surface: SurfaceClientRequest, Params: []string{"InitializeParams"}, Result: []string{"InitializeResponse"}},
	{Method: "initialized", Surface: SurfaceClientNotification},
	{Method: "item/commandExecution/outputDelta", Surface: SurfaceServerNotification, Params: []string{"CommandExecutionOutputDeltaNotificationParams"}},
	{Method: "item/commandExecution/requestApproval", Surface: SurfaceServerRequest, Params: []string{"CommandExecutionApprovalRequestParams"}},
	{Method: "item/completed", Surface: SurfaceServerNotification, Params: []string{"ItemLifecycleNotificationParams", "DynamicToolCallItemCompletedNotificationParams", "CommandExecutionItemCompletedNotificationParams", "FileChangeItemCompletedNotificationParams", "MCPToolCallItemCompletedNotificationParams"}},
	{Method: "item/fileChange/patchUpdated", Surface: SurfaceServerNotification, Params: []string{"FileChangePatchUpdatedNotificationParams"}},
	{Method: "item/fileChange/requestApproval", Surface: SurfaceServerRequest, Params: []string{"FileChangeApprovalRequestParams"}, Result: []string{"FileChangeRequestApprovalResponse"}},
	{Method: "item/mcpToolCall/progress", Surface: SurfaceServerNotification, Params: []string{"MCPToolCallProgressNotificationParams"}},
	{Method: "item/permissions/requestApproval", Surface: SurfaceServerRequest, Params: []string{"PermissionsApprovalRequestParams"}},
	{Method: "item/started", Surface: SurfaceServerNotification, Params: []string{"ItemLifecycleNotificationParams", "DynamicToolCallItemStartedNotificationParams", "CommandExecutionItemStartedNotificationParams", "FileChangeItemStartedNotificationParams", "MCPToolCallItemStartedNotificationParams"}},
	{Method: "item/tool/call", Surface: SurfaceServerRequest, Params: []string{"DynamicToolCallParams"}, Result: []string{"DynamicToolCallResponse"}},
	{Method: "item/tool/requestUserInput", Surface: SurfaceServerRequest, Params: []string{"ToolRequestUserInputParams"}, Result: []string{"ToolRequestUserInputResponse"}},
	{Method: "mcpServer/elicitation/request", Surface: SurfaceServerRequest, Params: []string{"McpServerElicitationRequestParams"}, Result: []string{"McpServerElicitationRequestResponse"}},
	{Method: "serverRequest/resolved", Surface: SurfaceServerNotification, Params: []string{"ServerRequestResolvedNotificationParams"}},
	{Method: "thread/archive", Surface: SurfaceClientRequest, Params: []string{"ThreadArchiveParams"}, Result: []string{"ThreadArchiveResponse"}},
	{Method: "thread/archived", Surface: SurfaceServerNotification, Params: []string{"ThreadArchivedNotification"}},
	{Method: "thread/closed", Surface: SurfaceServerNotification, Params: []string{"ThreadClosedNotification"}},
	{Method: "thread/compact/start", Surface: SurfaceClientRequest, Params: []string{"ThreadCompactStartParams"}, Result: []string{"ThreadCompactStartResponse"}},
	{Method: "thread/compacted", Surface: SurfaceServerNotification, Params: []string{"ThreadCompactedNotificationParams"}},
	{Method: "thread/delete", Surface: SurfaceClientRequest, Params: []string{"ThreadDeleteParams"}, Result: []string{"ThreadDeleteResponse"}},
	{Method: "thread/deleted", Surface: SurfaceServerNotification, Params: []string{"ThreadDeletedNotification"}},
	{Method: "thread/goal/clear", Surface: SurfaceClientRequest, Params: []string{"ThreadGoalClearParams"}, Result: []string{"ThreadGoalClearResponse"}},
	{Method: "thread/goal/cleared", Surface: SurfaceServerNotification, Params: []string{"ThreadGoalClearedNotification"}},
	{Method: "thread/goal/get", Surface: SurfaceClientRequest, Params: []string{"ThreadGoalGetParams"}, Result: []string{"ThreadGoalGetResponse"}},
	{Method: "thread/goal/set", Surface: SurfaceClientRequest, Params: []string{"ThreadGoalSetParams"}, Result: []string{"ThreadGoalSetResponse"}},
	{Method: "thread/goal/updated", Surface: SurfaceServerNotification, Params: []string{"ThreadGoalUpdatedNotification"}},
	{Method: "thread/list", Surface: SurfaceClientRequest, Params: []string{"ThreadListParams"}, Result: []string{"ThreadListResponse"}},
	{Method: "thread/loaded/list", Surface: SurfaceClientRequest, Params: []string{"ThreadLoadedListParams"}, Result: []string{"ThreadLoadedListResponse"}},
	{Method: "thread/memoryMode/set", Surface: SurfaceClientRequest, Params: []string{"ThreadMemoryModeSetParams"}, Result: []string{"ThreadMemoryModeSetResponse"}},
	{Method: "thread/metadata/update", Surface: SurfaceClientRequest, Params: []string{"ThreadMetadataUpdateParams"}, Result: []string{"ThreadMetadataUpdateResponse"}},
	{Method: "thread/name/set", Surface: SurfaceClientRequest, Params: []string{"ThreadSetNameParams"}, Result: []string{"ThreadSetNameResponse"}},
	{Method: "thread/name/updated", Surface: SurfaceServerNotification, Params: []string{"ThreadNameUpdatedNotification"}},
	{Method: "thread/read", Surface: SurfaceClientRequest, Params: []string{"ThreadReadParams"}, Result: []string{"ThreadReadResponse"}},
	{Method: "thread/tokenUsage/updated", Surface: SurfaceServerNotification, Params: []string{"ThreadTokenUsageUpdatedNotificationParams"}},
	{Method: "thread/unarchive", Surface: SurfaceClientRequest, Params: []string{"ThreadUnarchiveParams"}, Result: []string{"ThreadUnarchiveResponse"}},
	{Method: "thread/unarchived", Surface: SurfaceServerNotification, Params: []string{"ThreadUnarchivedNotification"}},
	{Method: "thread/unsubscribe", Surface: SurfaceClientRequest, Params: []string{"ThreadUnsubscribeParams"}, Result: []string{"ThreadUnsubscribeResponse"}},
	{Method: "turn/diff/updated", Surface: SurfaceServerNotification, Params: []string{"TurnDiffUpdatedNotificationParams"}},
}

var itemPayloadBindings = []ItemPayloadBinding{
	{Kind: ItemTypeCommandExecution, Type: "CommandExecutionItem"},
	{Kind: ItemTypeContextCompaction, Type: "ContextCompactionItem"},
	{Kind: ItemTypeDynamicToolCall, Type: "DynamicToolCallItem"},
	{Kind: ItemTypeFileChange, Type: "FileChangeItem"},
	{Kind: ItemTypeMCPToolCall, Type: "MCPToolCallItem"},
}

func WireTypeBindings() []WireTypeBinding {
	out := make([]WireTypeBinding, len(wireTypeBindings))
	for i, binding := range wireTypeBindings {
		out[i] = binding
		out[i].Params = append([]string(nil), binding.Params...)
		out[i].Result = append([]string(nil), binding.Result...)
	}
	return out
}

// ItemPayloadBindings returns a stable copy of the known durable runtime item
// payload mappings.
func ItemPayloadBindings() []ItemPayloadBinding {
	out := make([]ItemPayloadBinding, len(itemPayloadBindings))
	copy(out, itemPayloadBindings)
	return out
}
