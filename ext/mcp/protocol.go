package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

const (
	jsonRPCCodeParseError     = -32700
	jsonRPCCodeInvalidRequest = -32600
	jsonRPCCodeMethodNotFound = -32601
	jsonRPCCodeInvalidParams  = -32602
	jsonRPCCodeInternalError  = -32603

	// MCP-specific error codes introduced by the 2026-07-28 specification.
	// HeaderMismatch is returned when a required request header (Mcp-Method or
	// Mcp-Name) is present but does not match the JSON-RPC body.
	jsonRPCCodeHeaderMismatch = -32020
	// MissingRequiredClientCapability is returned when a server needs a client
	// capability the client did not declare in _meta.clientCapabilities. Its
	// data field MUST carry {"requiredCapabilities":[...]}.
	jsonRPCCodeMissingRequiredClientCapability = -32021
	// UnsupportedProtocolVersion is returned when the protocol version in the
	// request _meta or MCP-Protocol-Version header is not supported.
	jsonRPCCodeUnsupportedProtocolVersion = -32022
)

// maxMRTRRounds bounds the number of input_required round trips a client will
// complete for a single logical request before treating the exchange as failed.
const maxMRTRRounds = 8

// HeaderMismatchError constructs a JSON-RPC -32020 error.
func HeaderMismatchError(message string) *jsonRPCError {
	if message == "" {
		message = "header mismatch"
	}
	return &jsonRPCError{
		Code:    jsonRPCCodeHeaderMismatch,
		Message: message,
	}
}

// MissingRequiredClientCapabilityError constructs a JSON-RPC -32021 error whose
// data carries the list of capabilities the server required but the client did
// not declare.
func MissingRequiredClientCapabilityError(required []string) *jsonRPCError {
	data, _ := json.Marshal(map[string]any{"requiredCapabilities": required})
	return &jsonRPCError{
		Code:    jsonRPCCodeMissingRequiredClientCapability,
		Message: "missing required client capability",
		Data:    data,
	}
}

// UnsupportedProtocolVersionError constructs a JSON-RPC -32022 error for the
// given unsupported protocol version. Its data carries the versions this
// implementation supports and the version that was requested, so the client
// can retry with a mutually supported version.
func UnsupportedProtocolVersionError(version string) *jsonRPCError {
	data, _ := json.Marshal(map[string]any{
		"supported": SupportedProtocolVersions,
		"requested": version,
	})
	return &jsonRPCError{
		Code:    jsonRPCCodeUnsupportedProtocolVersion,
		Message: "unsupported protocol version: " + version,
		Data:    data,
	}
}

// Notification is a server-initiated JSON-RPC notification.
type Notification struct {
	Method string
	Params json.RawMessage
}

// NotificationHandler handles a server notification.
type NotificationHandler func(Notification)

// RequestHandler handles a server-initiated JSON-RPC request. In the
// 2026-07-28 protocol sampling, elicitation, and roots are delivered via MRTR
// rather than server-initiated requests, so these handlers are now exercised
// by the client-side MRTR fulfillment path.
type RequestHandler func(context.Context, json.RawMessage) (any, error)

// RootsProvider returns the current set of roots exposed to an MCP server.
//
// Deprecated: Roots are deprecated in the 2026-07-28 protocol (SEP-2577). The
// provider is still invoked when a server sends a roots/list input request via
// MRTR; new clients should avoid advertising roots.
type RootsProvider func(context.Context) ([]Root, error)

// SamplingHandler handles sampling/createMessage requests. In the 2026-07-28
// protocol these arrive as MRTR input requests rather than server-initiated
// JSON-RPC calls.
//
// Deprecated: Sampling is deprecated in the 2026-07-28 protocol (SEP-2577).
type SamplingHandler func(context.Context, *CreateMessageParams) (*CreateMessageResult, error)

// ElicitationHandler handles elicitation/create requests. In the 2026-07-28
// protocol these arrive as MRTR input requests.
type ElicitationHandler func(context.Context, *ElicitationParams) (*ElicitationResult, error)

// ClientConfig configures client-side MCP capabilities exposed to servers.
type ClientConfig struct {
	ClientInfo         *ImplementationInfo
	Capabilities       ClientCapabilities
	RootsProvider      RootsProvider
	SamplingHandler    SamplingHandler
	ElicitationHandler ElicitationHandler
}

// StaticRoots returns a provider that always returns the same roots.
func StaticRoots(roots ...Root) RootsProvider {
	copied := append([]Root(nil), roots...)
	return func(context.Context) ([]Root, error) {
		return append([]Root(nil), copied...), nil
	}
}

// WithTasksExtension returns a ClientConfig that advertises support for the
// io.modelcontextprotocol/tasks extension in _meta.clientCapabilities.extensions.
// Clients MUST advertise the extension before the server will return a task
// (resultType "task") from tools/call, resources/read, or prompts/get.
func WithTasksExtension(cfg ClientConfig) ClientConfig {
	if cfg.Capabilities.Extensions == nil {
		cfg.Capabilities.Extensions = map[string]json.RawMessage{}
	}
	cfg.Capabilities.Extensions[ExtensionTasks] = json.RawMessage(`{}`)
	return cfg
}

func defaultClientConfig() ClientConfig {
	return ClientConfig{
		ClientInfo: &ImplementationInfo{
			Name:    clientName,
			Version: clientVersion,
		},
	}
}

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCMessage is a generic JSON-RPC 2.0 message.
type jsonRPCMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *jsonRPCError    `json:"error,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// rpcCaller sends a single JSON-RPC request and returns the raw result body.
type rpcCaller func(ctx context.Context, method string, params any) (json.RawMessage, error)

type rpcRespond func(context.Context, any, any, *jsonRPCError) error

var samplingCapabilityRegistry sync.Map

type clientState struct {
	mu                   sync.Mutex
	nextID               atomic.Int64
	pending              map[int64]chan *jsonRPCMessage
	notificationHandlers map[string]map[int64]NotificationHandler
	requestHandlers      map[string]RequestHandler
	nextHandlerID        int64
	closed               bool
	protocolVersion      string
	capabilities         ServerCapabilities
	serverInfo           *ServerInfo
	instructions         string
	clientInfo           ImplementationInfo
	clientCapabilities   ClientCapabilities
}

func newClientState(configs ...ClientConfig) *clientState {
	cfg := defaultClientConfig()
	if len(configs) > 0 {
		override := configs[0]
		if override.ClientInfo != nil {
			info := *override.ClientInfo
			cfg.ClientInfo = &info
		}
		cfg.Capabilities = override.Capabilities
		cfg.RootsProvider = override.RootsProvider
		cfg.SamplingHandler = override.SamplingHandler
		cfg.ElicitationHandler = override.ElicitationHandler
	}

	state := &clientState{
		pending:              make(map[int64]chan *jsonRPCMessage),
		notificationHandlers: make(map[string]map[int64]NotificationHandler),
		requestHandlers:      make(map[string]RequestHandler),
		protocolVersion:      protocolVersion,
	}

	if cfg.ClientInfo != nil {
		state.clientInfo = *cfg.ClientInfo
	}
	state.clientCapabilities = resolveClientCapabilities(cfg)

	if cfg.RootsProvider != nil {
		state.requestHandlers["roots/list"] = func(ctx context.Context, _ json.RawMessage) (any, error) {
			roots, err := cfg.RootsProvider(ctx)
			if err != nil {
				return nil, &jsonRPCError{
					Code:    jsonRPCCodeInternalError,
					Message: fmt.Sprintf("mcp: roots provider failed: %v", err),
				}
			}
			return &ListRootsResult{Roots: append([]Root(nil), roots...)}, nil
		}
	}
	if cfg.SamplingHandler != nil {
		state.requestHandlers["sampling/createMessage"] = func(ctx context.Context, raw json.RawMessage) (any, error) {
			var params CreateMessageParams
			if err := json.Unmarshal(raw, &params); err != nil {
				return nil, &jsonRPCError{
					Code:    jsonRPCCodeInvalidParams,
					Message: fmt.Sprintf("mcp: invalid sampling params: %v", err),
				}
			}
			return cfg.SamplingHandler(ctx, &params)
		}
	}
	if cfg.ElicitationHandler != nil {
		state.requestHandlers["elicitation/create"] = func(ctx context.Context, raw json.RawMessage) (any, error) {
			var params ElicitationParams
			if err := json.Unmarshal(raw, &params); err != nil {
				return nil, &jsonRPCError{
					Code:    jsonRPCCodeInvalidParams,
					Message: fmt.Sprintf("mcp: invalid elicitation params: %v", err),
				}
			}
			return cfg.ElicitationHandler(ctx, &params)
		}
	}

	return state
}

func resolveClientCapabilities(cfg ClientConfig) ClientCapabilities {
	caps := cloneClientCapabilities(cfg.Capabilities)
	if cfg.RootsProvider != nil && caps.Roots == nil {
		caps.Roots = &RootsCapability{}
	}
	if cfg.SamplingHandler != nil {
		inferred := samplingCapabilitiesForHandler(cfg.SamplingHandler)
		if caps.Sampling == nil {
			if inferred != nil {
				caps.Sampling = inferred
			} else {
				caps.Sampling = &ClientSamplingCapability{}
			}
		} else if inferred != nil {
			if caps.Sampling.Context == nil && inferred.Context != nil {
				caps.Sampling.Context = &EmptyCapability{}
			}
			if caps.Sampling.Tools == nil && inferred.Tools != nil {
				caps.Sampling.Tools = &EmptyCapability{}
			}
		}
	}
	if cfg.ElicitationHandler != nil && caps.Elicitation == nil {
		caps.Elicitation = &ElicitationCapability{}
	}
	return caps
}

func registerSamplingCapabilities(handler SamplingHandler, caps *ClientSamplingCapability) SamplingHandler {
	if handler == nil || caps == nil {
		return handler
	}
	ptr := reflect.ValueOf(handler).Pointer()
	samplingCapabilityRegistry.Store(ptr, cloneSamplingCapability(caps))
	return handler
}

func samplingCapabilitiesForHandler(handler SamplingHandler) *ClientSamplingCapability {
	if handler == nil {
		return nil
	}
	ptr := reflect.ValueOf(handler).Pointer()
	caps, ok := samplingCapabilityRegistry.Load(ptr)
	if !ok {
		return nil
	}
	typed, ok := caps.(*ClientSamplingCapability)
	if !ok {
		return nil
	}
	return cloneSamplingCapability(typed)
}

func cloneSamplingCapability(in *ClientSamplingCapability) *ClientSamplingCapability {
	if in == nil {
		return nil
	}
	out := *in
	if in.Context != nil {
		empty := *in.Context
		out.Context = &empty
	}
	if in.Tools != nil {
		empty := *in.Tools
		out.Tools = &empty
	}
	return &out
}

func cloneClientCapabilities(in ClientCapabilities) ClientCapabilities {
	out := in
	if in.Roots != nil {
		roots := *in.Roots
		out.Roots = &roots
	}
	if in.Sampling != nil {
		sampling := *in.Sampling
		if in.Sampling.Context != nil {
			empty := *in.Sampling.Context
			sampling.Context = &empty
		}
		if in.Sampling.Tools != nil {
			empty := *in.Sampling.Tools
			sampling.Tools = &empty
		}
		out.Sampling = &sampling
	}
	if in.Elicitation != nil {
		elicitation := *in.Elicitation
		if in.Elicitation.Form != nil {
			empty := *in.Elicitation.Form
			elicitation.Form = &empty
		}
		if in.Elicitation.URL != nil {
			empty := *in.Elicitation.URL
			elicitation.URL = &empty
		}
		out.Elicitation = &elicitation
	}
	if in.Experimental != nil {
		out.Experimental = cloneNestedAnyMap(in.Experimental)
	}
	if in.Extensions != nil {
		out.Extensions = cloneExtensionsMap(in.Extensions)
	}
	return out
}

func cloneNestedAnyMap(in map[string]map[string]any) map[string]map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneAnyMap(v)
	}
	return out
}

func cloneExtensionsMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for k, v := range in {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

// tasksAdvertised reports whether this client advertises the tasks extension in
// its _meta.clientCapabilities.extensions. The low-level round trip treats
// resultType "task" as valid only when this is true.
func (s *clientState) tasksAdvertised() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clientCapabilities.Extensions == nil {
		return false
	}
	_, ok := s.clientCapabilities.Extensions[ExtensionTasks]
	return ok
}

// OnNotification registers a handler for a server notification method.
// Pass an empty method to receive all notifications.
func (s *clientState) OnNotification(method string, handler NotificationHandler) func() {
	key := method
	if key == "" {
		key = "*"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextHandlerID++
	handlerID := s.nextHandlerID
	if s.notificationHandlers[key] == nil {
		s.notificationHandlers[key] = make(map[int64]NotificationHandler)
	}
	s.notificationHandlers[key][handlerID] = handler

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		handlers := s.notificationHandlers[key]
		if handlers == nil {
			return
		}
		delete(handlers, handlerID)
		if len(handlers) == 0 {
			delete(s.notificationHandlers, key)
		}
	}
}

// HandleRequest registers or replaces a handler for a server-initiated request.
// In the 2026-07-28 protocol server-initiated requests are not used for
// sampling/elicitation/roots; this registry is primarily exercised by the MRTR
// fulfillment path. It is retained so a misbehaving server that issues a
// direct request still receives a deterministic response.
func (s *clientState) HandleRequest(method string, handler RequestHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if handler == nil {
		delete(s.requestHandlers, method)
		return
	}
	s.requestHandlers[method] = handler
}

// Capabilities returns the server capabilities observed via server/discover.
func (s *clientState) Capabilities() ServerCapabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneServerCapabilities(s.capabilities)
}

// ClientCapabilities returns the client capabilities this transport advertises.
func (s *clientState) ClientCapabilities() ClientCapabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneClientCapabilities(s.clientCapabilities)
}

// ServerInfo returns the server identity observed via server/discover.
func (s *clientState) ServerInfo() *ServerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.serverInfo == nil {
		return nil
	}
	info := *s.serverInfo
	return &info
}

// ClientInfo returns the client identity advertised on every request.
func (s *clientState) ClientInfo() ImplementationInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clientInfo
}

// Instructions returns the optional server instructions from server/discover.
func (s *clientState) Instructions() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.instructions
}

// ProtocolVersion returns the MCP protocol version this client advertises.
func (s *clientState) ProtocolVersion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.protocolVersion
}

func (s *clientState) setDiscoverResult(result *DiscoverResult) {
	if result == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(result.SupportedVersions) > 0 {
		s.protocolVersion = result.SupportedVersions[0]
	}
	s.capabilities = cloneServerCapabilities(result.Capabilities)
	s.instructions = result.Instructions
	if result.ServerInfo != nil {
		info := *result.ServerInfo
		s.serverInfo = &info
	} else {
		s.serverInfo = nil
	}
}

// requestMeta builds the reserved _meta envelope carried by every stateless
// request. protocolVersion and clientCapabilities are required; clientInfo is
// included when set.
func (s *clientState) requestMeta() RequestMeta {
	s.mu.Lock()
	defer s.mu.Unlock()
	caps := cloneClientCapabilities(s.clientCapabilities)
	meta := RequestMeta{
		ProtocolVersion:    s.protocolVersion,
		ClientCapabilities: &caps,
	}
	if s.clientInfo.Name != "" {
		info := s.clientInfo
		meta.ClientInfo = &info
	}
	return meta
}

func (s *clientState) prepareCall() (int64, chan *jsonRPCMessage, error) {
	id := s.nextID.Add(1)
	ch := make(chan *jsonRPCMessage, 1)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, nil, errors.New("mcp: client is closed")
	}
	s.pending[id] = ch
	return id, ch, nil
}

func (s *clientState) removePending(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, id)
}

func (s *clientState) awaitResponse(ctx context.Context, id int64, ch chan *jsonRPCMessage) (json.RawMessage, error) {
	select {
	case resp, ok := <-ch:
		if !ok || resp == nil {
			return nil, errors.New("mcp: connection closed while waiting for response")
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		s.removePending(id)
		return nil, ctx.Err()
	}
}

func (s *clientState) dispatchMessage(msg *jsonRPCMessage, respond rpcRespond) {
	if msg == nil {
		return
	}

	if msg.Method != "" {
		if hasJSONRPCID(msg.ID) {
			s.dispatchRequest(msg, respond)
			return
		}
		s.dispatchNotification(Notification{
			Method: msg.Method,
			Params: append(json.RawMessage(nil), msg.Params...),
		})
		return
	}

	if !hasJSONRPCID(msg.ID) {
		return
	}

	id, err := parsePendingID(msg.ID)
	if err != nil {
		return
	}

	s.mu.Lock()
	ch, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()

	if ok {
		ch <- msg
	}
}

func (s *clientState) dispatchRequest(msg *jsonRPCMessage, respond rpcRespond) {
	if respond == nil {
		return
	}

	s.mu.Lock()
	handler := s.requestHandlers[msg.Method]
	s.mu.Unlock()

	requestID := normalizeID(msg.ID)
	params := append(json.RawMessage(nil), msg.Params...)

	go func() {
		if handler == nil {
			_ = respond(context.Background(), requestID, nil, &jsonRPCError{
				Code:    jsonRPCCodeMethodNotFound,
				Message: "method not found: " + msg.Method,
			})
			return
		}

		result, err := handler(context.Background(), params)
		if err != nil {
			_ = respond(context.Background(), requestID, nil, rpcErrorFromError(err))
			return
		}
		_ = respond(context.Background(), requestID, result, nil)
	}()
}

func (s *clientState) dispatchNotification(note Notification) {
	s.mu.Lock()
	specific := cloneNotificationHandlers(s.notificationHandlers[note.Method])
	wildcard := cloneNotificationHandlers(s.notificationHandlers["*"])
	s.mu.Unlock()

	for _, handler := range specific {
		go handler(note)
	}
	for _, handler := range wildcard {
		go handler(note)
	}
}

func (s *clientState) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
}

func (s *clientState) failPendingCall(id int64) {
	s.mu.Lock()
	ch, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()

	if ok {
		select {
		case ch <- nil:
		default:
		}
	}
}

// mergeRequestMeta marshals params to a JSON object and injects the reserved
// _meta keys from meta. Existing _meta entries (e.g. progressToken, traceparent)
// are preserved; the reserved keys always override. A nil params yields an
// empty object so every request carries _meta.
func mergeRequestMeta(params any, meta RequestMeta) (json.RawMessage, error) {
	obj := map[string]json.RawMessage{}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(raw)) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			if err := json.Unmarshal(raw, &obj); err != nil {
				return nil, err
			}
		}
	}

	metaRaw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	if existing, ok := obj["_meta"]; ok && len(bytes.TrimSpace(existing)) > 0 {
		var existingMap map[string]json.RawMessage
		if err := json.Unmarshal(existing, &existingMap); err == nil && existingMap != nil {
			var newMeta map[string]json.RawMessage
			if err := json.Unmarshal(metaRaw, &newMeta); err != nil {
				return nil, err
			}
			for k, v := range newMeta {
				existingMap[k] = v
			}
			metaRaw, _ = json.Marshal(existingMap)
		}
	}
	obj["_meta"] = metaRaw
	return json.Marshal(obj)
}

// readResultType extracts the resultType field from a result body. An absent
// resultType is treated as "complete" per the 2026-07-28 spec.
func readResultType(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ResultTypeComplete
	}
	var probe struct {
		ResultType string `json:"resultType"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ResultTypeComplete
	}
	if probe.ResultType == "" {
		return ResultTypeComplete
	}
	return probe.ResultType
}

// stampResultType injects resultType into a marshaled result object if it is
// absent. Used by the server response layer to stamp "complete" on ordinary
// results.
func stampResultType(raw json.RawMessage, rt string) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw
	}
	if _, ok := obj["resultType"]; ok {
		return raw
	}
	obj["resultType"] = mustRawJSON([]byte(`"` + rt + `"`))
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}

// stampServerInfo injects _meta.io.modelcontextprotocol/serverInfo into a
// marshaled result object when absent.
func stampServerInfo(raw json.RawMessage, info ServerInfo) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw
	}
	metaRaw, ok := obj["_meta"]
	if !ok || len(bytes.TrimSpace(metaRaw)) == 0 {
		metaObj := map[string]any{MetaServerInfo: info}
		data, _ := json.Marshal(metaObj)
		obj["_meta"] = data
		out, err := json.Marshal(obj)
		if err != nil {
			return raw
		}
		return out
	}
	var metaMap map[string]json.RawMessage
	if err := json.Unmarshal(metaRaw, &metaMap); err != nil {
		return raw
	}
	if _, exists := metaMap[MetaServerInfo]; !exists {
		serverInfoRaw, _ := json.Marshal(info)
		metaMap[MetaServerInfo] = serverInfoRaw
		newMeta, _ := json.Marshal(metaMap)
		obj["_meta"] = newMeta
		out, err := json.Marshal(obj)
		if err != nil {
			return raw
		}
		return out
	}
	return raw
}

func mustRawJSON(data []byte) json.RawMessage {
	return json.RawMessage(data)
}

// roundTrip sends method with params via call, carrying the stateless _meta
// envelope. When mrtr is true, input_required results drive the MRTR loop:
// each inputRequest is fulfilled via the registered client handlers and the
// original request is retried with a new JSON-RPC id, merging inputResponses
// and echoing requestState verbatim. Non-MRTR methods reject input_required.
// An unrecognized resultType is an error. The final "complete" result body is
// returned for the caller to unmarshal.
func (s *clientState) roundTrip(ctx context.Context, call rpcCaller, method string, params any, mrtr bool) (json.RawMessage, error) {
	baseParams := normalizeParamsMap(params)
	for range maxMRTRRounds {
		paramsRaw, err := mergeRequestMeta(baseParams, s.requestMeta())
		if err != nil {
			return nil, err
		}
		raw, err := call(ctx, method, paramsRaw)
		if err != nil {
			return nil, err
		}
		switch rt := readResultType(raw); rt {
		case ResultTypeComplete:
			return raw, nil
		case ResultTypeInputRequired:
			if !mrtr {
				return nil, fmt.Errorf("mcp: method %q returned input_required but is not MRTR-eligible", method)
			}
			var irr InputRequiredResult
			if err := json.Unmarshal(raw, &irr); err != nil {
				return nil, fmt.Errorf("mcp: failed to parse input_required result: %w", err)
			}
			if len(irr.InputRequests) == 0 && irr.RequestState == "" {
				return nil, errors.New("mcp: input_required result has no inputRequests or requestState")
			}
			responses, err := s.fulfillInputRequests(ctx, irr.InputRequests)
			if err != nil {
				return nil, err
			}
			retry := cloneParamsMap(baseParams)
			if len(responses) > 0 {
				retry["inputResponses"] = responses
			}
			if irr.RequestState != "" {
				retry["requestState"] = irr.RequestState
			}
			baseParams = retry
			continue
		case ResultTypeTask:
			// resultType "task" is valid only when the client advertised the
			// tasks extension; otherwise it is an unexpected result type. The
			// caller is responsible for parsing the CreateTaskResult and
			// polling tasks/get (no auto-poll here).
			if !s.tasksAdvertised() {
				return nil, fmt.Errorf("mcp: unexpected resultType %q for method %q", rt, method)
			}
			return raw, nil
		default:
			return nil, fmt.Errorf("mcp: unrecognized resultType %q for method %q", rt, method)
		}
	}
	return nil, fmt.Errorf("mcp: MRTR round limit (%d) exceeded for method %q", maxMRTRRounds, method)
}

// fulfillInputRequests invokes the registered client handler for each input
// request and marshals the results into InputResponses keyed by the server
// identifiers.
func (s *clientState) fulfillInputRequests(ctx context.Context, reqs InputRequests) (InputResponses, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	s.mu.Lock()
	handlers := make(map[string]RequestHandler, len(reqs))
	for id, ir := range reqs {
		if h, ok := s.requestHandlers[ir.Method]; ok {
			handlers[id] = h
		}
	}
	s.mu.Unlock()

	responses := make(InputResponses, len(reqs))
	for id, ir := range reqs {
		handler, ok := handlers[id]
		if !ok {
			return nil, fmt.Errorf("mcp: no client handler for input request %q (method %q)", id, ir.Method)
		}
		result, err := handler(ctx, ir.Params)
		if err != nil {
			return nil, fmt.Errorf("mcp: input request %q failed: %w", id, err)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("mcp: failed to marshal input response %q: %w", id, err)
		}
		responses[id] = raw
	}
	return responses, nil
}

func normalizeParamsMap(params any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	switch v := params.(type) {
	case map[string]any:
		return cloneParamsMap(v)
	case json.RawMessage:
		if len(bytes.TrimSpace(v)) == 0 {
			return map[string]any{}
		}
		var out map[string]any
		if err := json.Unmarshal(v, &out); err != nil {
			return map[string]any{}
		}
		return out
	default:
		raw, err := json.Marshal(params)
		if err != nil {
			return map[string]any{}
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return map[string]any{}
		}
		return out
	}
}

func cloneParamsMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// discover calls server/discover and stores the result on the client state.
func discover(ctx context.Context, call rpcCaller, state *clientState) (*DiscoverResult, error) {
	raw, err := state.roundTrip(ctx, call, "server/discover", nil, false)
	if err != nil {
		return nil, fmt.Errorf("mcp: server/discover failed: %w", err)
	}
	var result DiscoverResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse discover result: %w", err)
	}
	state.setDiscoverResult(&result)
	return &result, nil
}

func listTools(ctx context.Context, call rpcCaller, state *clientState) ([]Tool, error) {
	raw, err := state.roundTrip(ctx, call, "tools/list", nil, false)
	if err != nil {
		return nil, err
	}
	var list listToolsResult
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse tools list: %w", err)
	}
	return list.Tools, nil
}

func callTool(ctx context.Context, call rpcCaller, state *clientState, name string, args map[string]any) (*ToolResult, error) {
	raw, err := callToolRaw(ctx, call, state, name, args)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/call failed: %w", err)
	}
	if rt := readResultType(raw); rt == ResultTypeTask {
		return nil, errors.New("mcp: tools/call returned a task; use CallToolAwait to poll it")
	}
	var toolResult ToolResult
	if err := json.Unmarshal(raw, &toolResult); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse tool result: %w", err)
	}
	return &toolResult, nil
}

// callToolRaw issues tools/call and returns the raw result body. Callers inspect
// the resultType to distinguish a synchronous ToolResult (resultType "complete")
// from a CreateTaskResult (resultType "task") and poll accordingly.
func callToolRaw(ctx context.Context, call rpcCaller, state *clientState, name string, args map[string]any) (json.RawMessage, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	return state.roundTrip(ctx, call, "tools/call", params, true)
}

// getTask calls tasks/get and returns the current task state.
func getTask(ctx context.Context, call rpcCaller, state *clientState, taskID string) (*Task, error) {
	raw, err := state.roundTrip(ctx, call, "tasks/get", map[string]any{"taskId": taskID}, false)
	if err != nil {
		return nil, fmt.Errorf("mcp: tasks/get failed: %w", err)
	}
	var task Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse task result: %w", err)
	}
	return &task, nil
}

// updateTask calls tasks/update with the client's input responses.
func updateTask(ctx context.Context, call rpcCaller, state *clientState, taskID string, responses InputResponses) error {
	params := map[string]any{"taskId": taskID}
	if len(responses) > 0 {
		params["inputResponses"] = responses
	}
	if _, err := state.roundTrip(ctx, call, "tasks/update", params, false); err != nil {
		return fmt.Errorf("mcp: tasks/update failed: %w", err)
	}
	return nil
}

// cancelTask calls tasks/cancel for the given task id.
func cancelTask(ctx context.Context, call rpcCaller, state *clientState, taskID string) error {
	if _, err := state.roundTrip(ctx, call, "tasks/cancel", map[string]any{"taskId": taskID}, false); err != nil {
		return fmt.Errorf("mcp: tasks/cancel failed: %w", err)
	}
	return nil
}

func listResources(ctx context.Context, call rpcCaller, state *clientState) ([]Resource, error) {
	raw, err := state.roundTrip(ctx, call, "resources/list", nil, false)
	if err != nil {
		return nil, err
	}
	var list listResourcesResult
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse resources list: %w", err)
	}
	return list.Resources, nil
}

func readResource(ctx context.Context, call rpcCaller, state *clientState, uri string) (*ReadResourceResult, error) {
	raw, err := state.roundTrip(ctx, call, "resources/read", map[string]any{"uri": uri}, true)
	if err != nil {
		return nil, fmt.Errorf("mcp: resources/read failed: %w", err)
	}
	var readResult ReadResourceResult
	if err := json.Unmarshal(raw, &readResult); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse resource contents: %w", err)
	}
	return &readResult, nil
}

func listResourceTemplates(ctx context.Context, call rpcCaller, state *clientState) ([]ResourceTemplate, error) {
	raw, err := state.roundTrip(ctx, call, "resources/templates/list", nil, false)
	if err != nil {
		return nil, err
	}
	var list listResourceTemplatesResult
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse resource templates list: %w", err)
	}
	return list.ResourceTemplates, nil
}

func listPrompts(ctx context.Context, call rpcCaller, state *clientState) ([]Prompt, error) {
	raw, err := state.roundTrip(ctx, call, "prompts/list", nil, false)
	if err != nil {
		return nil, err
	}
	var list listPromptsResult
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse prompts list: %w", err)
	}
	return list.Prompts, nil
}

func getPrompt(ctx context.Context, call rpcCaller, state *clientState, name string, args map[string]string) (*PromptResult, error) {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	}
	raw, err := state.roundTrip(ctx, call, "prompts/get", params, true)
	if err != nil {
		return nil, fmt.Errorf("mcp: prompts/get failed: %w", err)
	}
	var promptResult PromptResult
	if err := json.Unmarshal(raw, &promptResult); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse prompt result: %w", err)
	}
	return &promptResult, nil
}

func cloneServerCapabilities(in ServerCapabilities) ServerCapabilities {
	out := in
	if in.Tools != nil {
		tools := *in.Tools
		out.Tools = &tools
	}
	if in.Prompts != nil {
		prompts := *in.Prompts
		out.Prompts = &prompts
	}
	if in.Resources != nil {
		resources := *in.Resources
		out.Resources = &resources
	}
	if in.Experimental != nil {
		out.Experimental = cloneAnyMap(in.Experimental)
	}
	return out
}

func cloneNotificationHandlers(in map[int64]NotificationHandler) []NotificationHandler {
	if len(in) == 0 {
		return nil
	}
	out := make([]NotificationHandler, 0, len(in))
	for _, handler := range in {
		out = append(out, handler)
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func rpcErrorFromError(err error) *jsonRPCError {
	if err == nil {
		return nil
	}
	var rpcErr *jsonRPCError
	if errors.As(err, &rpcErr) {
		return rpcErr
	}
	return &jsonRPCError{
		Code:    jsonRPCCodeInternalError,
		Message: err.Error(),
	}
}

func hasJSONRPCID(raw *json.RawMessage) bool {
	if raw == nil {
		return false
	}
	trimmed := bytes.TrimSpace(*raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

func parsePendingID(raw *json.RawMessage) (int64, error) {
	if raw == nil {
		return 0, errors.New("missing JSON-RPC id")
	}
	var id int64
	if err := json.Unmarshal(*raw, &id); err == nil {
		return id, nil
	}
	return 0, fmt.Errorf("unsupported response id: %s", string(*raw))
}

// normalizeID converts the raw JSON id to a concrete type for serialization.
func normalizeID(raw *json.RawMessage) any {
	if raw == nil {
		return nil
	}

	var intID int64
	if err := json.Unmarshal(*raw, &intID); err == nil {
		return intID
	}

	var floatID float64
	if err := json.Unmarshal(*raw, &floatID); err == nil {
		return floatID
	}

	var strID string
	if err := json.Unmarshal(*raw, &strID); err == nil {
		return strID
	}

	var rawCopy json.RawMessage
	rawCopy = append(rawCopy, (*raw)...)
	return rawCopy
}

func rawJSONID(value any) *json.RawMessage {
	data, _ := json.Marshal(value)
	raw := json.RawMessage(data)
	return &raw
}
