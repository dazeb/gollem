package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCommandExecControlSchemaAndBindings(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"CommandExecOutputDeltaNotification",
		"CommandExecOutputStream",
		"CommandExecResizeParams",
		"CommandExecResizeResponse",
		"CommandExecTerminalSize",
		"CommandExecTerminateParams",
		"CommandExecTerminateResponse",
		"CommandExecWriteParams",
		"CommandExecWriteResponse",
	} {
		if _, ok := defs[name].(Schema); !ok {
			t.Errorf("schema missing %s", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	assertStringEnum(t, defs["CommandExecOutputStream"], "stdout", "stderr")
	assertSchemaRequired(t, defs["CommandExecTerminalSize"].(Schema), "rows", "cols")
	assertSchemaRequired(t, defs["CommandExecOutputDeltaNotification"].(Schema),
		"processId", "stream", "deltaBase64", "capReached")
	assertSchemaRequiredFieldAlternatives(t, defs["CommandExecWriteParams"].(Schema),
		[]string{"processId"}, []string{"id"})
	assertSchemaRequiredFieldAlternatives(t, defs["CommandExecTerminateParams"].(Schema),
		[]string{"processId"}, []string{"id"})
	assertSchemaRequiredFieldAlternatives(t, defs["CommandExecResizeParams"].(Schema),
		[]string{"processId", "size"},
		[]string{"processId", "cols", "rows"},
		[]string{"id", "cols", "rows"})
	resizeVariants := requiredFieldAlternatives(defs["CommandExecResizeParams"].(Schema))
	publicResize := resizeVariants[0].(Schema)
	publicResizeProperties := publicResize["properties"].(Schema)
	if size, ok := publicResizeProperties["size"].(Schema); !ok || size["$ref"] != "#/$defs/CommandExecTerminalSize" {
		t.Fatalf("public resize size schema = %#v, want non-null terminal-size ref", publicResizeProperties["size"])
	}
	for _, name := range []string{
		"CommandExecResizeResponse",
		"CommandExecTerminateResponse",
		"CommandExecWriteResponse",
	} {
		assertNoRequiredFields(t, defs[name].(Schema))
	}

	bindings := WireTypeBindings()
	for _, test := range []struct {
		method string
		params string
		result string
	}{
		{method: "command/exec/resize", params: "CommandExecResizeParams", result: "CommandExecResizeResponse"},
		{method: "command/exec/terminate", params: "CommandExecTerminateParams", result: "CommandExecTerminateResponse"},
		{method: "command/exec/write", params: "CommandExecWriteParams", result: "CommandExecWriteResponse"},
	} {
		assertBinding(t, bindings, test.method, SurfaceClientRequest, test.params)
		assertBinding(t, bindings, test.method, SurfaceClientRequest, test.result)
	}
	assertBinding(t, bindings, "command/exec/outputDelta", SurfaceServerNotification, "CommandExecOutputDeltaNotification")
}

func TestCommandExecControlParamsPreferPublicFieldsAndPreserveLegacyForms(t *testing.T) {
	var publicWrite CommandExecWriteParams
	if err := json.Unmarshal([]byte(`{
		"processId":"public",
		"id":"legacy",
		"deltaBase64":"aGVsbG8K",
		"data":"ignored",
		"encoding":"utf8",
		"closeStdin":true
	}`), &publicWrite); err != nil {
		t.Fatalf("decode public write: %v", err)
	}
	input, encoding := publicWrite.EffectiveInput()
	if publicWrite.EffectiveProcessID() != "public" || input != "aGVsbG8K" || encoding != "base64" || !publicWrite.ShouldCloseStdin() {
		t.Fatalf("public write = %#v input=%q encoding=%q", publicWrite, input, encoding)
	}

	var legacyWrite CommandExecWriteParams
	if err := json.Unmarshal([]byte(`{"id":"legacy","data":"hello","encoding":"utf8","close":true}`), &legacyWrite); err != nil {
		t.Fatalf("decode legacy write: %v", err)
	}
	input, encoding = legacyWrite.EffectiveInput()
	if legacyWrite.EffectiveProcessID() != "legacy" || input != "hello" || encoding != "utf8" || !legacyWrite.ShouldCloseStdin() {
		t.Fatalf("legacy write = %#v input=%q encoding=%q", legacyWrite, input, encoding)
	}

	var nullWrite CommandExecWriteParams
	if err := json.Unmarshal([]byte(`{
		"processId":"",
		"id":"legacy",
		"deltaBase64":null,
		"data":"ignored",
		"closeStdin":false,
		"close":true
	}`), &nullWrite); err != nil {
		t.Fatalf("decode null public write: %v", err)
	}
	input, encoding = nullWrite.EffectiveInput()
	if !nullWrite.HasProcessID() || nullWrite.EffectiveProcessID() != "" ||
		!nullWrite.HasDeltaBase64() || input != "" || encoding != "base64" ||
		!nullWrite.HasCloseStdin() || nullWrite.ShouldCloseStdin() {
		t.Fatalf("null public write = %#v input=%q encoding=%q", nullWrite, input, encoding)
	}
	encodedNullWrite, err := json.Marshal(nullWrite)
	if err != nil {
		t.Fatalf("marshal null public write: %v", err)
	}
	var nullWriteFields map[string]json.RawMessage
	if err := json.Unmarshal(encodedNullWrite, &nullWriteFields); err != nil {
		t.Fatalf("decode marshaled null public write: %v", err)
	}
	if string(nullWriteFields["deltaBase64"]) != "null" {
		t.Fatalf("marshaled null public write = %s", encodedNullWrite)
	}
	if string(nullWriteFields["processId"]) != `""` || string(nullWriteFields["closeStdin"]) != "false" {
		t.Fatalf("marshaled explicit public write zero values = %s", encodedNullWrite)
	}

	publicResize := CommandExecResizeParams{
		ProcessID: "public",
		ID:        "legacy",
		Size:      &CommandExecTerminalSize{Rows: 24, Cols: 80},
		Rows:      1,
		Cols:      2,
	}
	cols, rows, valid := publicResize.EffectiveSize()
	if publicResize.EffectiveProcessID() != "public" || cols != 80 || rows != 24 || !valid {
		t.Fatalf("public resize = %#v cols=%d rows=%d", publicResize, cols, rows)
	}

	legacyResize := CommandExecResizeParams{ID: "legacy", Rows: 40, Cols: 120}
	cols, rows, valid = legacyResize.EffectiveSize()
	if legacyResize.EffectiveProcessID() != "legacy" || cols != 120 || rows != 40 || !valid {
		t.Fatalf("legacy resize = %#v cols=%d rows=%d", legacyResize, cols, rows)
	}

	var nullResize CommandExecResizeParams
	if err := json.Unmarshal([]byte(`{"processId":"","id":"legacy","size":null,"cols":120,"rows":40}`), &nullResize); err != nil {
		t.Fatalf("decode null public resize: %v", err)
	}
	cols, rows, valid = nullResize.EffectiveSize()
	if !nullResize.HasProcessID() || nullResize.EffectiveProcessID() != "" || !nullResize.HasSize() || valid || cols != 0 || rows != 0 {
		t.Fatalf("null public resize = %#v cols=%d rows=%d valid=%t", nullResize, cols, rows, valid)
	}
	encodedNullResize, err := json.Marshal(nullResize)
	if err != nil {
		t.Fatalf("marshal null public resize: %v", err)
	}
	var nullResizeFields map[string]json.RawMessage
	if err := json.Unmarshal(encodedNullResize, &nullResizeFields); err != nil {
		t.Fatalf("decode marshaled null public resize: %v", err)
	}
	if string(nullResizeFields["processId"]) != `""` || string(nullResizeFields["size"]) != "null" {
		t.Fatalf("marshaled null public resize = %s", encodedNullResize)
	}

	var terminate CommandExecTerminateParams
	if err := json.Unmarshal([]byte(`{"processId":"","id":"legacy"}`), &terminate); err != nil {
		t.Fatalf("decode public terminate: %v", err)
	}
	if !terminate.HasProcessID() || terminate.EffectiveProcessID() != "" {
		t.Fatalf("terminate effective process id = %q", terminate.EffectiveProcessID())
	}
	encodedTerminate, err := json.Marshal(terminate)
	if err != nil {
		t.Fatalf("marshal public terminate: %v", err)
	}
	var terminateFields map[string]json.RawMessage
	if err := json.Unmarshal(encodedTerminate, &terminateFields); err != nil {
		t.Fatalf("decode marshaled public terminate: %v", err)
	}
	if string(terminateFields["processId"]) != `""` {
		t.Fatalf("marshaled public terminate = %s", encodedTerminate)
	}
}

func TestCommandExecControlWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "command_exec_control_wire_v1.json"))
	if err != nil {
		t.Fatalf("read command exec control fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode command exec control fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s, want %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion, ProtocolVersion, SchemaVersion)
	}
	if len(fixture.Cases) != 11 {
		t.Fatalf("command exec control fixture has %d cases, want 11", len(fixture.Cases))
	}
	bindings := WireTypeBindings()
	seen := make(map[string]bool, len(fixture.Cases))
	for _, fixtureCase := range fixture.Cases {
		if seen[fixtureCase.Name] {
			t.Errorf("duplicate fixture case %q", fixtureCase.Name)
			continue
		}
		seen[fixtureCase.Name] = true
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
		}
		if fixtureCase.ParamsType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ParamsType)
		}
		if fixtureCase.ResultType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ResultType)
		}
	}
}
