package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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

	action, err := decodeActionRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

func decodeActionRequest(r *http.Request) (agui.Action, error) {
	if requestHasJSONBody(r) {
		return decodeJSONAction(r)
	}
	return decodeFormAction(r)
}

func requestHasJSONBody(r *http.Request) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json")
}

func decodeJSONAction(r *http.Request) (agui.Action, error) {
	defer r.Body.Close()

	var action agui.Action
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&action); err != nil {
		return agui.Action{}, fmt.Errorf("invalid request body: %w", err)
	}
	if err := dec.Decode(new(struct{})); err != io.EOF {
		if err == nil {
			return agui.Action{}, errors.New("invalid request body: multiple JSON values are not allowed")
		}
		return agui.Action{}, fmt.Errorf("invalid request body: %w", err)
	}
	return action, nil
}

func decodeFormAction(r *http.Request) (agui.Action, error) {
	if err := r.ParseForm(); err != nil {
		return agui.Action{}, fmt.Errorf("invalid form body: %w", err)
	}

	action := agui.Action{
		Type:       parseFormActionType(r),
		SessionID:  r.FormValue("session_id"),
		ToolCallID: r.FormValue("tool_call_id"),
		ToolName:   r.FormValue("tool_name"),
		Content:    r.FormValue("content"),
		Message:    firstNonEmptyFormValue(r, "message", "reason"),
	}

	approved, err := parseOptionalBoolFormValue(r, "approved")
	if err != nil {
		return agui.Action{}, err
	}
	action.Approved = approved
	if action.Type == "" && approved != nil {
		if *approved {
			action.Type = agui.ActionApproveToolCall
		} else {
			action.Type = agui.ActionDenyToolCall
		}
	}

	isError, err := parseOptionalBoolFormValue(r, "is_error")
	if err != nil {
		return agui.Action{}, err
	}
	if isError != nil {
		action.IsError = *isError
	}

	lastSeq, err := parseOptionalUint64FormValue(r, "last_seq")
	if err != nil {
		return agui.Action{}, err
	}
	if lastSeq != nil {
		action.LastSeq = *lastSeq
	}

	return action, nil
}

func parseOptionalBoolFormValue(r *http.Request, key string) (*bool, error) {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("invalid form body: %s must be a boolean", key)
	}
	return &parsed, nil
}

func parseOptionalUint64FormValue(r *http.Request, key string) (*uint64, error) {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid form body: %s must be an unsigned integer", key)
	}
	return &parsed, nil
}

func parseFormActionType(r *http.Request) string {
	if actionType := strings.TrimSpace(r.FormValue("type")); actionType != "" {
		return actionType
	}
	if actionType := normalizeFormActionType(firstNonEmptyFormValue(r, "decision", "action")); actionType != "" {
		return actionType
	}
	if firstNonEmptyFormValue(r, "approve") != "" {
		return agui.ActionApproveToolCall
	}
	if firstNonEmptyFormValue(r, "deny", "reject") != "" {
		return agui.ActionDenyToolCall
	}
	if firstNonEmptyFormValue(r, "abort", "abort_session") != "" {
		return agui.ActionAbortSession
	}
	return ""
}

func normalizeFormActionType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case agui.ActionApproveToolCall, "approve", "approved":
		return agui.ActionApproveToolCall
	case agui.ActionDenyToolCall, "deny", "denied", "reject", "rejected":
		return agui.ActionDenyToolCall
	case agui.ActionAbortSession, "abort", "aborted", "cancel", "cancelled", "canceled":
		return agui.ActionAbortSession
	default:
		return ""
	}
}

func firstNonEmptyFormValue(r *http.Request, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(r.FormValue(key)); value != "" {
			return value
		}
	}
	return ""
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
	if runtime != nil {
		return h.config.ApprovalBridge
	}
	return nil
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
