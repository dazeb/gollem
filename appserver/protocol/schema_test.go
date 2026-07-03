package protocol

import (
	"encoding/json"
	"testing"
)

func TestJSONSchemaContainsEnvelopeAndMethodInventory(t *testing.T) {
	schema := JSONSchema()
	if schema["x-gollem-protocol-version"] != ProtocolVersion {
		t.Fatalf("protocol version = %v", schema["x-gollem-protocol-version"])
	}
	if _, err := json.Marshal(schema); err != nil {
		t.Fatalf("schema must be JSON serializable: %v", err)
	}

	methods, ok := schema["x-gollem-methods"].([]MethodInfo)
	if !ok {
		t.Fatalf("x-gollem-methods = %T", schema["x-gollem-methods"])
	}
	if len(methods) != 224 {
		t.Fatalf("schema method inventory has %d rows, want 224", len(methods))
	}

	defs := schema["$defs"].(Schema)
	request := defs["Request"].(Schema)
	props := request["properties"].(Schema)
	if _, exists := props["jsonrpc"]; exists {
		t.Fatal("request schema should not expose jsonrpc")
	}
	methodSchema := props["method"].(Schema)
	enum := methodSchema["enum"].([]any)
	assertEnumContains(t, enum, "initialize")
	assertEnumContains(t, enum, "turn/retry")

	notification := defs["Notification"].(Schema)
	notificationProps := notification["properties"].(Schema)
	notificationMethods := notificationProps["method"].(Schema)["enum"].([]any)
	assertEnumContains(t, notificationMethods, "initialized")
	assertEnumContains(t, notificationMethods, "thread/started")
}

func assertEnumContains(t *testing.T, enum []any, want string) {
	t.Helper()
	for _, value := range enum {
		if value == want {
			return
		}
	}
	t.Fatalf("enum missing %q", want)
}
