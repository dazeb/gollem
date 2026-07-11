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
		{Name: "ApprovalRequestBase", Type: reflect.TypeFor[ApprovalRequestBase]()},
		{Name: "ApprovalRespondParams", Type: reflect.TypeFor[ApprovalRespondParams]()},
		{Name: "ApprovalRespondResult", Type: reflect.TypeFor[ApprovalRespondResult]()},
		{Name: "ClientInfo", Type: reflect.TypeFor[ClientInfo]()},
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
		{Name: "ContextCompactionItem", Type: reflect.TypeFor[ContextCompactionItem]()},
		{Name: "DaemonShutdownParams", Type: reflect.TypeFor[DaemonShutdownParams]()},
		{Name: "DaemonShutdownState", Type: reflect.TypeFor[DaemonShutdownState]()},
		{Name: "DaemonStartResult", Type: reflect.TypeFor[DaemonStartResult]()},
		{Name: "DaemonStatus", Type: reflect.TypeFor[DaemonStatus]()},
		{Name: "DaemonStopResult", Type: reflect.TypeFor[DaemonStopResult]()},
		{Name: "DaemonVersion", Type: reflect.TypeFor[DaemonVersion]()},
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
		{Name: "ImplementationInfo", Type: reflect.TypeFor[ImplementationInfo]()},
		{Name: "InitializeCapabilities", Type: reflect.TypeFor[InitializeCapabilities]()},
		{Name: "InitializeParams", Type: reflect.TypeFor[InitializeParams]()},
		{Name: "InitializeResponse", Type: reflect.TypeFor[InitializeResponse]()},
		{Name: "ItemLifecycleNotificationParams", Type: reflect.TypeFor[ItemLifecycleNotificationParams]()},
		{Name: "MCPContent", Type: reflect.TypeFor[MCPContent]()},
		{Name: "MCPToolCallError", Type: reflect.TypeFor[MCPToolCallError]()},
		{Name: "MCPToolCallItem", Type: reflect.TypeFor[MCPToolCallItem]()},
		{Name: "MCPToolCallItemCompletedNotificationParams", Type: reflect.TypeFor[MCPToolCallItemCompletedNotificationParams]()},
		{Name: "MCPToolCallItemStartedNotificationParams", Type: reflect.TypeFor[MCPToolCallItemStartedNotificationParams]()},
		{Name: "MCPToolCallProgressNotificationParams", Type: reflect.TypeFor[MCPToolCallProgressNotificationParams]()},
		{Name: "MCPToolCallResult", Type: reflect.TypeFor[MCPToolCallResult]()},
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
		{Name: "PermissionsApprovalRequestParams", Type: reflect.TypeFor[PermissionsApprovalRequestParams]()},
		{Name: "ServerRequestResolvedNotificationParams", Type: reflect.TypeFor[ServerRequestResolvedNotificationParams]()},
		{Name: "ServerCapabilities", Type: reflect.TypeFor[ServerCapabilities]()},
		{Name: "Surface", Type: reflect.TypeFor[Surface]()},
		{Name: "SortDirection", Type: reflect.TypeFor[SortDirection]()},
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
