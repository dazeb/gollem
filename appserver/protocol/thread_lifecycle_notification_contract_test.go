package protocol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThreadLifecycleNotificationSchemaAndBindings(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"ThreadArchivedNotification",
		"ThreadClosedNotification",
		"ThreadDeletedNotification",
		"ThreadUnarchivedNotification",
	} {
		definition, ok := defs[name].(Schema)
		if !ok {
			t.Errorf("schema missing %s", name)
			continue
		}
		assertSchemaRequired(t, definition, "threadId")
	}
	if t.Failed() {
		t.FailNow()
	}

	bindings := WireTypeBindings()
	for _, test := range []struct {
		method   string
		typeName string
	}{
		{method: "thread/archived", typeName: "ThreadArchivedNotification"},
		{method: "thread/closed", typeName: "ThreadClosedNotification"},
		{method: "thread/deleted", typeName: "ThreadDeletedNotification"},
		{method: "thread/unarchived", typeName: "ThreadUnarchivedNotification"},
	} {
		assertBinding(t, bindings, test.method, SurfaceServerNotification, test.typeName)
	}
}

func TestThreadLifecycleNotificationWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "thread_lifecycle_notification_wire_v1.json"))
	if err != nil {
		t.Fatalf("read thread lifecycle notification fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode thread lifecycle notification fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s, want %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion, ProtocolVersion, SchemaVersion)
	}
	if len(fixture.Cases) != 4 {
		t.Fatalf("thread lifecycle notification fixture has %d cases, want 4", len(fixture.Cases))
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
		assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ParamsType)
	}
}
