package protocol

import "testing"

func TestMethodRegistryCountsAndKeyMethods(t *testing.T) {
	methods := Methods()
	if len(methods) != 224 {
		t.Fatalf("Methods() returned %d entries, want 224", len(methods))
	}

	counts := map[Surface]int{}
	for _, info := range methods {
		counts[info.Surface]++
	}
	wantCounts := map[Surface]int{
		SurfaceClientRequest:      125,
		SurfaceServerNotification: 70,
		SurfaceServerRequest:      11,
		SurfaceClientNotification: 1,
		SurfaceGollemExtension:    17,
	}
	for surface, want := range wantCounts {
		if got := counts[surface]; got != want {
			t.Fatalf("%s count = %d, want %d", surface, got, want)
		}
	}

	assertMethod(t, "initialize", SurfaceClientRequest, MethodImplemented)
	assertMethod(t, "thread/start", SurfaceClientRequest, MethodBlocked)
	assertMethod(t, "model/list", SurfaceClientRequest, MethodImplemented)
	assertMethod(t, "modelProvider/capabilities/read", SurfaceClientRequest, MethodImplemented)
	assertMethod(t, "fs/readFile", SurfaceClientRequest, MethodImplemented)
	assertMethod(t, "git/status", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "turn/retry", SurfaceGollemExtension, MethodBlocked)
	assertMethod(t, "provider/list", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "provider/capabilities/read", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "tool/list", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "cache/stats", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "cache/benchmark", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "fs/watch", SurfaceClientRequest, MethodImplemented)
	assertMethod(t, "fs/unwatch", SurfaceClientRequest, MethodImplemented)
	assertMethod(t, "daemon/status", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "daemon/version", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "daemon/start", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "daemon/stop", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "daemon/restart", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "fs/changed", SurfaceServerNotification, MethodImplemented)
	assertMethod(t, "process/outputDelta", SurfaceServerNotification, MethodImplemented)
	assertMethod(t, "process/exited", SurfaceServerNotification, MethodImplemented)
	assertMethod(t, "thread/status/changed", SurfaceServerNotification, MethodImplemented)
	assertMethod(t, "cache/benchmark/completed", SurfaceServerNotification, MethodImplemented)
	assertMethod(t, "serverRequest/resolved", SurfaceServerNotification, MethodImplemented)
	assertMethod(t, "item/commandExecution/requestApproval", SurfaceServerRequest, MethodImplemented)
	assertMethod(t, "item/fileChange/requestApproval", SurfaceServerRequest, MethodImplemented)
	assertMethod(t, "item/permissions/requestApproval", SurfaceServerRequest, MethodImplemented)
	assertMethod(t, "approval/respond", SurfaceGollemExtension, MethodImplemented)
	assertMethod(t, "feedback/upload", SurfaceClientRequest, MethodNotApplicable)
	assertMethod(t, "initialized", SurfaceClientNotification, MethodImplemented)
}

func TestMethodsReturnsCopy(t *testing.T) {
	methods := Methods()
	methods[0].Method = "mutated"
	if IsKnownMethod("mutated") {
		t.Fatal("Methods returned mutable registry storage")
	}
}

func assertMethod(t *testing.T, method string, surface Surface, state MethodState) {
	t.Helper()
	info, ok := LookupMethod(method)
	if !ok {
		t.Fatalf("method %q not registered", method)
	}
	if info.Surface != surface || info.State != state {
		t.Fatalf("%s = surface %s state %s, want %s/%s", method, info.Surface, info.State, surface, state)
	}
}
