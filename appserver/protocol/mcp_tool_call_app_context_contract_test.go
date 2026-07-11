package protocol

import (
	"encoding/json"
	"testing"
)

func TestMcpToolCallAppContextSchemaIsExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	context, ok := defs["McpToolCallAppContext"].(Schema)
	if !ok {
		t.Fatal("$defs missing McpToolCallAppContext")
	}
	assertSchemaRequiredNames(
		t,
		context,
		"connectorId",
		"linkId",
		"resourceUri",
		"appName",
		"templateId",
		"actionName",
	)
	properties := context["properties"].(Schema)
	if connector := properties["connectorId"].(Schema); connector["type"] != "string" {
		t.Fatalf("connectorId = %#v", connector)
	}
	for _, name := range []string{"linkId", "resourceUri", "appName", "templateId", "actionName"} {
		assertNullableStringSchema(t, properties[name])
	}
	if context["additionalProperties"] != false {
		t.Fatalf("McpToolCallAppContext allows extra fields: %#v", context)
	}
}

func TestMcpToolCallAppContextWireValidation(t *testing.T) {
	valid := []string{
		`{"connectorId":"connector-1","linkId":null,"resourceUri":null,"appName":null,"templateId":null,"actionName":null}`,
		`{"connectorId":"","linkId":"link-1","resourceUri":"app://resource","appName":"Repo","templateId":"template-1","actionName":"search"}`,
	}
	for _, input := range valid {
		var context McpToolCallAppContext
		if err := json.Unmarshal([]byte(input), &context); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
		}
	}

	invalid := []string{
		`null`, `[]`, `{}`,
		`{"connectorId":null,"linkId":null,"resourceUri":null,"appName":null,"templateId":null,"actionName":null}`,
		`{"connectorId":"c","resourceUri":null,"appName":null,"templateId":null,"actionName":null}`,
		`{"connectorId":"c","linkId":null,"appName":null,"templateId":null,"actionName":null}`,
		`{"connectorId":"c","linkId":null,"resourceUri":null,"templateId":null,"actionName":null}`,
		`{"connectorId":"c","linkId":null,"resourceUri":null,"appName":null,"actionName":null}`,
		`{"connectorId":"c","linkId":null,"resourceUri":null,"appName":null,"templateId":null}`,
		`{"connectorId":"c","linkId":1,"resourceUri":null,"appName":null,"templateId":null,"actionName":null}`,
		`{"connector_id":"c","link_id":null,"resource_uri":null,"app_name":null,"template_id":null,"action_name":null}`,
		`{"connectorId":"c","linkId":null,"resourceUri":null,"appName":null,"templateId":null,"actionName":null,"extra":true}`,
	}
	for _, input := range invalid {
		var context McpToolCallAppContext
		if err := json.Unmarshal([]byte(input), &context); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
}

func TestMcpToolCallAppContextMarshalEmitsExplicitNulls(t *testing.T) {
	encoded, err := json.Marshal(McpToolCallAppContext{ConnectorID: "connector-1"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"connectorId":"connector-1","linkId":null,"resourceUri":null,"appName":null,"templateId":null,"actionName":null}`
	if string(encoded) != want {
		t.Fatalf("Marshal = %s, want %s", encoded, want)
	}

	var context *McpToolCallAppContext
	if err := context.UnmarshalJSON(encoded); err == nil {
		t.Fatal("nil McpToolCallAppContext receiver succeeded")
	}
}
