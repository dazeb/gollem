package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCommandExecutionSourceSchemaIsExactAndUnbound(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	assertStringEnumSchema(t, defs, "CommandExecutionSource", []string{
		"agent", "userShell", "unifiedExecStartup", "unifiedExecInteraction",
	})

	item := defs["CommandExecutionItem"].(Schema)
	source := item["properties"].(Schema)["source"].(Schema)
	if _, ok := source["$ref"]; ok {
		t.Fatalf("live CommandExecutionItem source unexpectedly references public enum: %#v", source)
	}
	if !reflect.DeepEqual(source, stringEnumSchema("agent", "userShell")) {
		t.Fatalf("live CommandExecutionItem source = %#v", source)
	}

	field, ok := reflect.TypeFor[CommandExecutionItem]().FieldByName("Source")
	if !ok || field.Type != reflect.TypeFor[string]() {
		t.Fatalf("live CommandExecutionItem Source type = %v, present %t", field.Type, ok)
	}
}

func TestCommandExecutionSourceWireValidation(t *testing.T) {
	valid := []string{
		"agent",
		"userShell",
		"unifiedExecStartup",
		"unifiedExecInteraction",
	}
	for _, input := range valid {
		var source CommandExecutionSource
		if err := json.Unmarshal([]byte(`"`+input+`"`), &source); err != nil {
			t.Errorf("decode %q: %v", input, err)
			continue
		}
		encoded, err := json.Marshal(source)
		if err != nil || string(encoded) != `"`+input+`"` {
			t.Errorf("marshal %q = %s, %v", input, encoded, err)
		}
	}

	for _, input := range []string{
		`null`, `1`, `true`, `[]`, `{}`,
		`""`, `"unknown"`, `"usershell"`, `"UnifiedExecStartup"`,
	} {
		var source CommandExecutionSource
		if err := json.Unmarshal([]byte(input), &source); err == nil {
			t.Errorf("decode %s succeeded", input)
		}
	}
	for _, value := range []CommandExecutionSource{"", "unknown", "usershell"} {
		if _, err := json.Marshal(value); err == nil {
			t.Errorf("marshal %q succeeded", value)
		}
	}

	var source *CommandExecutionSource
	if err := source.UnmarshalJSON([]byte(`"agent"`)); err == nil {
		t.Fatal("nil CommandExecutionSource receiver succeeded")
	}
}
