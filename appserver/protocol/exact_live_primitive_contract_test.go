package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExactLivePrimitiveSchemasAndBindings(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"RequestId",
		"CommandExecutionOutputDeltaNotification",
		"CommandExecutionStatus",
		"DynamicToolCallStatus",
		"FileChangePatchUpdatedNotification",
		"McpToolCallError",
		"McpToolCallProgressNotification",
		"McpToolCallStatus",
		"ServerRequestResolvedNotification",
		// Keep every Gollem v1 generated name available.
		"CommandExecutionOutputDeltaNotificationParams",
		"FileChangePatchUpdatedNotificationParams",
		"MCPToolCallError",
		"MCPToolCallProgressNotificationParams",
		"ServerRequestResolvedNotificationParams",
	} {
		if _, ok := defs[name]; !ok {
			t.Errorf("schema missing %s", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	if got := defs["RequestId"].(Schema)["$ref"]; got != "#/$defs/RequestID" {
		t.Fatalf("RequestId schema = %#v", defs["RequestId"])
	}
	assertStringEnum(t, defs["CommandExecutionStatus"], "inProgress", "completed", "failed", "declined")
	assertStringEnum(t, defs["DynamicToolCallStatus"], "inProgress", "completed", "failed")
	assertStringEnum(t, defs["McpToolCallStatus"], "inProgress", "completed", "failed")
	assertExactLivePrimitivePropertyRef(t, defs, "CommandExecutionItem", "status", "CommandExecutionStatus")
	assertExactLivePrimitivePropertyRef(t, defs, "DynamicToolCallItem", "status", "DynamicToolCallStatus")
	assertExactLivePrimitivePropertyRef(t, defs, "MCPToolCallItem", "status", "McpToolCallStatus")
	assertExactLivePrimitivePropertyRef(t, defs, "MCPToolCallItem", "error", "McpToolCallError")
	assertExactLivePrimitivePropertyRef(t, defs, "ServerRequestResolvedNotification", "requestId", "RequestId")

	for name, fields := range map[string][]string{
		"CommandExecutionOutputDeltaNotification": {"threadId", "turnId", "itemId", "delta"},
		"FileChangePatchUpdatedNotification":      {"threadId", "turnId", "itemId", "changes"},
		"McpToolCallError":                        {"message"},
		"McpToolCallProgressNotification":         {"threadId", "turnId", "itemId", "message"},
		"ServerRequestResolvedNotification":       {"threadId", "requestId"},
	} {
		assertSchemaRequired(t, defs[name].(Schema), fields...)
	}
	changes := defs["FileChangePatchUpdatedNotification"].(Schema)["properties"].(Schema)["changes"].(Schema)
	if changes["type"] != "array" || changes["anyOf"] != nil || changes["items"].(Schema)["$ref"] != "#/$defs/FileUpdateChange" {
		t.Fatalf("FileChangePatchUpdatedNotification.changes = %#v", changes)
	}

	for alias, canonical := range map[string]string{
		"CommandExecutionOutputDeltaNotificationParams": "CommandExecutionOutputDeltaNotification",
		"FileChangePatchUpdatedNotificationParams":      "FileChangePatchUpdatedNotification",
		"MCPToolCallError":                              "McpToolCallError",
		"MCPToolCallProgressNotificationParams":         "McpToolCallProgressNotification",
		"ServerRequestResolvedNotificationParams":       "ServerRequestResolvedNotification",
	} {
		if got := defs[alias].(Schema)["$ref"]; got != "#/$defs/"+canonical {
			t.Errorf("%s alias = %#v", alias, defs[alias])
		}
	}

	wantBindings := map[string]string{
		"item/commandExecution/outputDelta": "CommandExecutionOutputDeltaNotification",
		"item/fileChange/patchUpdated":      "FileChangePatchUpdatedNotification",
		"item/mcpToolCall/progress":         "McpToolCallProgressNotification",
		"serverRequest/resolved":            "ServerRequestResolvedNotification",
	}
	for method, want := range wantBindings {
		binding, ok := exactLivePrimitiveBinding(WireTypeBindings(), method)
		if !ok {
			t.Errorf("missing %s binding", method)
			continue
		}
		if binding.Surface != SurfaceServerNotification || !reflect.DeepEqual(binding.Params, []string{want}) || len(binding.Result) != 0 {
			t.Errorf("%s binding = %+v, want server-notification params [%s]", method, binding, want)
		}
	}
}

func TestExactLivePrimitiveValuesPreserveAndCanonicalizeWire(t *testing.T) {
	values := []struct {
		name string
		got  any
		want string
	}{
		{
			name: "command output delta",
			got:  CommandExecutionOutputDeltaNotification{ThreadID: "thread", TurnID: "turn", ItemID: "item", Delta: "output"},
			want: `{"threadId":"thread","turnId":"turn","itemId":"item","delta":"output"}`,
		},
		{
			name: "empty file patch",
			got:  FileChangePatchUpdatedNotification{ThreadID: "thread", TurnID: "turn", ItemID: "item"},
			want: `{"threadId":"thread","turnId":"turn","itemId":"item","changes":[]}`,
		},
		{
			name: "MCP error",
			got:  McpToolCallError{Message: "failed"},
			want: `{"message":"failed"}`,
		},
		{
			name: "MCP progress",
			got:  McpToolCallProgressNotification{ThreadID: "thread", TurnID: "turn", ItemID: "item", Message: "working"},
			want: `{"threadId":"thread","turnId":"turn","itemId":"item","message":"working"}`,
		},
		{
			name: "resolved string request",
			got:  ServerRequestResolvedNotification{ThreadID: "thread", RequestID: NewStringID("request")},
			want: `{"threadId":"thread","requestId":"request"}`,
		},
		{
			name: "resolved numeric request",
			got:  ServerRequestResolvedNotification{ThreadID: "thread", RequestID: NewNumberID(42)},
			want: `{"threadId":"thread","requestId":42}`,
		},
	}
	for _, tc := range values {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.got)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("wire = %s, want %s", got, tc.want)
			}
		})
	}

	for _, wire := range []string{
		`{"threadId":"thread","requestId":"request"}`,
		`{"threadId":"thread","requestId":42}`,
	} {
		var resolved ServerRequestResolvedNotification
		if err := json.Unmarshal([]byte(wire), &resolved); err != nil {
			t.Errorf("Unmarshal(%s): %v", wire, err)
		} else if resolved.RequestID.Value() == nil {
			t.Errorf("Unmarshal(%s) produced an empty request id", wire)
		}
	}

	for name, values := range map[string][]string{
		"command": {string(CommandExecutionStatusInProgress), string(CommandExecutionStatusCompleted), string(CommandExecutionStatusFailed), string(CommandExecutionStatusDeclined)},
		"dynamic": {string(DynamicToolCallStatusInProgress), string(DynamicToolCallStatusCompleted), string(DynamicToolCallStatusFailed)},
		"MCP":     {string(McpToolCallStatusInProgress), string(McpToolCallStatusCompleted), string(McpToolCallStatusFailed)},
	} {
		if len(values) == 0 {
			t.Errorf("%s statuses are empty", name)
		}
	}
}

func exactLivePrimitiveBinding(bindings []WireTypeBinding, method string) (WireTypeBinding, bool) {
	for _, binding := range bindings {
		if binding.Method == method {
			return binding, true
		}
	}
	return WireTypeBinding{}, false
}

func assertExactLivePrimitivePropertyRef(t *testing.T, defs Schema, definition, property, want string) {
	t.Helper()
	properties := defs[definition].(Schema)["properties"].(Schema)
	schema := properties[property].(Schema)
	if variants, ok := schema["anyOf"].([]any); ok && len(variants) > 0 {
		schema = variants[0].(Schema)
	}
	if schema["$ref"] != "#/$defs/"+want {
		t.Fatalf("%s.%s = %#v, want %s ref", definition, property, properties[property], want)
	}
}
