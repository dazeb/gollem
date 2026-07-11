package protocol

import "testing"

func TestFileChangeApprovalResponseSchemaAndBinding(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{"FileChangeApprovalDecision", "FileChangeRequestApprovalResponse"} {
		if _, ok := defs[name].(Schema); !ok {
			t.Errorf("schema missing %s", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	assertStringEnum(t, defs["FileChangeApprovalDecision"], "accept", "acceptForSession", "decline", "cancel")
	assertSchemaRequired(t, defs["FileChangeRequestApprovalResponse"].(Schema), "decision")
	bindings := WireTypeBindings()
	assertBinding(t, bindings, "item/fileChange/requestApproval", SurfaceServerRequest, "FileChangeApprovalRequestParams")
	assertBinding(t, bindings, "item/fileChange/requestApproval", SurfaceServerRequest, "FileChangeRequestApprovalResponse")
}
