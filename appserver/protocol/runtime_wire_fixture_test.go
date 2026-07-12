package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type runtimeWireFixture struct {
	ProtocolVersion string                   `json:"protocolVersion"`
	SchemaVersion   string                   `json:"schemaVersion"`
	Cases           []runtimeWireFixtureCase `json:"cases"`
}

type runtimeWireFixtureCase struct {
	Name        string          `json:"name"`
	Surface     Surface         `json:"surface"`
	Method      string          `json:"method"`
	Envelope    string          `json:"envelope,omitempty"`
	ParamsType  string          `json:"paramsType,omitempty"`
	ResultType  string          `json:"resultType,omitempty"`
	PayloadType string          `json:"payloadType,omitempty"`
	Message     json.RawMessage `json:"message"`
}

func TestInitializeWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "initialize_wire_v1.json"))
	if err != nil {
		t.Fatalf("read initialize fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode initialize fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s, want %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion, ProtocolVersion, SchemaVersion)
	}
	if len(fixture.Cases) != 3 {
		t.Fatalf("initialize fixture has %d cases, want 3", len(fixture.Cases))
	}

	cases := make(map[string]runtimeWireFixtureCase, len(fixture.Cases))
	for _, fixtureCase := range fixture.Cases {
		if _, exists := cases[fixtureCase.Name]; exists {
			t.Fatalf("duplicate fixture case %q", fixtureCase.Name)
		}
		cases[fixtureCase.Name] = fixtureCase
	}

	requestCase := requireRuntimeFixtureCase(t, cases, "initialize-request", "initialize", SurfaceClientRequest, "request")
	requestPayload, err := fixtureMessagePayload(requestCase)
	if err != nil {
		t.Fatalf("initialize request: %v", err)
	}
	var params InitializeParams
	if err := decodeRuntimeFixture(requestPayload, &params); err != nil {
		t.Fatalf("decode InitializeParams: %v", err)
	}
	if params.ClientInfo.Name != "gollem-typescript-fixture" || params.ClientInfo.Version != "1.0.0" ||
		params.Capabilities == nil || !params.Capabilities.ExperimentalAPI || params.Capabilities.RequestAttestation ||
		!params.Capabilities.MCPServerOpenAIFormElicitation ||
		!params.Capabilities.Experimental["typedInitialize"] {
		t.Fatalf("initialize params = %+v", params)
	}
	assertBinding(t, WireTypeBindings(), "initialize", SurfaceClientRequest, "InitializeParams")

	responseCase := requireRuntimeFixtureCase(t, cases, "initialize-response", "initialize", SurfaceClientRequest, "response")
	responsePayload, err := fixtureMessagePayload(responseCase)
	if err != nil {
		t.Fatalf("initialize response: %v", err)
	}
	var response InitializeResponse
	if err := decodeRuntimeFixture(responsePayload, &response); err != nil {
		t.Fatalf("decode InitializeResponse: %v", err)
	}
	if response.ProtocolVersion != ProtocolVersion || response.UserAgent != "gollem-appserver/dev" ||
		response.CodexHome != "/workspace/.gollem" || response.PlatformFamily != "unix" || response.PlatformOS != "macos" ||
		len(response.Methods) != 2 {
		t.Fatalf("initialize response = %+v", response)
	}
	if response.Methods[0].Surface != SurfaceClientRequest || response.Methods[1].State != MethodImplemented {
		t.Fatalf("initialize method metadata = %+v", response.Methods)
	}
	assertBinding(t, WireTypeBindings(), "initialize", SurfaceClientRequest, "InitializeResponse")

	notificationCase := requireRuntimeFixtureCase(t, cases, "initialized-notification", "initialized", SurfaceClientNotification, "notification")
	if _, err := fixtureMessagePayload(notificationCase); err != nil {
		t.Fatalf("initialized notification: %v", err)
	}
	assertBindingMethod(t, WireTypeBindings(), "initialized", SurfaceClientNotification)
}

func TestDynamicToolCallWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "dynamic_tool_call_wire_v1.json"))
	if err != nil {
		t.Fatalf("read dynamic tool fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode dynamic tool fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion || len(fixture.Cases) != 3 {
		t.Fatalf("dynamic tool fixture metadata = %s/%s/%d", fixture.ProtocolVersion, fixture.SchemaVersion, len(fixture.Cases))
	}
	bindings := WireTypeBindings()
	for _, fixtureCase := range fixture.Cases {
		payload, err := fixtureMessagePayload(fixtureCase)
		if err != nil {
			t.Fatalf("%s payload: %v", fixtureCase.Name, err)
		}
		target := runtimeFixtureTarget(firstFixtureType(fixtureCase))
		if err := decodeRuntimeFixture(payload, target); err != nil {
			t.Fatalf("%s decode: %v", fixtureCase.Name, err)
		}
		if fixtureCase.ParamsType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ParamsType)
		}
		if fixtureCase.ResultType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ResultType)
		}
	}
}

func TestUserInputWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "user_input_wire_v1.json"))
	if err != nil {
		t.Fatalf("read user input fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode user input fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion || len(fixture.Cases) != 2 {
		t.Fatalf("user input fixture metadata = %s/%s/%d", fixture.ProtocolVersion, fixture.SchemaVersion, len(fixture.Cases))
	}
	bindings := WireTypeBindings()
	for _, fixtureCase := range fixture.Cases {
		payload, err := fixtureMessagePayload(fixtureCase)
		if err != nil {
			t.Fatalf("%s payload: %v", fixtureCase.Name, err)
		}
		target := runtimeFixtureTarget(firstFixtureType(fixtureCase))
		if err := decodeRuntimeFixture(payload, target); err != nil {
			t.Fatalf("%s decode: %v", fixtureCase.Name, err)
		}
		if fixtureCase.ParamsType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ParamsType)
		}
		if fixtureCase.ResultType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ResultType)
		}
	}
}

func TestMcpElicitationWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "mcp_elicitation_wire_v1.json"))
	if err != nil {
		t.Fatalf("read MCP elicitation fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode MCP elicitation fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion || len(fixture.Cases) != 4 {
		t.Fatalf("MCP elicitation fixture metadata = %s/%s/%d", fixture.ProtocolVersion, fixture.SchemaVersion, len(fixture.Cases))
	}
	bindings := WireTypeBindings()
	for _, fixtureCase := range fixture.Cases {
		payload, err := fixtureMessagePayload(fixtureCase)
		if err != nil {
			t.Fatalf("%s payload: %v", fixtureCase.Name, err)
		}
		target := runtimeFixtureTarget(firstFixtureType(fixtureCase))
		if err := decodeRuntimeFixture(payload, target); err != nil {
			t.Fatalf("%s decode: %v", fixtureCase.Name, err)
		}
		if fixtureCase.ParamsType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ParamsType)
		}
		if fixtureCase.ResultType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ResultType)
		}
	}
}

func TestCommandApprovalResponseWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "command_approval_response_wire_v1.json"))
	if err != nil {
		t.Fatalf("read command approval fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode command approval fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion || len(fixture.Cases) != 2 {
		t.Fatalf("command approval fixture metadata = %s/%s/%d", fixture.ProtocolVersion, fixture.SchemaVersion, len(fixture.Cases))
	}
	bindings := WireTypeBindings()
	for _, fixtureCase := range fixture.Cases {
		payload, err := fixtureMessagePayload(fixtureCase)
		if err != nil {
			t.Fatalf("%s payload: %v", fixtureCase.Name, err)
		}
		target := runtimeFixtureTarget(firstFixtureType(fixtureCase))
		if err := decodeRuntimeFixture(payload, target); err != nil {
			t.Fatalf("%s decode: %v", fixtureCase.Name, err)
		}
		if fixtureCase.ParamsType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ParamsType)
		}
		if fixtureCase.ResultType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ResultType)
		}
	}
}

func requireRuntimeFixtureCase(t *testing.T, cases map[string]runtimeWireFixtureCase, name, method string, surface Surface, envelope string) runtimeWireFixtureCase {
	t.Helper()
	fixtureCase, ok := cases[name]
	if !ok {
		t.Fatalf("fixture missing %s", name)
	}
	if fixtureCase.Method != method || fixtureCase.Surface != surface || fixtureCase.Envelope != envelope {
		t.Fatalf("fixture %s metadata = %s/%s/%s, want %s/%s/%s", name, fixtureCase.Surface, fixtureCase.Method, fixtureCase.Envelope, surface, method, envelope)
	}
	return fixtureCase
}

func TestRuntimeWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "runtime_wire_v1.json"))
	if err != nil {
		t.Fatalf("read runtime fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode runtime fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s, want %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion, ProtocolVersion, SchemaVersion)
	}

	wantCases := map[string]bool{
		"dynamic-tool-started":           false,
		"command-completed":              false,
		"command-output-delta":           false,
		"file-change-completed":          false,
		"file-change-patch-updated":      false,
		"mcp-call-completed":             false,
		"mcp-call-progress":              false,
		"server-request-resolved-string": false,
		"server-request-resolved-number": false,
		"context-compaction-item":        false,
		"deprecation-notice":             false,
		"thread-compacted":               false,
		"token-usage-updated":            false,
		"turn-diff-updated":              false,
		"file-change-approval":           false,
		"file-change-approval-response":  false,
		"daemon-status":                  false,
	}
	bindings := WireTypeBindings()
	payloadBindings := ItemPayloadBindings()
	seen := make(map[string]bool, len(fixture.Cases))
	for _, fixtureCase := range fixture.Cases {
		if seen[fixtureCase.Name] {
			t.Errorf("duplicate fixture case %q", fixtureCase.Name)
			continue
		}
		seen[fixtureCase.Name] = true
		if _, ok := wantCases[fixtureCase.Name]; !ok {
			t.Errorf("unexpected fixture case %q", fixtureCase.Name)
		} else {
			wantCases[fixtureCase.Name] = true
		}

		info, ok := LookupMethod(fixtureCase.Method)
		if !ok {
			t.Errorf("%s: unknown method %q", fixtureCase.Name, fixtureCase.Method)
			continue
		}
		if info.Surface != fixtureCase.Surface {
			t.Errorf("%s: surface = %s, registry has %s", fixtureCase.Name, fixtureCase.Surface, info.Surface)
			continue
		}

		payload, err := fixtureMessagePayload(fixtureCase)
		if err != nil {
			t.Errorf("%s: %v", fixtureCase.Name, err)
			continue
		}
		target := runtimeFixtureTarget(firstFixtureType(fixtureCase))
		if target == nil {
			t.Errorf("%s: unsupported fixture type", fixtureCase.Name)
			continue
		}
		if err := decodeRuntimeFixture(payload, target); err != nil {
			t.Errorf("%s: decode %s: %v", fixtureCase.Name, firstFixtureType(fixtureCase), err)
			continue
		}

		if fixtureCase.ParamsType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ParamsType)
		}
		if fixtureCase.ResultType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ResultType)
		}
		if fixtureCase.PayloadType != "" {
			lifecycle := target.(*ItemLifecycleNotificationParams)
			if lifecycle.Item == nil {
				t.Errorf("%s: lifecycle item is nil", fixtureCase.Name)
				continue
			}
			if !hasItemPayloadBinding(payloadBindings, lifecycle.Item.Kind, fixtureCase.PayloadType) {
				t.Errorf("%s: no payload binding for %s/%s", fixtureCase.Name, lifecycle.Item.Kind, fixtureCase.PayloadType)
			}
			payloadTarget := runtimeFixtureTarget(fixtureCase.PayloadType)
			if err := decodeRuntimeFixture(lifecycle.Item.Payload, payloadTarget); err != nil {
				t.Errorf("%s: decode nested %s: %v", fixtureCase.Name, fixtureCase.PayloadType, err)
			}
		}
	}
	for name, found := range wantCases {
		if !found {
			t.Errorf("fixture missing %s", name)
		}
	}
}

func fixtureMessagePayload(fixtureCase runtimeWireFixtureCase) (json.RawMessage, error) {
	envelope := fixtureCase.Envelope
	if envelope == "" {
		switch fixtureCase.Surface {
		case SurfaceServerNotification, SurfaceClientNotification:
			envelope = "notification"
		case SurfaceServerRequest, SurfaceClientRequest:
			envelope = "request"
		case SurfaceGollemExtension:
			envelope = "response"
		}
	}
	switch envelope {
	case "notification":
		var notification Notification
		if err := decodeRuntimeFixture(fixtureCase.Message, &notification); err != nil {
			return nil, err
		}
		if notification.Method != fixtureCase.Method {
			return nil, fmt.Errorf("notification method = %q, want %q", notification.Method, fixtureCase.Method)
		}
		return notification.Params, nil
	case "request":
		var request Request
		if err := decodeRuntimeFixture(fixtureCase.Message, &request); err != nil {
			return nil, err
		}
		if request.Method != fixtureCase.Method {
			return nil, fmt.Errorf("request method = %q, want %q", request.Method, fixtureCase.Method)
		}
		return request.Params, nil
	case "response":
		var response Response
		if err := decodeRuntimeFixture(fixtureCase.Message, &response); err != nil {
			return nil, err
		}
		return response.Result, nil
	default:
		return nil, fmt.Errorf("unsupported fixture envelope %q for surface %q", envelope, fixtureCase.Surface)
	}
}

func firstFixtureType(fixtureCase runtimeWireFixtureCase) string {
	if fixtureCase.ParamsType != "" {
		return fixtureCase.ParamsType
	}
	return fixtureCase.ResultType
}

func runtimeFixtureTarget(name string) any {
	switch name {
	case "CommandExecOutputDeltaNotification":
		return new(CommandExecOutputDeltaNotification)
	case "CommandExecResizeParams":
		return new(CommandExecResizeParams)
	case "CommandExecResizeResponse":
		return new(CommandExecResizeResponse)
	case "CommandExecTerminateParams":
		return new(CommandExecTerminateParams)
	case "CommandExecTerminateResponse":
		return new(CommandExecTerminateResponse)
	case "CommandExecWriteParams":
		return new(CommandExecWriteParams)
	case "CommandExecWriteResponse":
		return new(CommandExecWriteResponse)
	case "DynamicToolCallItemStartedNotificationParams":
		return new(DynamicToolCallItemStartedNotificationParams)
	case "DynamicToolCallParams":
		return new(DynamicToolCallParams)
	case "DynamicToolCallResponse":
		return new(DynamicToolCallResponse)
	case "ToolRequestUserInputParams":
		return new(ToolRequestUserInputParams)
	case "ToolRequestUserInputResponse":
		return new(ToolRequestUserInputResponse)
	case "McpServerElicitationRequestParams":
		return new(McpServerElicitationRequestParams)
	case "McpServerElicitationRequestResponse":
		return new(McpServerElicitationRequestResponse)
	case "CommandExecutionApprovalRequestParams":
		return new(CommandExecutionApprovalRequestParams)
	case "CommandExecutionRequestApprovalResponse":
		return new(CommandExecutionRequestApprovalResponse)
	case "CommandExecutionItemCompletedNotificationParams":
		return new(CommandExecutionItemCompletedNotificationParams)
	case "CommandExecutionOutputDeltaNotification":
		return new(CommandExecutionOutputDeltaNotification)
	case "FileChangeItemCompletedNotificationParams":
		return new(FileChangeItemCompletedNotificationParams)
	case "FileChangeItemStartedNotificationParams":
		return new(FileChangeItemStartedNotificationParams)
	case "FileChangePatchUpdatedNotificationParams":
		return new(FileChangePatchUpdatedNotificationParams)
	case "FileChangePatchUpdatedNotification":
		return new(FileChangePatchUpdatedNotification)
	case "MCPToolCallItemCompletedNotificationParams":
		return new(MCPToolCallItemCompletedNotificationParams)
	case "McpToolCallProgressNotification":
		return new(McpToolCallProgressNotification)
	case "ServerRequestResolvedNotification":
		return new(ServerRequestResolvedNotification)
	case "ItemLifecycleNotificationParams":
		return new(ItemLifecycleNotificationParams)
	case "ContextCompactionItem":
		return new(ContextCompactionItem)
	case "ContextCompactedNotification":
		return new(ContextCompactedNotification)
	case "DeprecationNoticeNotification":
		return new(DeprecationNoticeNotification)
	case "ThreadTokenUsageUpdatedNotification":
		return new(ThreadTokenUsageUpdatedNotification)
	case "TurnDiffUpdatedNotification":
		return new(TurnDiffUpdatedNotification)
	case "ThreadCompactedNotificationParams":
		return new(ThreadCompactedNotificationParams)
	case "ThreadTokenUsageUpdatedNotificationParams":
		return new(ThreadTokenUsageUpdatedNotificationParams)
	case "TurnDiffUpdatedNotificationParams":
		return new(TurnDiffUpdatedNotificationParams)
	case "FileChangeApprovalRequestParams":
		return new(FileChangeApprovalRequestParams)
	case "FileChangeRequestApprovalResponse":
		return new(FileChangeRequestApprovalResponse)
	case "DaemonStatus":
		return new(DaemonStatus)
	case "ThreadListParams":
		return new(ThreadListParams)
	case "ThreadListResult":
		return new(ThreadListResult)
	case "ThreadReadParams":
		return new(ThreadReadParams)
	case "ThreadReadResult":
		return new(ThreadReadResult)
	case "ThreadArchiveParams":
		return new(ThreadArchiveParams)
	case "ThreadArchiveResponse":
		return new(ThreadArchiveResponse)
	case "ThreadArchivedNotification":
		return new(ThreadArchivedNotification)
	case "ThreadClosedNotification":
		return new(ThreadClosedNotification)
	case "ThreadDeleteParams":
		return new(ThreadDeleteParams)
	case "ThreadDeleteResponse":
		return new(ThreadDeleteResponse)
	case "ThreadDeletedNotification":
		return new(ThreadDeletedNotification)
	case "ThreadLoadedListParams":
		return new(ThreadLoadedListParams)
	case "ThreadLoadedListResponse":
		return new(ThreadLoadedListResponse)
	case "ThreadMemoryModeSetParams":
		return new(ThreadMemoryModeSetParams)
	case "ThreadMemoryModeSetResponse":
		return new(ThreadMemoryModeSetResponse)
	case "ThreadMetadataUpdateParams":
		return new(ThreadMetadataUpdateParams)
	case "ThreadMetadataUpdateResult":
		return new(ThreadMetadataUpdateResult)
	case "ThreadNameUpdatedNotification":
		return new(ThreadNameUpdatedNotification)
	case "ThreadGoalClearParams":
		return new(ThreadGoalClearParams)
	case "ThreadGoalClearResponse":
		return new(ThreadGoalClearResponse)
	case "ThreadGoalClearedNotification":
		return new(ThreadGoalClearedNotification)
	case "ThreadGoalGetParams":
		return new(ThreadGoalGetParams)
	case "ThreadGoalGetResponse":
		return new(ThreadGoalGetResponse)
	case "ThreadGoalSetParams":
		return new(ThreadGoalSetParams)
	case "ThreadGoalSetResponse":
		return new(ThreadGoalSetResponse)
	case "ThreadGoalUpdatedNotification":
		return new(ThreadGoalUpdatedNotification)
	case "ThreadSetNameParams":
		return new(ThreadSetNameParams)
	case "ThreadSetNameResponse":
		return new(ThreadSetNameResponse)
	case "ThreadUnarchiveParams":
		return new(ThreadUnarchiveParams)
	case "ThreadUnarchiveResult":
		return new(ThreadUnarchiveResult)
	case "ThreadUnarchivedNotification":
		return new(ThreadUnarchivedNotification)
	case "ThreadUnsubscribeParams":
		return new(ThreadUnsubscribeParams)
	case "ThreadUnsubscribeResponse":
		return new(ThreadUnsubscribeResponse)
	default:
		return nil
	}
}

func hasItemPayloadBinding(bindings []ItemPayloadBinding, kind, typeName string) bool {
	for _, binding := range bindings {
		if binding.Kind == kind && binding.Type == typeName {
			return true
		}
	}
	return false
}

func decodeRuntimeFixture(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
