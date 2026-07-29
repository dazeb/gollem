package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// ServerToolHandler handles an MCP tools/call request.
type ServerToolHandler func(context.Context, *RequestContext, map[string]any) (*ToolResult, error)

// ResourceReadHandler handles an MCP resources/read request.
type ResourceReadHandler func(context.Context, *RequestContext, string) (*ReadResourceResult, error)

// PromptGetHandler handles an MCP prompts/get request.
type PromptGetHandler func(context.Context, *RequestContext, string, map[string]string) (*PromptResult, error)

type serverTool struct {
	definition Tool
	handler    ServerToolHandler
}

// RequestContext represents a single client request being serviced by an MCP
// server. In the 2026-07-28 stateless protocol each request self-describes its
// client capabilities and identity via _meta; the server MUST NOT infer state
// from prior requests on the same connection.
//
// MRTR: a tool/resource/prompt handler that needs additional information from
// the client calls NeedInput and then returns a nil result. The dispatch layer
// detects the recorded *InputRequiredResult and marshals it with resultType
// "input_required". On retry, the client merges inputResponses and echoes
// requestState; handlers consume them via InputResponses and RequestState.
type RequestContext struct {
	server        *Server
	meta          RequestMeta
	cont          MRTRContinuation
	headers       map[string]string
	inputRequired *InputRequiredResult
	taskCreated   *CreateTaskResult
	requestID     any
}

// RequestID returns the JSON-RPC id of the request this context services.
func (rc *RequestContext) RequestID() any {
	if rc == nil {
		return nil
	}
	return rc.requestID
}

// ClientCapabilities returns the capabilities advertised by the client for this
// request (from _meta.clientCapabilities).
func (rc *RequestContext) ClientCapabilities() ClientCapabilities {
	if rc == nil {
		return ClientCapabilities{}
	}
	if rc.meta.ClientCapabilities != nil {
		return cloneClientCapabilities(*rc.meta.ClientCapabilities)
	}
	return ClientCapabilities{}
}

// ClientInfo returns the client identity advertised for this request, if any.
func (rc *RequestContext) ClientInfo() *ImplementationInfo {
	if rc == nil || rc.meta.ClientInfo == nil {
		return nil
	}
	info := *rc.meta.ClientInfo
	return &info
}

// ProtocolVersion returns the protocol version the client used for this request.
func (rc *RequestContext) ProtocolVersion() string {
	if rc == nil {
		return ""
	}
	return rc.meta.ProtocolVersion
}

// LogLevel returns the per-request log level the client requested via _meta, if
// any. Servers MUST NOT emit notifications/message for requests without it.
func (rc *RequestContext) LogLevel() string {
	if rc == nil {
		return ""
	}
	return rc.meta.LogLevel
}

// ClientSupports reports whether the client declared the given capability in
// this request's _meta.clientCapabilities. Capability is one of "roots",
// "sampling", or "elicitation".
func (rc *RequestContext) ClientSupports(capability string) bool {
	caps := rc.ClientCapabilities()
	switch capability {
	case "roots":
		return caps.Roots != nil
	case "sampling":
		return caps.Sampling != nil
	case "elicitation":
		return caps.Elicitation != nil
	default:
		return false
	}
}

// InputResponses returns the answers the client supplied on an MRTR retry, keyed
// by the server-assigned input request identifiers. Empty on a first request.
func (rc *RequestContext) InputResponses() InputResponses {
	if rc == nil {
		return nil
	}
	return rc.cont.InputResponses
}

// RequestState returns the opaque requestState the client echoed verbatim from
// the prior InputRequiredResult. It is attacker-controlled; higher layers are
// responsible for any integrity verification. Empty on a first request.
func (rc *RequestContext) RequestState() string {
	if rc == nil {
		return ""
	}
	return rc.cont.RequestState
}

// XHeader returns the value of an x-mcp-* request header (case-insensitive),
// or empty if not present. Tool handlers use this to read SEP-2243 custom
// headers passed from tool params.
func (rc *RequestContext) XHeader(name string) string {
	if rc == nil || rc.headers == nil {
		return ""
	}
	for k, v := range rc.headers {
		if equalFoldASCII(k, name) {
			return v
		}
	}
	return ""
}

// NeedInput records an *InputRequiredResult on the request context. A handler
// calls it when it needs the client to fulfill input requests, then returns a
// nil result; the dispatch layer detects the recorded value and marshals it
// with resultType "input_required". The server MUST NOT include an input
// request for a capability the client did not declare; the response layer
// validates this and returns MissingRequiredClientCapability if violated.
// The returned pointer is the recorded value, for inspection or logging.
func (rc *RequestContext) NeedInput(inputRequests InputRequests, requestState string) *InputRequiredResult {
	rc.inputRequired = &InputRequiredResult{
		ResultType:    ResultTypeInputRequired,
		InputRequests: inputRequests,
		RequestState:  requestState,
	}
	return rc.inputRequired
}

// ClientSupportsExtension reports whether the client declared support for the
// given MCP extension in _meta.clientCapabilities.extensions. ext is an
// extension identifier such as ExtensionTasks.
func (rc *RequestContext) ClientSupportsExtension(ext string) bool {
	if rc == nil {
		return false
	}
	caps := rc.ClientCapabilities()
	if caps.Extensions == nil {
		return false
	}
	_, ok := caps.Extensions[ext]
	return ok
}

// CreateTask mints a task in the server's task store and returns a
// CreateTaskResult (resultType "task") for the client to poll. It is only
// allowed when the client declared support for the tasks extension; otherwise
// it returns an error so the handler falls back to a synchronous result. The
// handler returns the *CreateTaskResult directly; the dispatch layer stamps
// serverInfo and leaves resultType "task" intact.
func (rc *RequestContext) CreateTask(initial Task) (*CreateTaskResult, error) {
	if rc == nil || rc.server == nil {
		return nil, errors.New("mcp: no server bound to request context")
	}
	if !rc.server.tasksEnabled {
		return nil, errors.New("mcp: tasks extension not enabled on this server")
	}
	if !rc.ClientSupportsExtension(ExtensionTasks) {
		return nil, errors.New("mcp: client did not declare the tasks extension")
	}
	if initial.TaskID == "" {
		initial.TaskID = newTaskID()
	}
	if initial.Status == "" {
		initial.Status = TaskStatusWorking
	}
	rc.server.tasks.put(&taskEntry{task: initial})
	result := &CreateTaskResult{ResultType: ResultTypeTask, Task: initial}
	rc.taskCreated = result
	return result, nil
}

// Server is a stateless MCP server with tool/resource/prompt registries. The
// 2026-07-28 protocol removes the initialize handshake and sessions: each
// request is self-describing and servers MUST NOT carry state between requests.
type Server struct {
	mu sync.Mutex
	wg sync.WaitGroup

	nextID  atomic.Int64
	pending map[int64]chan *jsonRPCMessage

	writeMu sync.Mutex
	writeFn func([]byte) error
	closed  bool
	peerEOF bool

	tools             []serverTool
	resources         []Resource
	resourceTemplates []ResourceTemplate
	resourceReader    ResourceReadHandler
	prompts           []Prompt
	promptGetter      PromptGetHandler

	serverInfo      ServerInfo
	instructions    string
	protocol        string
	listCacheTTLMs  int64
	listCacheScope  string
	listCacheConfig bool

	hub          *subscriptionHub
	tasksEnabled bool
	tasks        *taskStore
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithServerInfo sets the server identity echoed in result _meta and discover.
func WithServerInfo(info ServerInfo) ServerOption {
	return func(s *Server) {
		s.serverInfo = info
	}
}

// WithServerInstructions sets the instructions returned by server/discover.
func WithServerInstructions(instructions string) ServerOption {
	return func(s *Server) {
		s.instructions = instructions
	}
}

// WithListCache sets the default cache freshness hints emitted on tools/list,
// prompts/list, resources/list, resources/templates/list, and resources/read
// results. ttlMs is the freshness hint in milliseconds; scope is "public" or
// "private". Defaults are ttlMs=60000 and scope="public".
func WithListCache(ttlMs int64, scope string) ServerOption {
	return func(s *Server) {
		s.listCacheTTLMs = ttlMs
		s.listCacheConfig = true
		if scope != "" {
			s.listCacheScope = scope
		}
	}
}

// WithTasks enables the io.modelcontextprotocol/tasks extension on the server.
// When enabled, the server advertises the extension in server/discover and
// RequestContext.CreateTask may be used by tool/resource/prompt handlers to
// return long-running tasks. Default off so existing servers are unaffected.
func WithTasks() ServerOption {
	return func(s *Server) {
		s.tasksEnabled = true
	}
}

// NewServer constructs a reusable stateless MCP server.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		pending:        make(map[int64]chan *jsonRPCMessage),
		serverInfo:     ServerInfo{Name: "gollem-mcp-server", Version: "1.0.0"},
		protocol:       ProtocolVersion,
		listCacheTTLMs: defaultListCacheTTLMs,
		listCacheScope: CacheScopePublic,
		hub:            newSubscriptionHub(),
		tasks:          newTaskStore(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// defaultListCacheTTLMs is the default freshness hint (ms) for list/read
// results when WithListCache has not been used.
const defaultListCacheTTLMs = 60000

// SetListCache configures the default cache freshness hints for list/read
// results. ttlMs is a freshness hint in milliseconds; scope is "public" or
// "private" (empty defaults to "public").
func (s *Server) SetListCache(ttlMs int64, scope string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCacheTTLMs = ttlMs
	if scope != "" {
		s.listCacheScope = scope
	} else {
		s.listCacheScope = CacheScopePublic
	}
	s.listCacheConfig = true
}

func (s *Server) cacheConfigLocked() (int64, string) {
	ttl := s.listCacheTTLMs
	scope := s.listCacheScope
	if scope == "" {
		scope = CacheScopePublic
	}
	return ttl, scope
}

func (s *Server) cacheableLocked() CacheableResult {
	ttl, scope := s.cacheConfigLocked()
	out := CacheableResult{CacheScope: scope}
	if ttl > 0 {
		v := ttl
		out.TTLMs = &v
	}
	return out
}

// AddTool registers or replaces a server tool.
func (s *Server) AddTool(tool Tool, handler ServerToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tools {
		if s.tools[i].definition.Name == tool.Name {
			s.tools[i] = serverTool{definition: tool, handler: handler}
			return
		}
	}
	s.tools = append(s.tools, serverTool{definition: tool, handler: handler})
}

// SetResources configures server resources and the optional read handler.
func (s *Server) SetResources(resources []Resource, templates []ResourceTemplate, reader ResourceReadHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources = append([]Resource(nil), resources...)
	s.resourceTemplates = append([]ResourceTemplate(nil), templates...)
	s.resourceReader = reader
}

// SetPrompts configures server prompts and the optional prompts/get handler.
func (s *Server) SetPrompts(prompts []Prompt, getter PromptGetHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append([]Prompt(nil), prompts...)
	s.promptGetter = getter
}

// ServerInfo returns the server identity.
func (s *Server) ServerInfo() ServerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serverInfo
}

// ProtocolVersion returns the protocol version this server speaks.
func (s *Server) ProtocolVersion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.protocol == "" {
		return ProtocolVersion
	}
	return s.protocol
}

// NotifyToolsListChanged emits notifications/tools/list_changed to every
// active subscriptions/listen subscriber that opted in to tools list changes.
func (s *Server) NotifyToolsListChanged() {
	s.hub.emit("notifications/tools/list_changed", nil, func(f SubscriptionFilter) bool {
		return f.ToolsListChanged
	})
}

// NotifyResourcesListChanged emits notifications/resources/list_changed to
// every active subscriber that opted in to resources list changes.
func (s *Server) NotifyResourcesListChanged() {
	s.hub.emit("notifications/resources/list_changed", nil, func(f SubscriptionFilter) bool {
		return f.ResourcesListChanged
	})
}

// NotifyPromptsListChanged emits notifications/prompts/list_changed to every
// active subscriber that opted in to prompts list changes.
func (s *Server) NotifyPromptsListChanged() {
	s.hub.emit("notifications/prompts/list_changed", nil, func(f SubscriptionFilter) bool {
		return f.PromptsListChanged
	})
}

// NotifyResourceUpdated emits notifications/resources/updated for uri to every
// active subscriber whose resourceSubscriptions contains uri.
func (s *Server) NotifyResourceUpdated(uri string) {
	s.hub.emit("notifications/resources/updated", map[string]any{"uri": uri}, func(f SubscriptionFilter) bool {
		return containsString(f.ResourceSubscriptions, uri)
	})
}

// UpdateTask advances a task in the store. The mutate callback receives the
// current task and may mutate it in place (e.g. set Status to completed and
// populate Result). It is the application-facing API for driving long-running
// tasks created via RequestContext.CreateTask. Unknown task ids are ignored.
func (s *Server) UpdateTask(taskID string, mutate func(*Task)) {
	if mutate == nil {
		return
	}
	entry, ok := s.tasks.get(taskID)
	if !ok {
		return
	}
	entry.update(mutate)
	// TODO(mcp-2026): notifications/tasks over subscriptions when an active
	// subscription opts in. Until then clients poll tasks/get.
}

// HandleMessage handles a single JSON-RPC message. Request dispatch is
// asynchronous so the read loop continues while a handler runs. Used by
// framed (stdio) transports that own the response writer.
func (s *Server) HandleMessage(ctx context.Context, msg *jsonRPCMessage) {
	if msg == nil {
		return
	}
	if msg.Method != "" {
		if hasJSONRPCID(msg.ID) {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handleRequest(ctx, msg)
			}()
			return
		}
		s.handleNotification(msg)
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

func (s *Server) handleNotification(msg *jsonRPCMessage) {
	// The 2026-07-28 protocol removes notifications/initialized. The remaining
	// server-side notification of interest is notifications/cancelled, which a
	// stdio client uses to cancel an active subscriptions/listen stream.
	if msg.Method == "notifications/cancelled" {
		s.handleSubscriptionCancel(msg.Params)
	}
}

// handleSubscriptionCancel deregisters the subscriptions/listen stream whose
// JSON-RPC id matches the cancelled request id and sends it the graceful
// closure response. Used by stdio where the client cancels via notification.
func (s *Server) handleSubscriptionCancel(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var params struct {
		RequestID *json.RawMessage `json:"requestId"`
	}
	if err := json.Unmarshal(raw, &params); err != nil || params.RequestID == nil {
		return
	}
	id := normalizeID(params.RequestID)
	key := idKeyString(id)
	s.hub.mu.Lock()
	sub, ok := s.hub.subs[key]
	if ok {
		delete(s.hub.subs, key)
	}
	s.hub.mu.Unlock()
	if ok && sub.deliver != nil {
		sub.deliver(subscriptionClosureResponse(id))
	}
}

func (s *Server) handleRequest(ctx context.Context, msg *jsonRPCMessage) {
	resp, streaming := s.dispatchRequestStreaming(ctx, msg, nil)
	if streaming {
		// subscriptions/listen owns its own response stream; do not write an
		// inline JSON-RPC response here.
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		_ = s.writeResponse(resp.ID, nil, rpcErrorFromError(err))
		return
	}
	_ = s.writeJSON(data)
}

// dispatchRequest validates the request _meta, dispatches the method, and
// returns a marshaled response body (with resultType and serverInfo stamped on
// success). It is shared by the async framed path and the synchronous HTTP
// path so both honor the same validation and stamping rules. headers carries
// optional x-mcp-* request headers made available to tool handlers via
// RequestContext.XHeader. The streaming return is true when the method is
// subscriptions/listen on a framed transport: the stream owns the response and
// the caller MUST NOT write an inline response.
func (s *Server) dispatchRequestWithHeaders(ctx context.Context, msg *jsonRPCMessage, headers map[string]string) jsonRPCResponse {
	resp, _ := s.dispatchRequestStreaming(ctx, msg, headers)
	return resp
}

func (s *Server) dispatchRequestStreaming(ctx context.Context, msg *jsonRPCMessage, headers map[string]string) (jsonRPCResponse, bool) {
	requestID := normalizeID(msg.ID)

	meta, metaErr := parseRequestMeta(msg.Params)
	if metaErr != nil {
		return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: metaErr}, false
	}

	cont := parseContinuation(msg.Params)
	rc := &RequestContext{
		server:    s,
		meta:      meta,
		cont:      cont,
		headers:   headers,
		requestID: requestID,
	}

	result, rpcErr := s.dispatchMethod(ctx, rc, msg.Method, msg.Params)
	if rpcErr != nil {
		return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: rpcErr}, false
	}

	if _, ok := result.(*subscriptionStreamMarker); ok {
		// The subscription stream has been registered and the ack has been
		// sent on the shared channel. The graceful-closure response is sent
		// later by the transport / cancel handler.
		return jsonRPCResponse{JSONRPC: "2.0", ID: requestID}, true
	}

	return s.buildSuccessResponse(requestID, rc, result), false
}

// HandleRequestSync processes a single JSON-RPC request synchronously and
// returns the marshaled JSON-RPC response (with resultType and serverInfo
// stamped on success). Used by stateless transports (HTTP) that return the
// response inline. headers carries optional x-mcp-* request headers.
func (s *Server) HandleRequestSync(ctx context.Context, msg *jsonRPCMessage, headers map[string]string) json.RawMessage {
	resp := s.dispatchRequestWithHeaders(ctx, msg, headers)
	data, err := json.Marshal(resp)
	if err != nil {
		errResp := jsonRPCResponse{JSONRPC: "2.0", ID: resp.ID, Error: rpcErrorFromError(err)}
		data, _ := json.Marshal(errResp)
		return data
	}
	return data
}

// buildSuccessResponse stamps resultType and serverInfo onto a marshaled
// result. *InputRequiredResult results keep resultType "input_required" and are
// checked for required-capability compliance: a server MUST NOT request a
// capability the client did not declare in _meta.clientCapabilities.
// *CreateTaskResult results keep resultType "task" (the tasks extension).
func (s *Server) buildSuccessResponse(requestID any, rc *RequestContext, result any) jsonRPCResponse {
	if result == nil {
		result = struct{}{}
	}
	info := s.ServerInfo()

	if irr, ok := result.(*InputRequiredResult); ok {
		if missing := missingRequiredCapabilities(rc, irr.InputRequests); len(missing) > 0 {
			return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: MissingRequiredClientCapabilityError(missing)}
		}
		raw, err := json.Marshal(irr)
		if err != nil {
			return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: rpcErrorFromError(err)}
		}
		raw = stampServerInfo(raw, info)
		return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Result: raw}
	}

	if ctr, ok := result.(*CreateTaskResult); ok {
		// The server MUST NOT return a task to a client that did not declare
		// the tasks extension; CreateTask already enforces this, but defend
		// in depth by rejecting any task result to a non-declaring client.
		if !rc.ClientSupportsExtension(ExtensionTasks) {
			return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: &jsonRPCError{
				Code:    jsonRPCCodeInvalidParams,
				Message: "mcp: server returned a task but the client did not declare the tasks extension",
			}}
		}
		raw, err := json.Marshal(ctr)
		if err != nil {
			return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: rpcErrorFromError(err)}
		}
		raw = stampServerInfo(raw, info)
		return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Result: raw}
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: rpcErrorFromError(err)}
	}
	raw = stampResultType(raw, ResultTypeComplete)
	raw = stampServerInfo(raw, info)
	return jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Result: raw}
}

func (s *Server) dispatchMethod(ctx context.Context, rc *RequestContext, method string, raw json.RawMessage) (any, *jsonRPCError) {
	switch method {
	case "server/discover":
		return s.handleDiscover(), nil
	case "tools/list":
		return s.handleToolsList(), nil
	case "tools/call":
		return s.handleToolsCall(ctx, rc, raw)
	case "resources/list":
		return s.handleResourcesList(), nil
	case "resources/read":
		return s.handleResourcesRead(ctx, rc, raw)
	case "resources/templates/list":
		return s.handleResourceTemplatesList(), nil
	case "prompts/list":
		return s.handlePromptsList(), nil
	case "prompts/get":
		return s.handlePromptGet(ctx, rc, raw)
	case "subscriptions/listen":
		return s.handleSubscriptionsListen(rc, raw)
	case "tasks/get":
		return s.handleTaskGet(rc, raw)
	case "tasks/update":
		return s.handleTaskUpdate(rc, raw)
	case "tasks/cancel":
		return s.handleTaskCancel(rc, raw)
	default:
		return nil, &jsonRPCError{
			Code:    jsonRPCCodeMethodNotFound,
			Message: "method not found: " + method,
		}
	}
}

// handleSubscriptionsListen registers a subscription on the shared (stdio)
// channel. The ack is sent immediately as the first message; the
// graceful-closure response is sent on notifications/cancelled or connection
// drop. On the HTTP transport the POST handler intercepts subscriptions/listen
// and streams SSE directly, so this path only runs for framed transports.
func (s *Server) handleSubscriptionsListen(rc *RequestContext, raw json.RawMessage) (any, *jsonRPCError) {
	filter := parseSubscribeParams(raw)
	id := rc.requestID
	ack := ackMessage(id, filter)
	if err := s.writeJSON(ack); err != nil {
		return nil, &jsonRPCError{Code: jsonRPCCodeInternalError, Message: "mcp: failed to send subscription ack: " + err.Error()}
	}
	sub := &subscription{
		id:     id,
		idKey:  idKeyString(id),
		filter: filter,
		deliver: func(data []byte) {
			_ = s.writeJSON(data)
		},
	}
	s.hub.register(sub)
	return &subscriptionStreamMarker{}, nil
}

func (s *Server) handleDiscover() *DiscoverResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &DiscoverResult{
		SupportedVersions: append([]string(nil), SupportedProtocolVersions...),
		Capabilities:      s.serverCapabilitiesLocked(),
		ServerInfo:        cloneServerInfo(s.serverInfo),
		Instructions:      s.instructions,
		CacheableResult:   s.cacheableLocked(),
	}
}

func (s *Server) handleToolsList() *listToolsResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	tools := make([]Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool.definition)
	}
	return &listToolsResult{
		Tools:           tools,
		CacheableResult: s.cacheableLocked(),
	}
}

func (s *Server) handleToolsCall(ctx context.Context, rc *RequestContext, raw json.RawMessage) (any, *jsonRPCError) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: fmt.Sprintf("invalid tools/call params: %v", err),
		}
	}

	s.mu.Lock()
	var entry *serverTool
	for i := range s.tools {
		if s.tools[i].definition.Name == params.Name {
			tool := s.tools[i]
			entry = &tool
			break
		}
	}
	s.mu.Unlock()

	if entry == nil || entry.handler == nil {
		return nil, &jsonRPCError{
			Code:    jsonRPCCodeMethodNotFound,
			Message: "unknown tool: " + params.Name,
		}
	}

	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}

	result, err := entry.handler(ctx, rc, params.Arguments)
	if err != nil {
		return nil, rpcErrorFromError(err)
	}
	if rc.inputRequired != nil {
		return rc.inputRequired, nil
	}
	if rc.taskCreated != nil {
		return rc.taskCreated, nil
	}
	if result == nil {
		return &ToolResult{Content: []Content{{Type: "text", Text: ""}}}, nil
	}
	return result, nil
}

func (s *Server) handleResourcesList() *listResourcesResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &listResourcesResult{
		Resources:       append([]Resource(nil), s.resources...),
		CacheableResult: s.cacheableLocked(),
	}
}

func (s *Server) handleResourcesRead(ctx context.Context, rc *RequestContext, raw json.RawMessage) (any, *jsonRPCError) {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: fmt.Sprintf("invalid resources/read params: %v", err),
		}
	}

	s.mu.Lock()
	reader := s.resourceReader
	s.mu.Unlock()
	if reader == nil {
		return nil, &jsonRPCError{
			Code:    jsonRPCCodeMethodNotFound,
			Message: "resources/read not supported",
		}
	}

	result, err := reader(ctx, rc, params.URI)
	if err != nil {
		return nil, rpcErrorFromError(err)
	}
	if rc.inputRequired != nil {
		return rc.inputRequired, nil
	}
	if rc.taskCreated != nil {
		return rc.taskCreated, nil
	}
	if result == nil {
		result = &ReadResourceResult{}
	}
	s.mu.Lock()
	cache := s.cacheableLocked()
	s.mu.Unlock()
	if result.TTLMs == nil {
		result.TTLMs = cache.TTLMs
	}
	if result.CacheScope == "" {
		result.CacheScope = cache.CacheScope
	}
	return result, nil
}

func (s *Server) handleResourceTemplatesList() *listResourceTemplatesResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &listResourceTemplatesResult{
		ResourceTemplates: append([]ResourceTemplate(nil), s.resourceTemplates...),
		CacheableResult:   s.cacheableLocked(),
	}
}

func (s *Server) handlePromptsList() *listPromptsResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &listPromptsResult{
		Prompts:         append([]Prompt(nil), s.prompts...),
		CacheableResult: s.cacheableLocked(),
	}
}

func (s *Server) handlePromptGet(ctx context.Context, rc *RequestContext, raw json.RawMessage) (any, *jsonRPCError) {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: fmt.Sprintf("invalid prompts/get params: %v", err),
		}
	}

	s.mu.Lock()
	getter := s.promptGetter
	s.mu.Unlock()
	if getter == nil {
		return nil, &jsonRPCError{
			Code:    jsonRPCCodeMethodNotFound,
			Message: "prompts/get not supported",
		}
	}

	result, err := getter(ctx, rc, params.Name, params.Arguments)
	if err != nil {
		return nil, rpcErrorFromError(err)
	}
	if rc.inputRequired != nil {
		return rc.inputRequired, nil
	}
	if rc.taskCreated != nil {
		return rc.taskCreated, nil
	}
	return result, nil
}

// requireTasks guards the tasks/* methods: the server must have the extension
// enabled and the client must have declared support for it.
func (s *Server) requireTasks(rc *RequestContext, method string) *jsonRPCError {
	if !s.tasksEnabled {
		return &jsonRPCError{Code: jsonRPCCodeMethodNotFound, Message: "method not found: " + method}
	}
	if !rc.ClientSupportsExtension(ExtensionTasks) {
		return &jsonRPCError{Code: jsonRPCCodeMethodNotFound, Message: "method not found: " + method}
	}
	return nil
}

func (s *Server) handleTaskGet(rc *RequestContext, raw json.RawMessage) (any, *jsonRPCError) {
	if rpcErr := s.requireTasks(rc, "tasks/get"); rpcErr != nil {
		return nil, rpcErr
	}
	var params struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &jsonRPCError{Code: jsonRPCCodeInvalidParams, Message: fmt.Sprintf("invalid tasks/get params: %v", err)}
	}
	entry, ok := s.tasks.get(params.TaskID)
	if !ok {
		return nil, &jsonRPCError{Code: jsonRPCCodeInvalidParams, Message: "task not found: " + params.TaskID}
	}
	task := entry.snapshot()
	return &task, nil
}

func (s *Server) handleTaskUpdate(rc *RequestContext, raw json.RawMessage) (any, *jsonRPCError) {
	if rpcErr := s.requireTasks(rc, "tasks/update"); rpcErr != nil {
		return nil, rpcErr
	}
	var params struct {
		TaskID         string         `json:"taskId"`
		InputResponses InputResponses `json:"inputResponses"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &jsonRPCError{Code: jsonRPCCodeInvalidParams, Message: fmt.Sprintf("invalid tasks/update params: %v", err)}
	}
	entry, ok := s.tasks.get(params.TaskID)
	if !ok {
		return nil, &jsonRPCError{Code: jsonRPCCodeInvalidParams, Message: "task not found: " + params.TaskID}
	}
	entry.applyInputResponses(params.InputResponses)
	return struct{}{}, nil
}

func (s *Server) handleTaskCancel(rc *RequestContext, raw json.RawMessage) (any, *jsonRPCError) {
	if rpcErr := s.requireTasks(rc, "tasks/cancel"); rpcErr != nil {
		return nil, rpcErr
	}
	var params struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &jsonRPCError{Code: jsonRPCCodeInvalidParams, Message: fmt.Sprintf("invalid tasks/cancel params: %v", err)}
	}
	entry, ok := s.tasks.get(params.TaskID)
	if !ok {
		return nil, &jsonRPCError{Code: jsonRPCCodeInvalidParams, Message: "task not found: " + params.TaskID}
	}
	entry.requestCancel()
	return struct{}{}, nil
}

func (s *Server) prepareCall() (int64, chan *jsonRPCMessage, error) {
	id := s.nextID.Add(1)
	ch := make(chan *jsonRPCMessage, 1)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, nil, errors.New("mcp: server is closed")
	}
	if s.peerEOF {
		return 0, nil, errors.New("mcp: connection closed")
	}
	s.pending[id] = ch
	return id, ch, nil
}

func (s *Server) removePending(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, id)
}

func (s *Server) awaitResponse(ctx context.Context, id int64, ch chan *jsonRPCMessage) (json.RawMessage, error) {
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

func (s *Server) writeResponse(id any, result json.RawMessage, rpcErr *jsonRPCError) error {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   rpcErr,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return s.writeJSON(data)
}

func (s *Server) writeJSON(data []byte) error {
	s.mu.Lock()
	writeFn := s.writeFn
	closed := s.closed
	s.mu.Unlock()

	if closed {
		return errors.New("mcp: server is closed")
	}
	if writeFn == nil {
		return errors.New("mcp: no active transport writer")
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeFn(data)
}

// Close marks the current server session closed and fails pending nested requests.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.peerEOF = true
	s.failPendingLocked()
	if s.hub != nil {
		s.hub.clear()
	}
	return nil
}

// WaitIdle waits for all in-flight request handlers to finish.
func (s *Server) WaitIdle() {
	s.wg.Wait()
}

func (s *Server) attachWriter(writeFn func([]byte) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeFn = writeFn
	s.closed = false
	s.peerEOF = false
}

func (s *Server) markPeerClosed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerEOF = true
	s.failPendingLocked()
	if s.hub != nil {
		s.hub.clear()
	}
}

func (s *Server) failPendingLocked() {
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
}

func (s *Server) serverCapabilitiesLocked() ServerCapabilities {
	var caps ServerCapabilities
	if len(s.tools) > 0 {
		caps.Tools = &ToolCapabilities{}
	}
	if len(s.resources) > 0 || len(s.resourceTemplates) > 0 || s.resourceReader != nil {
		caps.Resources = &ResourceCapabilities{}
	}
	if len(s.prompts) > 0 || s.promptGetter != nil {
		caps.Prompts = &PromptCapabilities{}
	}
	if s.tasksEnabled {
		if caps.Extensions == nil {
			caps.Extensions = map[string]json.RawMessage{}
		}
		caps.Extensions[ExtensionTasks] = json.RawMessage(`{}`)
	}
	return caps
}

func cloneServerInfo(info ServerInfo) *ServerInfo {
	cloned := info
	return &cloned
}

// parseRequestMeta extracts and validates the reserved _meta fields from a
// request's params. protocolVersion is required and must be supported.
// clientCapabilities is required. Both missing cases yield -32602; an
// unsupported version yields -32022.
func parseRequestMeta(raw json.RawMessage) (RequestMeta, *jsonRPCError) {
	meta := RequestMeta{}
	if len(raw) == 0 {
		return meta, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: "missing required _meta: protocolVersion",
		}
	}

	var probe struct {
		Meta *json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return meta, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: fmt.Sprintf("invalid request params: %v", err),
		}
	}
	if probe.Meta == nil || len(*probe.Meta) == 0 {
		return meta, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: "missing required _meta: protocolVersion",
		}
	}

	var metaMap map[string]json.RawMessage
	if err := json.Unmarshal(*probe.Meta, &metaMap); err != nil {
		return meta, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: fmt.Sprintf("invalid _meta: %v", err),
		}
	}

	pvRaw, ok := metaMap[MetaProtocolVersion]
	if !ok || len(pvRaw) == 0 {
		return meta, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: "missing required _meta: protocolVersion",
		}
	}
	var pv string
	if err := json.Unmarshal(pvRaw, &pv); err != nil {
		return meta, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: fmt.Sprintf("invalid _meta protocolVersion: %v", err),
		}
	}
	if !isSupportedProtocolVersion(pv) {
		return meta, UnsupportedProtocolVersionError(pv)
	}
	meta.ProtocolVersion = pv

	capsRaw, ok := metaMap[MetaClientCapabilities]
	if !ok || len(capsRaw) == 0 {
		return meta, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: "missing required _meta: clientCapabilities",
		}
	}
	var caps ClientCapabilities
	if err := json.Unmarshal(capsRaw, &caps); err != nil {
		return meta, &jsonRPCError{
			Code:    jsonRPCCodeInvalidParams,
			Message: fmt.Sprintf("invalid _meta clientCapabilities: %v", err),
		}
	}
	meta.ClientCapabilities = &caps

	if infoRaw, ok := metaMap[MetaClientInfo]; ok && len(infoRaw) > 0 {
		var info ImplementationInfo
		if err := json.Unmarshal(infoRaw, &info); err == nil {
			meta.ClientInfo = &info
		}
	}
	if logRaw, ok := metaMap[MetaLogLevel]; ok && len(logRaw) > 0 {
		var level string
		if err := json.Unmarshal(logRaw, &level); err == nil {
			meta.LogLevel = level
		}
	}
	return meta, nil
}

// parseContinuation extracts the MRTR retry fields (inputResponses,
// requestState) from a request's params. Empty on a first (non-retry) request.
func parseContinuation(raw json.RawMessage) MRTRContinuation {
	var cont MRTRContinuation
	if len(raw) == 0 {
		return cont
	}
	var probe struct {
		InputResponses InputResponses `json:"inputResponses"`
		RequestState   string         `json:"requestState"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return cont
	}
	return MRTRContinuation{
		InputResponses: probe.InputResponses,
		RequestState:   probe.RequestState,
	}
}

func isSupportedProtocolVersion(v string) bool {
	for _, supported := range SupportedProtocolVersions {
		if supported == v {
			return true
		}
	}
	return false
}

// requiredCapabilitiesForInputRequests returns the distinct client capabilities
// a set of input requests requires, in deterministic order.
func requiredCapabilitiesForInputRequests(reqs InputRequests) []string {
	seen := map[string]bool{}
	var out []string
	for _, ir := range reqs {
		cap := capabilityForMethod(ir.Method)
		if cap == "" || seen[cap] {
			continue
		}
		seen[cap] = true
		out = append(out, cap)
	}
	return out
}

// missingRequiredCapabilities returns the capabilities required by the input
// requests that the client did not declare on this request.
func missingRequiredCapabilities(rc *RequestContext, reqs InputRequests) []string {
	var missing []string
	for _, cap := range requiredCapabilitiesForInputRequests(reqs) {
		if !rc.ClientSupports(cap) {
			missing = append(missing, cap)
		}
	}
	return missing
}

// capabilityForMethod maps an input request method to the client capability it
// requires. Returns "" for methods with no capability requirement.
func capabilityForMethod(method string) string {
	switch method {
	case "sampling/createMessage":
		return "sampling"
	case "elicitation/create":
		return "elicitation"
	case "roots/list":
		return "roots"
	default:
		return ""
	}
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// ServerTransport is the minimal interface implemented by server transports.
type ServerTransport interface {
	io.Closer
	Run(context.Context) error
}
