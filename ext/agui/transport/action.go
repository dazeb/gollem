package transport

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/fugue-labs/gollem/ext/agui"
)

// SessionRuntime holds the transport-owned mutable control state for one live
// AGUI session.
type SessionRuntime struct {
	Session        *agui.Session
	ApprovalBridge *agui.ApprovalBridge
	Cancel         func()
}

// SessionRegistry resolves live runtimes by session ID.
type SessionRegistry interface {
	Get(sessionID string) (*SessionRuntime, bool)
}

// MapSessionRegistry is an in-memory SessionRegistry.
type MapSessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*SessionRuntime
}

// NewMapSessionRegistry creates an empty runtime registry.
func NewMapSessionRegistry() *MapSessionRegistry {
	return &MapSessionRegistry{sessions: make(map[string]*SessionRuntime)}
}

// Get returns the runtime for sessionID.
func (r *MapSessionRegistry) Get(sessionID string) (*SessionRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.sessions[sessionID]
	return rt, ok
}

// Set stores runtime under sessionID.
func (r *MapSessionRegistry) Set(sessionID string, runtime *SessionRuntime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sessionID] = runtime
}

// Delete removes sessionID from the registry.
func (r *MapSessionRegistry) Delete(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, sessionID)
}

// CancelStore resolves a live cancel function by session ID.
type CancelStore interface {
	GetCancel(sessionID string) (func(), bool)
}

// MapCancelStore is an in-memory CancelStore.
type MapCancelStore struct {
	mu      sync.RWMutex
	cancels map[string]func()
}

// NewMapCancelStore creates an empty cancel store.
func NewMapCancelStore() *MapCancelStore {
	return &MapCancelStore{cancels: make(map[string]func())}
}

// GetCancel returns the cancel function for sessionID.
func (s *MapCancelStore) GetCancel(sessionID string) (func(), bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cancel, ok := s.cancels[sessionID]
	return cancel, ok
}

// SetCancel stores cancel under sessionID.
func (s *MapCancelStore) SetCancel(sessionID string, cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancels[sessionID] = cancel
}

// DeleteCancel removes the stored cancel function for sessionID.
func (s *MapCancelStore) DeleteCancel(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, sessionID)
}

// ActionHandlerConfig configures the AGUI POST action endpoint.
type ActionHandlerConfig struct {
	// Sessions is the preferred live session source.
	Sessions SessionRegistry

	// Runtimes is a convenience fallback for plain in-memory maps.
	Runtimes map[string]*SessionRuntime

	// ApprovalBridge is a shared fallback bridge when approvals are not stored
	// per session.
	ApprovalBridge *agui.ApprovalBridge

	// ApprovalBridges stores approval bridges by session ID.
	ApprovalBridges map[string]*agui.ApprovalBridge

	// Cancels resolves abort callbacks by session ID.
	Cancels CancelStore

	// CancelFuncs stores abort callbacks by session ID.
	CancelFuncs map[string]func()
}

// ActionHandler handles AGUI POST actions.
type ActionHandler struct {
	config ActionHandlerConfig
}

// NewActionHandler constructs an AGUI action handler.
func NewActionHandler(config ActionHandlerConfig) *ActionHandler {
	return &ActionHandler{config: config}
}

// Handler is a convenience wrapper for NewActionHandler.
func Handler(config ActionHandlerConfig) http.Handler {
	return NewActionHandler(config)
}

// HandleAction is a convenience helper for function-style wiring.
func HandleAction(config ActionHandlerConfig, w http.ResponseWriter, r *http.Request) {
	NewActionHandler(config).ServeHTTP(w, r)
}

// ServeHTTP decodes agui.Action and routes it to approval/abort handling.
func (h *ActionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()

	var action agui.Action
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&action); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if action.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if action.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	switch action.Type {
	case agui.ActionApproveToolCall:
		h.handleApproval(w, action, true)
	case agui.ActionDenyToolCall:
		h.handleApproval(w, action, false)
	case agui.ActionAbortSession:
		h.handleAbort(w, action)
	default:
		writeError(w, http.StatusBadRequest, "unsupported action type: "+action.Type)
	}
}

func (h *ActionHandler) handleApproval(w http.ResponseWriter, action agui.Action, approved bool) {
	if action.ToolCallID == "" {
		writeError(w, http.StatusBadRequest, "tool_call_id is required")
		return
	}

	runtime, sessionOK := h.runtimeForSession(action.SessionID)
	if !sessionOK {
		writeError(w, http.StatusNotFound, "unknown session: "+action.SessionID)
		return
	}

	bridge := h.approvalBridgeForSession(action.SessionID, runtime)
	if bridge == nil {
		writeError(w, http.StatusNotFound, "unknown tool call: "+action.ToolCallID)
		return
	}

	if !bridge.Resolve(action.ToolCallID, approved, action.Message) {
		writeError(w, http.StatusNotFound, "unknown tool call: "+action.ToolCallID)
		return
	}

	writeSuccess(w, successResponse{
		OK:         true,
		Action:     action.Type,
		SessionID:  action.SessionID,
		ToolCallID: action.ToolCallID,
		Message:    action.Message,
	})
}

func (h *ActionHandler) handleAbort(w http.ResponseWriter, action agui.Action) {
	runtime, sessionOK := h.runtimeForSession(action.SessionID)
	if !sessionOK {
		writeError(w, http.StatusNotFound, "unknown session: "+action.SessionID)
		return
	}

	cancel := h.cancelForSession(action.SessionID, runtime)
	if cancel == nil {
		writeError(w, http.StatusNotFound, "unknown session: "+action.SessionID)
		return
	}

	cancel()
	if runtime != nil && runtime.Session != nil {
		runtime.Session.SetStatus(agui.SessionStatusAborted)
	}

	writeSuccess(w, successResponse{
		OK:        true,
		Action:    action.Type,
		SessionID: action.SessionID,
		Message:   "session aborted",
	})
}

func (h *ActionHandler) runtimeForSession(sessionID string) (*SessionRuntime, bool) {
	if h.config.Sessions != nil {
		return h.config.Sessions.Get(sessionID)
	}
	if h.config.Runtimes != nil {
		runtime, ok := h.config.Runtimes[sessionID]
		return runtime, ok
	}
	if _, ok := h.config.ApprovalBridges[sessionID]; ok {
		return nil, true
	}
	if _, ok := h.config.CancelFuncs[sessionID]; ok {
		return nil, true
	}
	if h.config.Cancels != nil {
		if _, ok := h.config.Cancels.GetCancel(sessionID); ok {
			return nil, true
		}
	}
	if h.config.ApprovalBridge != nil {
		return nil, true
	}
	return nil, false
}

func (h *ActionHandler) approvalBridgeForSession(sessionID string, runtime *SessionRuntime) *agui.ApprovalBridge {
	if runtime != nil && runtime.ApprovalBridge != nil {
		return runtime.ApprovalBridge
	}
	if h.config.ApprovalBridges != nil {
		if bridge, ok := h.config.ApprovalBridges[sessionID]; ok {
			return bridge
		}
	}
	return h.config.ApprovalBridge
}

func (h *ActionHandler) cancelForSession(sessionID string, runtime *SessionRuntime) func() {
	if runtime != nil && runtime.Cancel != nil {
		return runtime.Cancel
	}
	if h.config.CancelFuncs != nil {
		if cancel, ok := h.config.CancelFuncs[sessionID]; ok {
			return cancel
		}
	}
	if h.config.Cancels != nil {
		cancel, ok := h.config.Cancels.GetCancel(sessionID)
		if ok {
			return cancel
		}
	}
	return nil
}

type successResponse struct {
	OK         bool   `json:"ok"`
	Action     string `json:"action,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Message    string `json:"message,omitempty"`
}

type errorResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func writeSuccess(w http.ResponseWriter, v successResponse) {
	writeJSON(w, http.StatusOK, v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{OK: false, Error: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
