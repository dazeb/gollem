package protocol

// Surface is the JSON-RPC direction or Gollem extension source for a method.
type Surface string

const (
	SurfaceClientRequest      Surface = "client-request"
	SurfaceServerNotification Surface = "server-notification"
	SurfaceServerRequest      Surface = "server-request"
	SurfaceClientNotification Surface = "client-notification"
	SurfaceGollemExtension    Surface = "gollem-extension"
)

// MethodState is the current implementation state of a registered method.
type MethodState string

const (
	MethodImplemented       MethodState = "implemented"
	MethodBlocked           MethodState = "blocked"
	MethodDeferredStub      MethodState = "deferred-stub"
	MethodRenamedEquivalent MethodState = "renamed-equivalent"
	MethodNotApplicable     MethodState = "not-applicable"
)

// MethodInfo describes one schema-known app-server method.
type MethodInfo struct {
	Method  string      `json:"method"`
	Surface Surface     `json:"surface"`
	State   MethodState `json:"state"`
	Source  string      `json:"source,omitempty"`
}

// Methods returns a stable copy of the registered method inventory.
func Methods() []MethodInfo {
	out := make([]MethodInfo, len(methodRegistry))
	copy(out, methodRegistry)
	return out
}

// MethodsBySurface returns registered methods for one surface.
func MethodsBySurface(surface Surface) []MethodInfo {
	var out []MethodInfo
	for _, info := range methodRegistry {
		if info.Surface == surface {
			out = append(out, info)
		}
	}
	return out
}

// LookupMethod returns the first registered method with the given wire name.
func LookupMethod(method string) (MethodInfo, bool) {
	info, ok := methodIndex[method]
	return info, ok
}

// IsKnownMethod reports whether method is in the app-server contract inventory.
func IsKnownMethod(method string) bool {
	_, ok := LookupMethod(method)
	return ok
}
