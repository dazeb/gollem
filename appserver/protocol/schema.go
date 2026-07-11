package protocol

import (
	"encoding/json"
	"reflect"
)

//go:generate go run ./internal/cmd/schema

// Schema is a JSON Schema document fragment.
type Schema map[string]any

// JSONSchema returns the foundational app-server envelope, method inventory,
// exported wire definitions, and their method and durable-item bindings.
func JSONSchema() Schema {
	defs := foundationalSchemaDefinitions()
	for name, schema := range wireSchemaDefinitions() {
		defs[name] = schema
	}
	return Schema{
		"$schema":                        "https://json-schema.org/draft/2020-12/schema",
		"title":                          "Gollem App Server Protocol",
		"type":                           "object",
		"$defs":                          defs,
		"x-gollem-protocol-version":      ProtocolVersion,
		"x-gollem-schema-version":        SchemaVersion,
		"x-gollem-methods":               Methods(),
		"x-gollem-type-bindings":         WireTypeBindings(),
		"x-gollem-item-payload-bindings": ItemPayloadBindings(),
	}
}

func foundationalSchemaDefinitions() Schema {
	return Schema{
		"RequestID": Schema{
			"oneOf": []any{
				Schema{"type": "string"},
				Schema{"type": "integer"},
			},
		},
		"Error": Schema{
			"type": "object",
			"properties": Schema{
				"code":    Schema{"type": "integer"},
				"message": Schema{"type": "string"},
				"data":    Schema{},
			},
			"required": []any{"code", "message"},
		},
		"Request": Schema{
			"type": "object",
			"properties": Schema{
				"id":     Schema{"$ref": "#/$defs/RequestID"},
				"method": methodEnumSchema(SurfaceClientRequest, SurfaceGollemExtension),
				"params": Schema{},
			},
			"required":             []any{"id", "method"},
			"additionalProperties": false,
		},
		"Notification": Schema{
			"type": "object",
			"properties": Schema{
				"method": methodEnumSchema(SurfaceServerNotification, SurfaceClientNotification),
				"params": Schema{},
			},
			"required":             []any{"method"},
			"additionalProperties": false,
		},
		"Response": Schema{
			"type": "object",
			"properties": Schema{
				"id":     Schema{"$ref": "#/$defs/RequestID"},
				"result": Schema{},
				"error":  Schema{"$ref": "#/$defs/Error"},
			},
			"required":             []any{"id"},
			"additionalProperties": false,
		},
	}
}

func wireSchemaDefinitions() Schema {
	definitions := []wireSchemaDefinition{
		{Name: "AbsolutePathBuf", Type: reflect.TypeFor[AbsolutePathBuf]()},
		{Name: "ActivePermissionProfile", Type: reflect.TypeFor[ActivePermissionProfile]()},
		{Name: "AgentMessageInputContent", Type: reflect.TypeFor[AgentMessageInputContent]()},
		{Name: "AgentPath", Type: reflect.TypeFor[AgentPath]()},
		{Name: "AdditionalFileSystemPermissions", Type: reflect.TypeFor[AdditionalFileSystemPermissions]()},
		{Name: "AdditionalNetworkPermissions", Type: reflect.TypeFor[AdditionalNetworkPermissions]()},
		{Name: "AdditionalPermissionProfile", Type: reflect.TypeFor[AdditionalPermissionProfile]()},
		{Name: "ApprovalRequestBase", Type: reflect.TypeFor[ApprovalRequestBase]()},
		{Name: "ApprovalRespondParams", Type: reflect.TypeFor[ApprovalRespondParams]()},
		{Name: "ApprovalRespondResult", Type: reflect.TypeFor[ApprovalRespondResult]()},
		{Name: "ByteRange", Type: reflect.TypeFor[ByteRange]()},
		{Name: "ClientInfo", Type: reflect.TypeFor[ClientInfo]()},
		{Name: "CollabAgentState", Type: reflect.TypeFor[CollabAgentState]()},
		{Name: "CollabAgentStatus", Type: reflect.TypeFor[CollabAgentStatus]()},
		{Name: "CollabAgentTool", Type: reflect.TypeFor[CollabAgentTool]()},
		{Name: "CollabAgentToolCallStatus", Type: reflect.TypeFor[CollabAgentToolCallStatus]()},
		{Name: "CommandAction", Type: reflect.TypeFor[CommandAction]()},
		{Name: "CommandExecOutputDeltaNotification", Type: reflect.TypeFor[CommandExecOutputDeltaNotification]()},
		{Name: "CommandExecOutputStream", Type: reflect.TypeFor[CommandExecOutputStream]()},
		{Name: "CommandExecResizeParams", Type: reflect.TypeFor[CommandExecResizeParams]()},
		{Name: "CommandExecResizeResponse", Type: reflect.TypeFor[CommandExecResizeResponse]()},
		{Name: "CommandExecTerminalSize", Type: reflect.TypeFor[CommandExecTerminalSize]()},
		{Name: "CommandExecTerminateParams", Type: reflect.TypeFor[CommandExecTerminateParams]()},
		{Name: "CommandExecTerminateResponse", Type: reflect.TypeFor[CommandExecTerminateResponse]()},
		{Name: "CommandExecWriteParams", Type: reflect.TypeFor[CommandExecWriteParams]()},
		{Name: "CommandExecWriteResponse", Type: reflect.TypeFor[CommandExecWriteResponse]()},
		{Name: "CommandExecutionAction", Type: reflect.TypeFor[CommandExecutionAction]()},
		{Name: "CommandExecutionApprovalDecision", Type: reflect.TypeFor[CommandExecutionApprovalDecision]()},
		{Name: "CommandExecutionApprovalRequestParams", Type: reflect.TypeFor[CommandExecutionApprovalRequestParams]()},
		{Name: "CommandExecutionRequestApprovalResponse", Type: reflect.TypeFor[CommandExecutionRequestApprovalResponse]()},
		{Name: "CommandExecutionItem", Type: reflect.TypeFor[CommandExecutionItem]()},
		{Name: "CommandExecutionItemCompletedNotificationParams", Type: reflect.TypeFor[CommandExecutionItemCompletedNotificationParams]()},
		{Name: "CommandExecutionItemStartedNotificationParams", Type: reflect.TypeFor[CommandExecutionItemStartedNotificationParams]()},
		{Name: "CommandExecutionOutputDeltaNotificationParams", Type: reflect.TypeFor[CommandExecutionOutputDeltaNotificationParams]()},
		{Name: "ContentItem", Type: reflect.TypeFor[ContentItem]()},
		{Name: "ContextCompactionItem", Type: reflect.TypeFor[ContextCompactionItem]()},
		{Name: "DaemonShutdownParams", Type: reflect.TypeFor[DaemonShutdownParams]()},
		{Name: "DaemonShutdownState", Type: reflect.TypeFor[DaemonShutdownState]()},
		{Name: "DaemonStartResult", Type: reflect.TypeFor[DaemonStartResult]()},
		{Name: "DaemonStatus", Type: reflect.TypeFor[DaemonStatus]()},
		{Name: "DaemonStopResult", Type: reflect.TypeFor[DaemonStopResult]()},
		{Name: "DaemonVersion", Type: reflect.TypeFor[DaemonVersion]()},
		{Name: "DeprecationNoticeNotification", Type: reflect.TypeFor[DeprecationNoticeNotification]()},
		{Name: "DynamicToolCallContentItem", Type: reflect.TypeFor[DynamicToolCallContentItem]()},
		{Name: "DynamicToolCallOutputContentItem", Type: reflect.TypeFor[DynamicToolCallOutputContentItem]()},
		{Name: "DynamicToolCallParams", Type: reflect.TypeFor[DynamicToolCallParams]()},
		{Name: "DynamicToolCallResponse", Type: reflect.TypeFor[DynamicToolCallResponse]()},
		{Name: "DynamicToolCallItem", Type: reflect.TypeFor[DynamicToolCallItem]()},
		{Name: "DynamicToolCallItemCompletedNotificationParams", Type: reflect.TypeFor[DynamicToolCallItemCompletedNotificationParams]()},
		{Name: "DynamicToolCallItemStartedNotificationParams", Type: reflect.TypeFor[DynamicToolCallItemStartedNotificationParams]()},
		{Name: "ExecPolicyAmendment", Type: reflect.TypeFor[ExecPolicyAmendment]()},
		{Name: "FileChangeApprovalRequestParams", Type: reflect.TypeFor[FileChangeApprovalRequestParams]()},
		{Name: "FileChangeApprovalDecision", Type: reflect.TypeFor[FileChangeApprovalDecision]()},
		{Name: "FileChangeRequestApprovalResponse", Type: reflect.TypeFor[FileChangeRequestApprovalResponse]()},
		{Name: "FileChangeArtifactEvidence", Type: reflect.TypeFor[FileChangeArtifactEvidence]()},
		{Name: "FileChangeItem", Type: reflect.TypeFor[FileChangeItem]()},
		{Name: "FileChangeItemCompletedNotificationParams", Type: reflect.TypeFor[FileChangeItemCompletedNotificationParams]()},
		{Name: "FileChangeItemStartedNotificationParams", Type: reflect.TypeFor[FileChangeItemStartedNotificationParams]()},
		{Name: "FileChangePatchUpdatedNotificationParams", Type: reflect.TypeFor[FileChangePatchUpdatedNotificationParams]()},
		{Name: "FileUpdateChange", Type: reflect.TypeFor[FileUpdateChange]()},
		{Name: "FileSystemAccessMode", Type: reflect.TypeFor[FileSystemAccessMode]()},
		{Name: "FileSystemPath", Type: reflect.TypeFor[FileSystemPath]()},
		{Name: "FileSystemSandboxEntry", Type: reflect.TypeFor[FileSystemSandboxEntry]()},
		{Name: "FileSystemSpecialPath", Type: reflect.TypeFor[FileSystemSpecialPath]()},
		{Name: "FileChangedNotification", Type: reflect.TypeFor[FileChangedNotification]()},
		{Name: "FsChangedNotification", Type: reflect.TypeFor[FsChangedNotification]()},
		{Name: "FsCopyParams", Type: reflect.TypeFor[FsCopyParams]()},
		{Name: "FsCopyResponse", Type: reflect.TypeFor[FsCopyResponse]()},
		{Name: "FsCreateDirectoryParams", Type: reflect.TypeFor[FsCreateDirectoryParams]()},
		{Name: "FsCreateDirectoryResponse", Type: reflect.TypeFor[FsCreateDirectoryResponse]()},
		{Name: "FsGetMetadataParams", Type: reflect.TypeFor[FsGetMetadataParams]()},
		{Name: "FsGetMetadataResponse", Type: reflect.TypeFor[FsGetMetadataResponse]()},
		{Name: "FsReadDirectoryEntry", Type: reflect.TypeFor[FsReadDirectoryEntry]()},
		{Name: "FsReadDirectoryParams", Type: reflect.TypeFor[FsReadDirectoryParams]()},
		{Name: "FsReadDirectoryResponse", Type: reflect.TypeFor[FsReadDirectoryResponse]()},
		{Name: "FsReadFileParams", Type: reflect.TypeFor[FsReadFileParams]()},
		{Name: "FsReadFileResponse", Type: reflect.TypeFor[FsReadFileResponse]()},
		{Name: "FsRemoveParams", Type: reflect.TypeFor[FsRemoveParams]()},
		{Name: "FsRemoveResponse", Type: reflect.TypeFor[FsRemoveResponse]()},
		{Name: "FsUnwatchParams", Type: reflect.TypeFor[FsUnwatchParams]()},
		{Name: "FsUnwatchResponse", Type: reflect.TypeFor[FsUnwatchResponse]()},
		{Name: "FsWatchParams", Type: reflect.TypeFor[FsWatchParams]()},
		{Name: "FsWatchResponse", Type: reflect.TypeFor[FsWatchResponse]()},
		{Name: "FsWriteFileParams", Type: reflect.TypeFor[FsWriteFileParams]()},
		{Name: "FsWriteFileResponse", Type: reflect.TypeFor[FsWriteFileResponse]()},
		{Name: "FunctionCallOutputBody", Type: reflect.TypeFor[FunctionCallOutputBody]()},
		{Name: "FunctionCallOutputContentItem", Type: reflect.TypeFor[FunctionCallOutputContentItem]()},
		{Name: "GrantedPermissionProfile", Type: reflect.TypeFor[GrantedPermissionProfile]()},
		{Name: "HookPromptFragment", Type: reflect.TypeFor[HookPromptFragment]()},
		{Name: "ImageDetail", Type: reflect.TypeFor[ImageDetail]()},
		{Name: "ImplementationInfo", Type: reflect.TypeFor[ImplementationInfo]()},
		{Name: "InitializeCapabilities", Type: reflect.TypeFor[InitializeCapabilities]()},
		{Name: "InitializeParams", Type: reflect.TypeFor[InitializeParams]()},
		{Name: "InitializeResponse", Type: reflect.TypeFor[InitializeResponse]()},
		{Name: "InternalChatMessageMetadataPassthrough", Type: reflect.TypeFor[InternalChatMessageMetadataPassthrough]()},
		{Name: "ItemLifecycleNotificationParams", Type: reflect.TypeFor[ItemLifecycleNotificationParams]()},
		{Name: "LegacyAppPathString", Type: reflect.TypeFor[LegacyAppPathString]()},
		{Name: "LocalShellAction", Type: reflect.TypeFor[LocalShellAction]()},
		{Name: "LocalShellStatus", Type: reflect.TypeFor[LocalShellStatus]()},
		{Name: "MCPContent", Type: reflect.TypeFor[MCPContent]()},
		{Name: "MCPToolCallError", Type: reflect.TypeFor[MCPToolCallError]()},
		{Name: "MCPToolCallItem", Type: reflect.TypeFor[MCPToolCallItem]()},
		{Name: "MCPToolCallItemCompletedNotificationParams", Type: reflect.TypeFor[MCPToolCallItemCompletedNotificationParams]()},
		{Name: "MCPToolCallItemStartedNotificationParams", Type: reflect.TypeFor[MCPToolCallItemStartedNotificationParams]()},
		{Name: "MCPToolCallProgressNotificationParams", Type: reflect.TypeFor[MCPToolCallProgressNotificationParams]()},
		{Name: "MCPToolCallResult", Type: reflect.TypeFor[MCPToolCallResult]()},
		{Name: "McpToolCallAppContext", Type: reflect.TypeFor[McpToolCallAppContext]()},
		{Name: "MemoryCitation", Type: reflect.TypeFor[MemoryCitation]()},
		{Name: "MemoryCitationEntry", Type: reflect.TypeFor[MemoryCitationEntry]()},
		{Name: "MessagePhase", Type: reflect.TypeFor[MessagePhase]()},
		{Name: "McpElicitationArrayType", Type: reflect.TypeFor[McpElicitationArrayType]()},
		{Name: "McpElicitationBooleanSchema", Type: reflect.TypeFor[McpElicitationBooleanSchema]()},
		{Name: "McpElicitationBooleanType", Type: reflect.TypeFor[McpElicitationBooleanType]()},
		{Name: "McpElicitationConstOption", Type: reflect.TypeFor[McpElicitationConstOption]()},
		{Name: "McpElicitationEnumSchema", Type: reflect.TypeFor[McpElicitationEnumSchema]()},
		{Name: "McpElicitationLegacyTitledEnumSchema", Type: reflect.TypeFor[McpElicitationLegacyTitledEnumSchema]()},
		{Name: "McpElicitationMultiSelectEnumSchema", Type: reflect.TypeFor[McpElicitationMultiSelectEnumSchema]()},
		{Name: "McpElicitationNumberSchema", Type: reflect.TypeFor[McpElicitationNumberSchema]()},
		{Name: "McpElicitationNumberType", Type: reflect.TypeFor[McpElicitationNumberType]()},
		{Name: "McpElicitationObjectType", Type: reflect.TypeFor[McpElicitationObjectType]()},
		{Name: "McpElicitationPrimitiveSchema", Type: reflect.TypeFor[McpElicitationPrimitiveSchema]()},
		{Name: "McpElicitationSchema", Type: reflect.TypeFor[McpElicitationSchema]()},
		{Name: "McpElicitationSingleSelectEnumSchema", Type: reflect.TypeFor[McpElicitationSingleSelectEnumSchema]()},
		{Name: "McpElicitationStringFormat", Type: reflect.TypeFor[McpElicitationStringFormat]()},
		{Name: "McpElicitationStringSchema", Type: reflect.TypeFor[McpElicitationStringSchema]()},
		{Name: "McpElicitationStringType", Type: reflect.TypeFor[McpElicitationStringType]()},
		{Name: "McpElicitationTitledEnumItems", Type: reflect.TypeFor[McpElicitationTitledEnumItems]()},
		{Name: "McpElicitationTitledMultiSelectEnumSchema", Type: reflect.TypeFor[McpElicitationTitledMultiSelectEnumSchema]()},
		{Name: "McpElicitationTitledSingleSelectEnumSchema", Type: reflect.TypeFor[McpElicitationTitledSingleSelectEnumSchema]()},
		{Name: "McpElicitationUntitledEnumItems", Type: reflect.TypeFor[McpElicitationUntitledEnumItems]()},
		{Name: "McpElicitationUntitledMultiSelectEnumSchema", Type: reflect.TypeFor[McpElicitationUntitledMultiSelectEnumSchema]()},
		{Name: "McpElicitationUntitledSingleSelectEnumSchema", Type: reflect.TypeFor[McpElicitationUntitledSingleSelectEnumSchema]()},
		{Name: "McpServerElicitationAction", Type: reflect.TypeFor[McpServerElicitationAction]()},
		{Name: "McpServerElicitationRequestParams", Type: reflect.TypeFor[McpServerElicitationRequestParams]()},
		{Name: "McpServerElicitationRequestResponse", Type: reflect.TypeFor[McpServerElicitationRequestResponse]()},
		{Name: "MethodInfo", Type: reflect.TypeFor[MethodInfo]()},
		{Name: "MethodState", Type: reflect.TypeFor[MethodState]()},
		{Name: "NetworkPolicyAmendment", Type: reflect.TypeFor[NetworkPolicyAmendment]()},
		{Name: "NetworkPolicyRuleAction", Type: reflect.TypeFor[NetworkPolicyRuleAction]()},
		{Name: "PatchApplyStatus", Type: reflect.TypeFor[PatchApplyStatus]()},
		{Name: "PatchChangeKind", Type: reflect.TypeFor[PatchChangeKind]()},
		{Name: "PermissionGrantScope", Type: reflect.TypeFor[PermissionGrantScope]()},
		{Name: "PermissionsApprovalRequestParams", Type: reflect.TypeFor[PermissionsApprovalRequestParams]()},
		{Name: "PermissionsRequestApprovalParams", Type: reflect.TypeFor[PermissionsRequestApprovalParams]()},
		{Name: "PermissionsRequestApprovalResponse", Type: reflect.TypeFor[PermissionsRequestApprovalResponse]()},
		{Name: "RequestPermissionProfile", Type: reflect.TypeFor[RequestPermissionProfile]()},
		{Name: "ReasoningEffort", Type: reflect.TypeFor[ReasoningEffort]()},
		{Name: "ReasoningItemContent", Type: reflect.TypeFor[ReasoningItemContent]()},
		{Name: "ReasoningItemReasoningSummary", Type: reflect.TypeFor[ReasoningItemReasoningSummary]()},
		{Name: "ResponseItem", Type: reflect.TypeFor[ResponseItem]()},
		{Name: "ResponsesApiWebSearchAction", Type: reflect.TypeFor[ResponsesApiWebSearchAction]()},
		{Name: "ServerRequestResolvedNotificationParams", Type: reflect.TypeFor[ServerRequestResolvedNotificationParams]()},
		{Name: "ServerCapabilities", Type: reflect.TypeFor[ServerCapabilities]()},
		{Name: "Surface", Type: reflect.TypeFor[Surface]()},
		{Name: "SortDirection", Type: reflect.TypeFor[SortDirection]()},
		{Name: "SubAgentActivityKind", Type: reflect.TypeFor[SubAgentActivityKind]()},
		{Name: "ThreadArchiveParams", Type: reflect.TypeFor[ThreadArchiveParams]()},
		{Name: "ThreadArchiveResponse", Type: reflect.TypeFor[ThreadArchiveResponse]()},
		{Name: "ThreadArchivedNotification", Type: reflect.TypeFor[ThreadArchivedNotification]()},
		{Name: "ThreadClosedNotification", Type: reflect.TypeFor[ThreadClosedNotification]()},
		{Name: "ThreadCompactStartParams", Type: reflect.TypeFor[ThreadCompactStartParams]()},
		{Name: "ThreadCompactStartResponse", Type: reflect.TypeFor[ThreadCompactStartResponse]()},
		{Name: "ThreadCompactedNotificationParams", Type: reflect.TypeFor[ThreadCompactedNotificationParams]()},
		{Name: "ThreadDeleteParams", Type: reflect.TypeFor[ThreadDeleteParams]()},
		{Name: "ThreadDeleteResponse", Type: reflect.TypeFor[ThreadDeleteResponse]()},
		{Name: "ThreadDeletedNotification", Type: reflect.TypeFor[ThreadDeletedNotification]()},
		{Name: "ThreadGoal", Type: reflect.TypeFor[ThreadGoal]()},
		{Name: "ThreadGoalClearParams", Type: reflect.TypeFor[ThreadGoalClearParams]()},
		{Name: "ThreadGoalClearResponse", Type: reflect.TypeFor[ThreadGoalClearResponse]()},
		{Name: "ThreadGoalClearedNotification", Type: reflect.TypeFor[ThreadGoalClearedNotification]()},
		{Name: "ThreadGoalGetParams", Type: reflect.TypeFor[ThreadGoalGetParams]()},
		{Name: "ThreadGoalGetResponse", Type: reflect.TypeFor[ThreadGoalGetResponse]()},
		{Name: "ThreadGoalSetParams", Type: reflect.TypeFor[ThreadGoalSetParams]()},
		{Name: "ThreadGoalSetResponse", Type: reflect.TypeFor[ThreadGoalSetResponse]()},
		{Name: "ThreadGoalStatus", Type: reflect.TypeFor[ThreadGoalStatus]()},
		{Name: "ThreadGoalUpdatedNotification", Type: reflect.TypeFor[ThreadGoalUpdatedNotification]()},
		{Name: "ThreadLifecycleStatus", Type: reflect.TypeFor[ThreadLifecycleStatus]()},
		{Name: "ThreadListCwdFilter", Type: reflect.TypeFor[ThreadListCwdFilter]()},
		{Name: "ThreadListParams", Type: reflect.TypeFor[ThreadListParams]()},
		{Name: "ThreadListResponse", Type: reflect.TypeFor[ThreadListResponse]()},
		{Name: "ThreadLoadedListParams", Type: reflect.TypeFor[ThreadLoadedListParams]()},
		{Name: "ThreadLoadedListResponse", Type: reflect.TypeFor[ThreadLoadedListResponse]()},
		{Name: "ThreadMemoryMode", Type: reflect.TypeFor[ThreadMemoryMode]()},
		{Name: "ThreadMemoryModeSetParams", Type: reflect.TypeFor[ThreadMemoryModeSetParams]()},
		{Name: "ThreadMemoryModeSetResponse", Type: reflect.TypeFor[ThreadMemoryModeSetResponse]()},
		{Name: "ThreadMetadataGitInfoUpdateParams", Type: reflect.TypeFor[ThreadMetadataGitInfoUpdateParams]()},
		{Name: "ThreadMetadataUpdateParams", Type: reflect.TypeFor[ThreadMetadataUpdateParams]()},
		{Name: "ThreadMetadataUpdateResponse", Type: reflect.TypeFor[ThreadMetadataUpdateResponse]()},
		{Name: "ThreadNameUpdatedNotification", Type: reflect.TypeFor[ThreadNameUpdatedNotification]()},
		{Name: "ThreadReadParams", Type: reflect.TypeFor[ThreadReadParams]()},
		{Name: "ThreadReadResponse", Type: reflect.TypeFor[ThreadReadResponse]()},
		{Name: "ThreadRecord", Type: reflect.TypeFor[ThreadRecord]()},
		{Name: "ThreadSetNameParams", Type: reflect.TypeFor[ThreadSetNameParams]()},
		{Name: "ThreadSetNameResponse", Type: reflect.TypeFor[ThreadSetNameResponse]()},
		{Name: "ThreadSortKey", Type: reflect.TypeFor[ThreadSortKey]()},
		{Name: "ThreadSourceKind", Type: reflect.TypeFor[ThreadSourceKind]()},
		{Name: "ThreadTokenUsageUpdatedNotificationParams", Type: reflect.TypeFor[ThreadTokenUsageUpdatedNotificationParams]()},
		{Name: "ThreadUnarchiveParams", Type: reflect.TypeFor[ThreadUnarchiveParams]()},
		{Name: "ThreadUnarchiveResponse", Type: reflect.TypeFor[ThreadUnarchiveResponse]()},
		{Name: "ThreadUnarchivedNotification", Type: reflect.TypeFor[ThreadUnarchivedNotification]()},
		{Name: "ThreadUnsubscribeParams", Type: reflect.TypeFor[ThreadUnsubscribeParams]()},
		{Name: "ThreadUnsubscribeResponse", Type: reflect.TypeFor[ThreadUnsubscribeResponse]()},
		{Name: "ThreadUnsubscribeStatus", Type: reflect.TypeFor[ThreadUnsubscribeStatus]()},
		{Name: "TextElement", Type: reflect.TypeFor[TextElement]()},
		{Name: "TimelineItem", Type: reflect.TypeFor[TimelineItem]()},
		{Name: "TokenUsage", Type: reflect.TypeFor[TokenUsage]()},
		{Name: "TokenUsageBreakdown", Type: reflect.TypeFor[TokenUsageBreakdown]()},
		{Name: "ToolPayloadSummary", Type: reflect.TypeFor[ToolPayloadSummary]()},
		{Name: "ToolRequestUserInputAnswer", Type: reflect.TypeFor[ToolRequestUserInputAnswer]()},
		{Name: "ToolRequestUserInputOption", Type: reflect.TypeFor[ToolRequestUserInputOption]()},
		{Name: "ToolRequestUserInputParams", Type: reflect.TypeFor[ToolRequestUserInputParams]()},
		{Name: "ToolRequestUserInputQuestion", Type: reflect.TypeFor[ToolRequestUserInputQuestion]()},
		{Name: "ToolRequestUserInputResponse", Type: reflect.TypeFor[ToolRequestUserInputResponse]()},
		{Name: "TurnDiffUpdatedNotificationParams", Type: reflect.TypeFor[TurnDiffUpdatedNotificationParams]()},
		{Name: "TurnLifecycleStatus", Type: reflect.TypeFor[TurnLifecycleStatus]()},
		{Name: "TurnRecord", Type: reflect.TypeFor[TurnRecord]()},
		{Name: "UserInput", Type: reflect.TypeFor[UserInput]()},
		{Name: "WebSearchAction", Type: reflect.TypeFor[WebSearchAction]()},
		// Register exact public names after their aliases so nested schemas refer
		// to the public names. JSON and TypeScript output remain key-sorted.
		{Name: "ContextCompactedNotification", Type: reflect.TypeFor[ContextCompactedNotification]()},
		{Name: "RequestId", Type: reflect.TypeFor[RequestId]()},
		{Name: "CommandExecutionOutputDeltaNotification", Type: reflect.TypeFor[CommandExecutionOutputDeltaNotification]()},
		{Name: "CommandExecutionStatus", Type: reflect.TypeFor[CommandExecutionStatus]()},
		{Name: "DynamicToolCallStatus", Type: reflect.TypeFor[DynamicToolCallStatus]()},
		{Name: "FileChangePatchUpdatedNotification", Type: reflect.TypeFor[FileChangePatchUpdatedNotification]()},
		{Name: "McpToolCallError", Type: reflect.TypeFor[McpToolCallError]()},
		{Name: "McpToolCallProgressNotification", Type: reflect.TypeFor[McpToolCallProgressNotification]()},
		{Name: "McpToolCallStatus", Type: reflect.TypeFor[McpToolCallStatus]()},
		{Name: "ServerRequestResolvedNotification", Type: reflect.TypeFor[ServerRequestResolvedNotification]()},
		{Name: "ThreadTokenUsage", Type: reflect.TypeFor[ThreadTokenUsage]()},
		{Name: "ThreadTokenUsageUpdatedNotification", Type: reflect.TypeFor[ThreadTokenUsageUpdatedNotification]()},
		{Name: "TurnDiffUpdatedNotification", Type: reflect.TypeFor[TurnDiffUpdatedNotification]()},
	}
	names := make(map[reflect.Type]string, len(definitions))
	for _, definition := range definitions {
		names[definition.Type] = definition.Name
	}
	schemas := make(Schema, len(definitions))
	for _, definition := range definitions {
		schemas[definition.Name] = schemaForDefinition(definition.Type, names)
	}
	schemas["MethodState"] = stringEnumSchema(
		string(MethodImplemented),
		string(MethodBlocked),
		string(MethodDeferredStub),
		string(MethodRenamedEquivalent),
		string(MethodNotApplicable),
	)
	schemas["Surface"] = stringEnumSchema(
		string(SurfaceClientRequest),
		string(SurfaceServerNotification),
		string(SurfaceServerRequest),
		string(SurfaceClientNotification),
		string(SurfaceGollemExtension),
	)
	schemas["SortDirection"] = stringEnumSchema(string(SortDirectionAsc), string(SortDirectionDesc))
	schemas["AgentPath"] = Schema{"type": "string"}
	schemas["ReasoningEffort"] = Schema{"type": "string", "minLength": 1}
	schemas["CollabAgentStatus"] = stringEnumSchema(
		string(CollabAgentStatusPendingInit), string(CollabAgentStatusRunning),
		string(CollabAgentStatusInterrupted), string(CollabAgentStatusCompleted),
		string(CollabAgentStatusErrored), string(CollabAgentStatusShutdown),
		string(CollabAgentStatusNotFound),
	)
	schemas["CollabAgentTool"] = stringEnumSchema(
		string(CollabAgentToolSpawnAgent), string(CollabAgentToolSendInput),
		string(CollabAgentToolResumeAgent), string(CollabAgentToolWait),
		string(CollabAgentToolCloseAgent),
	)
	schemas["CollabAgentToolCallStatus"] = stringEnumSchema(
		string(CollabAgentToolCallStatusInProgress), string(CollabAgentToolCallStatusCompleted),
		string(CollabAgentToolCallStatusFailed),
	)
	schemas["SubAgentActivityKind"] = stringEnumSchema(
		string(SubAgentActivityStarted), string(SubAgentActivityInteracted),
		string(SubAgentActivityInterrupted),
	)
	schemas["CollabAgentState"] = collabAgentStateSchema()
	schemas["AgentMessageInputContent"] = rawResponseContentSchema(agentMessageInputContentVariants)
	schemas["ReasoningItemContent"] = rawResponseContentSchema(reasoningItemContentVariants)
	schemas["ReasoningItemReasoningSummary"] = rawResponseContentSchema(reasoningItemSummaryVariants)
	schemas["ResponsesApiWebSearchAction"] = responsesAPIWebSearchActionSchema()
	schemas["ContentItem"] = contentItemSchema(contentItemVariants)
	schemas["FunctionCallOutputContentItem"] = contentItemSchema(functionCallOutputContentItemVariants)
	schemas["FunctionCallOutputBody"] = functionCallOutputBodySchema()
	schemas["InternalChatMessageMetadataPassthrough"] = internalChatMessageMetadataSchema()
	schemas["LocalShellStatus"] = stringEnumSchema(
		string(LocalShellStatusCompleted), string(LocalShellStatusInProgress), string(LocalShellStatusIncomplete),
	)
	schemas["LocalShellAction"] = localShellActionSchema()
	schemas["ResponseItem"] = responseItemSchema()
	setSchemaIntegerMinimum(schemas["ByteRange"].(Schema), 0, "start", "end")
	schemas["ImageDetail"] = stringEnumSchema(
		string(ImageDetailAuto), string(ImageDetailLow), string(ImageDetailHigh), string(ImageDetailOriginal),
	)
	setSchemaIntegerMinimum(schemas["MemoryCitationEntry"].(Schema), 0, "lineStart", "lineEnd")
	schemas["MessagePhase"] = stringEnumSchema(
		string(MessagePhaseCommentary), string(MessagePhaseFinalAnswer),
	)
	schemas["UserInput"] = userInputSchema()
	schemas["ThreadLifecycleStatus"] = stringEnumSchema(
		string(ThreadLifecycleActive), string(ThreadLifecycleArchived), string(ThreadLifecycleDeleted),
	)
	schemas["ThreadGoalStatus"] = stringEnumSchema(
		string(ThreadGoalActive), string(ThreadGoalPaused), string(ThreadGoalBlocked),
		string(ThreadGoalUsageLimited), string(ThreadGoalBudgetLimited), string(ThreadGoalComplete),
	)
	schemas["ThreadMemoryMode"] = stringEnumSchema(
		string(ThreadMemoryModeEnabled), string(ThreadMemoryModeDisabled),
	)
	schemas["CommandExecOutputStream"] = stringEnumSchema(
		string(CommandExecOutputStdout), string(CommandExecOutputStderr),
	)
	schemas["CommandExecutionApprovalDecision"] = commandExecutionApprovalDecisionSchema()
	schemas["CommandAction"] = commandActionSchema()
	schemas["WebSearchAction"] = webSearchActionSchema()
	schemas["AbsolutePathBuf"] = Schema{
		"type":        "string",
		"description": "An absolute, lexically normalized local path.",
	}
	schemas["FileSystemAccessMode"] = stringEnumSchema(
		string(FileSystemAccessRead), string(FileSystemAccessWrite), string(FileSystemAccessDeny),
	)
	schemas["FileSystemPath"] = fileSystemPathSchema()
	schemas["FileSystemSpecialPath"] = fileSystemSpecialPathSchema()
	schemas["LegacyAppPathString"] = Schema{"type": "string"}
	schemas["PermissionGrantScope"] = stringEnumSchema(
		string(PermissionGrantTurn), string(PermissionGrantSession),
	)
	for alias, canonical := range map[string]string{
		"CommandExecutionOutputDeltaNotificationParams": "CommandExecutionOutputDeltaNotification",
		"FileChangePatchUpdatedNotificationParams":      "FileChangePatchUpdatedNotification",
		"MCPToolCallError":                              "McpToolCallError",
		"MCPToolCallProgressNotificationParams":         "McpToolCallProgressNotification",
		"ServerRequestResolvedNotificationParams":       "ServerRequestResolvedNotification",
		"ThreadCompactedNotificationParams":             "ContextCompactedNotification",
		"ThreadTokenUsageUpdatedNotificationParams":     "ThreadTokenUsageUpdatedNotification",
		"TokenUsage":                        "ThreadTokenUsage",
		"TurnDiffUpdatedNotificationParams": "TurnDiffUpdatedNotification",
	} {
		schemas[alias] = Schema{"$ref": "#/$defs/" + canonical}
	}
	schemas["RequestId"] = Schema{"$ref": "#/$defs/RequestID"}
	schemas["CommandExecutionStatus"] = stringEnumSchema(
		string(CommandExecutionStatusInProgress), string(CommandExecutionStatusCompleted),
		string(CommandExecutionStatusFailed), string(CommandExecutionStatusDeclined),
	)
	schemas["DynamicToolCallStatus"] = stringEnumSchema(
		string(DynamicToolCallStatusInProgress), string(DynamicToolCallStatusCompleted),
		string(DynamicToolCallStatusFailed),
	)
	schemas["McpToolCallStatus"] = stringEnumSchema(
		string(McpToolCallStatusInProgress), string(McpToolCallStatusCompleted),
		string(McpToolCallStatusFailed),
	)
	for definition, status := range map[string]string{
		"CommandExecutionItem": "CommandExecutionStatus",
		"DynamicToolCallItem":  "DynamicToolCallStatus",
		"MCPToolCallItem":      "McpToolCallStatus",
	} {
		properties := schemas[definition].(Schema)["properties"].(Schema)
		properties["status"] = Schema{"$ref": "#/$defs/" + status}
	}
	setSchemaIntegerMinimum(schemas["AdditionalFileSystemPermissions"].(Schema), 1, "globScanMaxDepth")
	if response, ok := schemas["PermissionsRequestApprovalResponse"].(Schema); ok {
		if properties, ok := response["properties"].(Schema); ok {
			if scope, ok := properties["scope"].(Schema); ok {
				scope["default"] = string(PermissionGrantTurn)
			}
		}
	}
	schemas["FileChangeApprovalDecision"] = stringEnumSchema(
		string(FileChangeApprovalAccept),
		string(FileChangeApprovalAcceptForSession),
		string(FileChangeApprovalDecline),
		string(FileChangeApprovalCancel),
	)
	schemas["PatchApplyStatus"] = stringEnumSchema(
		string(PatchApplyStatusInProgress), string(PatchApplyStatusCompleted),
		string(PatchApplyStatusFailed), string(PatchApplyStatusDeclined),
	)
	schemas["PatchChangeKind"] = patchChangeKindSchema()
	schemas["DynamicToolCallOutputContentItem"] = dynamicToolCallOutputContentItemSchema()
	schemas["McpElicitationArrayType"] = stringEnumSchema("array")
	schemas["McpElicitationBooleanType"] = stringEnumSchema("boolean")
	schemas["McpElicitationEnumSchema"] = schemaRefUnion(
		"McpElicitationSingleSelectEnumSchema",
		"McpElicitationMultiSelectEnumSchema",
		"McpElicitationLegacyTitledEnumSchema",
	)
	schemas["McpElicitationMultiSelectEnumSchema"] = schemaRefUnion(
		"McpElicitationUntitledMultiSelectEnumSchema",
		"McpElicitationTitledMultiSelectEnumSchema",
	)
	schemas["McpElicitationNumberType"] = stringEnumSchema("number", "integer")
	schemas["McpElicitationObjectType"] = stringEnumSchema("object")
	schemas["McpElicitationPrimitiveSchema"] = schemaRefUnion(
		"McpElicitationEnumSchema",
		"McpElicitationStringSchema",
		"McpElicitationNumberSchema",
		"McpElicitationBooleanSchema",
	)
	schemas["McpElicitationSingleSelectEnumSchema"] = schemaRefUnion(
		"McpElicitationUntitledSingleSelectEnumSchema",
		"McpElicitationTitledSingleSelectEnumSchema",
	)
	schemas["McpElicitationStringFormat"] = stringEnumSchema("email", "uri", "date", "date-time")
	schemas["McpElicitationStringType"] = stringEnumSchema("string")
	setSchemaIntegerMinimum(schemas["McpElicitationStringSchema"].(Schema), 0, "minLength", "maxLength")
	setSchemaIntegerMinimum(schemas["McpElicitationTitledMultiSelectEnumSchema"].(Schema), 0, "minItems", "maxItems")
	setSchemaIntegerMinimum(schemas["McpElicitationUntitledMultiSelectEnumSchema"].(Schema), 0, "minItems", "maxItems")
	schemas["McpServerElicitationAction"] = stringEnumSchema("accept", "decline", "cancel")
	schemas["McpServerElicitationRequestParams"] = mcpServerElicitationRequestParamsSchema()
	schemas["NetworkPolicyRuleAction"] = stringEnumSchema(
		string(NetworkPolicyRuleAllow), string(NetworkPolicyRuleDeny),
	)
	schemas["CommandExecWriteParams"] = schemaWithRequiredFieldAlternatives(
		schemas["CommandExecWriteParams"].(Schema),
		[]string{"processId"},
		[]string{"id"},
	)
	schemas["CommandExecTerminateParams"] = schemaWithRequiredFieldAlternatives(
		schemas["CommandExecTerminateParams"].(Schema),
		[]string{"processId"},
		[]string{"id"},
	)
	schemas["CommandExecResizeParams"] = schemaWithRequiredFieldAlternatives(
		schemas["CommandExecResizeParams"].(Schema),
		[]string{"processId", "size"},
		[]string{"processId", "cols", "rows"},
		[]string{"id", "cols", "rows"},
	)
	for _, name := range []string{"ThreadGoalSetParams", "ThreadGoalGetParams", "ThreadGoalClearParams"} {
		schemas[name] = schemaWithRequiredIDAlternative(schemas[name].(Schema))
	}
	schemas["ThreadMetadataUpdateParams"] = schemaWithRequiredIDAlternative(
		schemas["ThreadMetadataUpdateParams"].(Schema),
	)
	schemas["ThreadMemoryModeSetParams"] = schemaWithRequiredFieldAlternatives(
		schemas["ThreadMemoryModeSetParams"].(Schema),
		[]string{"threadId", "mode"},
		[]string{"id", "mode"},
		[]string{"threadId", "memoryMode"},
		[]string{"id", "memoryMode"},
	)
	schemas["ThreadListCwdFilter"] = Schema{"oneOf": []any{
		Schema{"type": "string"},
		Schema{"type": "array", "items": Schema{"type": "string"}},
	}}
	schemas["ThreadSortKey"] = stringEnumSchema(
		string(ThreadSortCreatedAt), string(ThreadSortUpdatedAt), string(ThreadSortRecencyAt),
	)
	schemas["ThreadSourceKind"] = stringEnumSchema(
		string(ThreadSourceCLI), string(ThreadSourceVSCode), string(ThreadSourceExec),
		string(ThreadSourceAppServer), string(ThreadSourceSubAgent), string(ThreadSourceSubAgentReview),
		string(ThreadSourceSubAgentCompact), string(ThreadSourceSubAgentSpawn),
		string(ThreadSourceSubAgentOther), string(ThreadSourceUnknown),
	)
	schemas["ThreadUnsubscribeStatus"] = stringEnumSchema(
		string(ThreadUnsubscribeNotLoaded), string(ThreadUnsubscribeNotSubscribed),
		string(ThreadUnsubscribeUnsubscribed),
	)
	schemas["TurnLifecycleStatus"] = stringEnumSchema(
		string(TurnLifecycleQueued), string(TurnLifecycleRunning), string(TurnLifecycleCompleted),
		string(TurnLifecycleFailed), string(TurnLifecycleInterrupted),
	)
	return schemas
}

func commandActionSchema() Schema {
	return Schema{"oneOf": []any{
		commandActionVariantSchema("read", []string{"command", "name", "path"}, Schema{
			"command": Schema{"type": "string"},
			"name":    Schema{"type": "string"},
			"path":    Schema{"$ref": "#/$defs/AbsolutePathBuf"},
		}),
		commandActionVariantSchema("listFiles", []string{"command", "path"}, Schema{
			"command": Schema{"type": "string"},
			"path":    nullableStringSchema(),
		}),
		commandActionVariantSchema("search", []string{"command", "query", "path"}, Schema{
			"command": Schema{"type": "string"},
			"query":   nullableStringSchema(),
			"path":    nullableStringSchema(),
		}),
		commandActionVariantSchema("unknown", []string{"command"}, Schema{
			"command": Schema{"type": "string"},
		}),
	}}
}

func collabAgentStateSchema() Schema {
	return Schema{
		"type": "object",
		"properties": Schema{
			"status":  Schema{"$ref": "#/$defs/CollabAgentStatus"},
			"message": nullableStringSchema(),
		},
		"required":             []string{"status", "message"},
		"additionalProperties": false,
	}
}

func rawResponseContentSchema(variants []rawResponseContentVariant) Schema {
	oneOf := make([]any, 0, len(variants))
	for _, variant := range variants {
		oneOf = append(oneOf, rawResponseContentVariantSchema(variant))
	}
	return Schema{"oneOf": oneOf}
}

func rawResponseContentVariantSchema(variant rawResponseContentVariant) Schema {
	return Schema{
		"type": "object",
		"properties": Schema{
			"type":        Schema{"type": "string", "enum": []any{variant.contentType}},
			variant.field: Schema{"type": "string"},
		},
		"required":             []string{"type", variant.field},
		"additionalProperties": false,
	}
}

func responsesAPIWebSearchActionSchema() Schema {
	return Schema{"oneOf": []any{
		responsesAPIWebSearchActionVariantSchema("search", Schema{
			"query": Schema{"type": "string"},
			"queries": Schema{
				"type": "array", "items": Schema{"type": "string"},
			},
		}),
		responsesAPIWebSearchActionVariantSchema("open_page", Schema{
			"url": Schema{"type": "string"},
		}),
		responsesAPIWebSearchActionVariantSchema("find_in_page", Schema{
			"url":     Schema{"type": "string"},
			"pattern": Schema{"type": "string"},
		}),
		responsesAPIWebSearchActionVariantSchema("other", nil),
	}}
}

func responsesAPIWebSearchActionVariantSchema(actionType string, fields Schema) Schema {
	properties := Schema{
		"type": Schema{"type": "string", "enum": []any{actionType}},
	}
	for name, schema := range fields {
		properties[name] = schema
	}
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"type"},
		"additionalProperties": false,
	}
}

func contentItemSchema(variants []contentItemVariant) Schema {
	items := make([]any, 0, len(variants))
	for _, variant := range variants {
		properties := Schema{
			"type":        Schema{"type": "string", "enum": []any{variant.itemType}},
			variant.field: Schema{"type": "string"},
		}
		if variant.optionalDetail {
			properties["detail"] = Schema{"$ref": "#/$defs/ImageDetail"}
		}
		items = append(items, Schema{
			"type":                 "object",
			"properties":           properties,
			"required":             []string{"type", variant.field},
			"additionalProperties": false,
		})
	}
	return Schema{"oneOf": items}
}

func functionCallOutputBodySchema() Schema {
	return Schema{"anyOf": []any{
		Schema{"type": "string"},
		Schema{"type": "array", "items": Schema{"$ref": "#/$defs/FunctionCallOutputContentItem"}},
	}}
}

func internalChatMessageMetadataSchema() Schema {
	return Schema{
		"type": "object",
		"properties": Schema{
			"turn_id": Schema{"type": "string"},
		},
		"additionalProperties": false,
	}
}

func localShellActionSchema() Schema {
	return Schema{"oneOf": []any{Schema{
		"type": "object",
		"properties": Schema{
			"type":              Schema{"type": "string", "enum": []any{"exec"}},
			"command":           Schema{"type": "array", "items": Schema{"type": "string"}},
			"timeout_ms":        nullableUnsignedIntegerSchema(),
			"working_directory": nullableStringSchema(),
			"env": Schema{"anyOf": []any{
				Schema{"type": "object", "additionalProperties": Schema{"type": "string"}},
				Schema{"type": "null"},
			}},
			"user": nullableStringSchema(),
		},
		"required":             []string{"type", "command", "timeout_ms", "working_directory", "env", "user"},
		"additionalProperties": false,
	}}}
}

func nullableUnsignedIntegerSchema() Schema {
	return Schema{"anyOf": []any{Schema{"type": "integer", "minimum": 0}, Schema{"type": "null"}}}
}

func responseItemSchema() Schema {
	return Schema{"oneOf": []any{
		responseItemVariantSchema("message", []string{"role", "content"}, Schema{
			"role":    Schema{"type": "string"},
			"content": Schema{"type": "array", "items": Schema{"$ref": "#/$defs/ContentItem"}},
			"phase":   Schema{"$ref": "#/$defs/MessagePhase"},
		}, true),
		responseItemVariantSchema("agent_message", []string{"author", "recipient", "content"}, Schema{
			"author":    Schema{"type": "string"},
			"recipient": Schema{"type": "string"},
			"content":   Schema{"type": "array", "items": Schema{"$ref": "#/$defs/AgentMessageInputContent"}},
		}, true),
		responseItemVariantSchema("reasoning", []string{"summary", "encrypted_content"}, Schema{
			"summary":           Schema{"type": "array", "items": Schema{"$ref": "#/$defs/ReasoningItemReasoningSummary"}},
			"content":           Schema{"type": "array", "items": Schema{"$ref": "#/$defs/ReasoningItemContent"}},
			"encrypted_content": nullableStringSchema(),
		}, true),
		responseItemVariantSchema("local_shell_call", []string{"call_id", "status", "action"}, Schema{
			"call_id": nullableStringSchema(),
			"status":  Schema{"$ref": "#/$defs/LocalShellStatus"},
			"action":  Schema{"$ref": "#/$defs/LocalShellAction"},
		}, true),
		responseItemVariantSchema("function_call", []string{"name", "arguments", "call_id"}, Schema{
			"name":      Schema{"type": "string"},
			"namespace": Schema{"type": "string"},
			"arguments": Schema{"type": "string"},
			"call_id":   Schema{"type": "string"},
		}, true),
		responseItemVariantSchema("tool_search_call", []string{"call_id", "execution", "arguments"}, Schema{
			"call_id":   nullableStringSchema(),
			"status":    Schema{"type": "string"},
			"execution": Schema{"type": "string"},
			"arguments": Schema{},
		}, true),
		responseItemVariantSchema("function_call_output", []string{"call_id", "output"}, Schema{
			"call_id": Schema{"type": "string"},
			"output":  Schema{"$ref": "#/$defs/FunctionCallOutputBody"},
		}, true),
		responseItemVariantSchema("custom_tool_call", []string{"call_id", "name", "input"}, Schema{
			"status":    Schema{"type": "string"},
			"call_id":   Schema{"type": "string"},
			"name":      Schema{"type": "string"},
			"namespace": Schema{"type": "string"},
			"input":     Schema{"type": "string"},
		}, true),
		responseItemVariantSchema("custom_tool_call_output", []string{"call_id", "output"}, Schema{
			"call_id": Schema{"type": "string"},
			"name":    Schema{"type": "string"},
			"output":  Schema{"$ref": "#/$defs/FunctionCallOutputBody"},
		}, true),
		responseItemVariantSchema("tool_search_output", []string{"call_id", "status", "execution", "tools"}, Schema{
			"call_id":   nullableStringSchema(),
			"status":    Schema{"type": "string"},
			"execution": Schema{"type": "string"},
			"tools":     Schema{"type": "array", "items": Schema{}},
		}, true),
		responseItemVariantSchema("web_search_call", nil, Schema{
			"status": Schema{"type": "string"},
			"action": Schema{"$ref": "#/$defs/WebSearchAction"},
		}, true),
		responseItemVariantSchema("image_generation_call", []string{"status", "result"}, Schema{
			"status":         Schema{"type": "string"},
			"revised_prompt": Schema{"type": "string"},
			"result":         Schema{"type": "string"},
		}, true),
		responseItemVariantSchema("compaction", []string{"encrypted_content"}, Schema{
			"encrypted_content": Schema{"type": "string"},
		}, true),
		responseItemVariantSchema("compaction_trigger", nil, nil, false),
		responseItemVariantSchema("context_compaction", nil, Schema{
			"encrypted_content": Schema{"type": "string"},
		}, true),
		responseItemVariantSchema("other", nil, nil, false),
	}}
}

func responseItemVariantSchema(itemType string, requiredFields []string, fields Schema, common bool) Schema {
	properties := Schema{
		"type": Schema{"type": "string", "enum": []any{itemType}},
	}
	if common {
		properties["id"] = Schema{"type": "string"}
		properties[responseItemMetadataField] = Schema{"$ref": "#/$defs/InternalChatMessageMetadataPassthrough"}
	}
	for name, schema := range fields {
		properties[name] = schema
	}
	required := []string{"type"}
	required = append(required, requiredFields...)
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func commandActionVariantSchema(actionType string, requiredFields []string, fields Schema) Schema {
	properties := Schema{
		"type": Schema{"type": "string", "enum": []any{actionType}},
	}
	for name, schema := range fields {
		properties[name] = schema
	}
	required := []string{"type"}
	required = append(required, requiredFields...)
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func webSearchActionSchema() Schema {
	return Schema{"oneOf": []any{
		webSearchActionVariantSchema("search", []string{"query", "queries"}, Schema{
			"query": nullableStringSchema(),
			"queries": Schema{"oneOf": []any{
				Schema{"type": "array", "items": Schema{"type": "string"}},
				Schema{"type": "null"},
			}},
		}),
		webSearchActionVariantSchema("openPage", []string{"url"}, Schema{
			"url": nullableStringSchema(),
		}),
		webSearchActionVariantSchema("findInPage", []string{"url", "pattern"}, Schema{
			"url":     nullableStringSchema(),
			"pattern": nullableStringSchema(),
		}),
		webSearchActionVariantSchema("other", nil, nil),
	}}
}

func webSearchActionVariantSchema(actionType string, requiredFields []string, fields Schema) Schema {
	properties := Schema{
		"type": Schema{"type": "string", "enum": []any{actionType}},
	}
	for name, schema := range fields {
		properties[name] = schema
	}
	required := []string{"type"}
	required = append(required, requiredFields...)
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func userInputSchema() Schema {
	return Schema{"oneOf": []any{
		userInputVariantSchema("text", []string{"text", "text_elements"}, Schema{
			"text": Schema{"type": "string"},
			"text_elements": Schema{
				"type":  "array",
				"items": Schema{"$ref": "#/$defs/TextElement"},
			},
		}),
		userInputVariantSchema("image", []string{"url"}, Schema{
			"detail": Schema{"$ref": "#/$defs/ImageDetail"},
			"url":    Schema{"type": "string"},
		}),
		userInputVariantSchema("localImage", []string{"path"}, Schema{
			"detail": Schema{"$ref": "#/$defs/ImageDetail"},
			"path":   Schema{"type": "string"},
		}),
		userInputVariantSchema("skill", []string{"name", "path"}, Schema{
			"name": Schema{"type": "string"},
			"path": Schema{"type": "string"},
		}),
		userInputVariantSchema("mention", []string{"name", "path"}, Schema{
			"name": Schema{"type": "string"},
			"path": Schema{"type": "string"},
		}),
	}}
}

func userInputVariantSchema(inputType string, requiredFields []string, fields Schema) Schema {
	properties := Schema{
		"type": Schema{"type": "string", "enum": []any{inputType}},
	}
	for name, schema := range fields {
		properties[name] = schema
	}
	required := []string{"type"}
	required = append(required, requiredFields...)
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func fileSystemSpecialPathSchema() Schema {
	return Schema{"oneOf": []any{
		fileSystemSpecialPathVariantSchema("root", false, false),
		fileSystemSpecialPathVariantSchema("minimal", false, false),
		fileSystemSpecialPathVariantSchema("project_roots", false, true),
		fileSystemSpecialPathVariantSchema("tmpdir", false, false),
		fileSystemSpecialPathVariantSchema("slash_tmp", false, false),
		fileSystemSpecialPathVariantSchema("unknown", true, true),
	}}
}

func fileSystemSpecialPathVariantSchema(kind string, includePath, includeSubpath bool) Schema {
	properties := Schema{
		"kind": Schema{"type": "string", "enum": []any{kind}},
	}
	required := []string{"kind"}
	if includePath {
		properties["path"] = Schema{"type": "string"}
		required = append(required, "path")
	}
	if includeSubpath {
		properties["subpath"] = nullableStringSchema()
		required = append(required, "subpath")
	}
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func fileSystemPathSchema() Schema {
	return Schema{"oneOf": []any{
		fileSystemPathVariantSchema("path", "path", Schema{"$ref": "#/$defs/LegacyAppPathString"}),
		fileSystemPathVariantSchema("glob_pattern", "pattern", Schema{"type": "string"}),
		fileSystemPathVariantSchema("special", "value", Schema{"$ref": "#/$defs/FileSystemSpecialPath"}),
	}}
}

func fileSystemPathVariantSchema(pathType, valueField string, valueSchema Schema) Schema {
	return Schema{
		"type": "object",
		"properties": Schema{
			"type":     Schema{"type": "string", "enum": []any{pathType}},
			valueField: valueSchema,
		},
		"required":             []string{"type", valueField},
		"additionalProperties": false,
	}
}

func patchChangeKindSchema() Schema {
	return Schema{"oneOf": []any{
		patchChangeKindVariantSchema("add", ""),
		patchChangeKindVariantSchema("delete", ""),
		patchChangeKindVariantSchema("update", "move_path"),
		patchChangeKindVariantSchema("update", "movePath"),
	}}
}

func dynamicToolCallOutputContentItemSchema() Schema {
	return Schema{"oneOf": []any{
		dynamicToolCallOutputContentItemVariantSchema("inputText", "text"),
		dynamicToolCallOutputContentItemVariantSchema("inputImage", "imageUrl"),
	}}
}

func dynamicToolCallOutputContentItemVariantSchema(contentType, valueField string) Schema {
	return Schema{
		"type": "object",
		"properties": Schema{
			"type":     Schema{"type": "string", "enum": []any{contentType}},
			valueField: Schema{"type": "string"},
		},
		"required":             []string{"type", valueField},
		"additionalProperties": false,
	}
}

func commandExecutionApprovalDecisionSchema() Schema {
	execPolicyVariant := Schema{
		"type":                 "object",
		"additionalProperties": false,
		"properties": Schema{
			CommandExecutionApprovalAcceptWithExecpolicyAmendment: Schema{
				"type":                 "object",
				"additionalProperties": false,
				"properties": Schema{
					"execpolicy_amendment": Schema{"$ref": "#/$defs/ExecPolicyAmendment"},
				},
				"required": []any{"execpolicy_amendment"},
			},
		},
		"required": []any{CommandExecutionApprovalAcceptWithExecpolicyAmendment},
	}
	networkPolicyVariant := Schema{
		"type":                 "object",
		"additionalProperties": false,
		"properties": Schema{
			CommandExecutionApprovalApplyNetworkPolicyAmendment: Schema{
				"type":                 "object",
				"additionalProperties": false,
				"properties": Schema{
					"network_policy_amendment": Schema{"$ref": "#/$defs/NetworkPolicyAmendment"},
				},
				"required": []any{"network_policy_amendment"},
			},
		},
		"required": []any{CommandExecutionApprovalApplyNetworkPolicyAmendment},
	}
	return Schema{"oneOf": []any{
		stringEnumSchema(CommandExecutionApprovalAccept),
		stringEnumSchema(CommandExecutionApprovalAcceptForSession),
		execPolicyVariant,
		networkPolicyVariant,
		stringEnumSchema(CommandExecutionApprovalDecline),
		stringEnumSchema(CommandExecutionApprovalCancel),
	}}
}

func schemaRefUnion(names ...string) Schema {
	variants := make([]any, 0, len(names))
	for _, name := range names {
		variants = append(variants, Schema{"$ref": "#/$defs/" + name})
	}
	return Schema{"anyOf": variants}
}

func mcpServerElicitationRequestParamsSchema() Schema {
	return Schema{"oneOf": []any{
		mcpServerElicitationRequestVariantSchema("form"),
		mcpServerElicitationRequestVariantSchema("openai/form"),
		mcpServerElicitationRequestVariantSchema("url"),
	}}
}

func mcpServerElicitationRequestVariantSchema(mode string) Schema {
	properties := Schema{
		"threadId":    Schema{"type": "string"},
		"turnId":      Schema{"anyOf": []any{Schema{"type": "string"}, Schema{"type": "null"}}},
		"serverName":  Schema{"type": "string"},
		"mode":        Schema{"type": "string", "enum": []any{mode}},
		"_meta":       Schema{},
		"message":     Schema{"type": "string"},
		"requestId":   Schema{"type": "string"},
		"itemId":      Schema{"type": "string"},
		"startedAtMs": Schema{"type": "integer"},
		"serverId":    Schema{"type": "string"},
		"schema": Schema{"anyOf": []any{
			Schema{"type": "object", "additionalProperties": Schema{}},
			Schema{"type": "null"},
		}},
		"metadata": Schema{"anyOf": []any{
			Schema{"type": "object", "additionalProperties": Schema{}},
			Schema{"type": "null"},
		}},
		"reason": Schema{"type": "string"},
	}
	required := []string{"threadId", "turnId", "serverName", "mode", "_meta", "message"}
	switch mode {
	case "form":
		properties["requestedSchema"] = Schema{"$ref": "#/$defs/McpElicitationSchema"}
		required = append(required, "requestedSchema")
	case "openai/form":
		properties["requestedSchema"] = Schema{}
		required = append(required, "requestedSchema")
	case "url":
		properties["url"] = Schema{"type": "string"}
		properties["elicitationId"] = Schema{"type": "string"}
		required = append(required, "url", "elicitationId")
	}
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func setSchemaIntegerMinimum(schema Schema, minimum int, names ...string) {
	properties, _ := schema["properties"].(Schema)
	for _, name := range names {
		if property, ok := properties[name].(Schema); ok {
			property["minimum"] = minimum
		}
	}
}

func patchChangeKindVariantSchema(kind, requiredMovePath string) Schema {
	properties := Schema{
		"type": Schema{"type": "string", "enum": []any{kind}},
	}
	required := []string{"type"}
	if kind == "update" {
		properties["move_path"] = nullableStringSchema()
		properties["movePath"] = nullableStringSchema()
		required = append(required, requiredMovePath)
	}
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func nullableStringSchema() Schema {
	return Schema{"anyOf": []any{Schema{"type": "string"}, Schema{"type": "null"}}}
}

func schemaWithRequiredIDAlternative(base Schema) Schema {
	return schemaWithRequiredFieldAlternatives(base, []string{"threadId"}, []string{"id"})
}

func schemaWithRequiredFieldAlternatives(base Schema, requiredFields ...[]string) Schema {
	variants := make([]any, 0, len(requiredFields))
	for _, required := range requiredFields {
		variant := make(Schema, len(base))
		for key, value := range base {
			variant[key] = value
		}
		variant["required"] = append([]string(nil), required...)
		variants = append(variants, variant)
	}
	return Schema{"anyOf": variants}
}

type wireSchemaDefinition struct {
	Name string
	Type reflect.Type
}

func MarshalJSONSchema() ([]byte, error) {
	data, err := json.MarshalIndent(JSONSchema(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func methodEnumSchema(surfaces ...Surface) Schema {
	allowed := make(map[Surface]bool, len(surfaces))
	for _, surface := range surfaces {
		allowed[surface] = true
	}
	var enum []any
	for _, info := range methodRegistry {
		if allowed[info.Surface] {
			enum = append(enum, info.Method)
		}
	}
	return Schema{"type": "string", "enum": enum}
}

func stringEnumSchema(values ...string) Schema {
	enum := make([]any, len(values))
	for i, value := range values {
		enum[i] = value
	}
	return Schema{"type": "string", "enum": enum}
}
