package appserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	appcache "github.com/fugue-labs/gollem/appserver/cache"
	"github.com/fugue-labs/gollem/appserver/catalog"
	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
	toolgit "github.com/fugue-labs/gollem/appserver/tools/git"
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
)

type connectionState int

const (
	stateNew connectionState = iota
	stateInitialized
	stateReady
)

// Server dispatches Codex-style app-server JSON-RPC requests to Gollem
// services. It is transport-neutral; stdio/socket/WebSocket layers can wrap it.
type Server struct {
	mu         sync.Mutex
	state      connectionState
	serverInfo protocol.ImplementationInfo
	clientInfo protocol.ImplementationInfo
	optOut     map[string]struct{}

	store   store.Store
	fs      *toolfs.Service
	process *toolprocess.Service
	git     *toolgit.Service
	catalog *catalog.Catalog
	cache   *appcache.Service
}

// Option configures a Server.
type Option func(*Server)

func WithImplementationInfo(info protocol.ImplementationInfo) Option {
	return func(s *Server) {
		if info.Name != "" {
			s.serverInfo = info
		}
	}
}

func WithStore(st store.Store) Option {
	return func(s *Server) {
		s.store = st
	}
}

func WithFilesystem(fs *toolfs.Service) Option {
	return func(s *Server) {
		s.fs = fs
	}
}

func WithProcess(process *toolprocess.Service) Option {
	return func(s *Server) {
		s.process = process
	}
}

func WithGit(git *toolgit.Service) Option {
	return func(s *Server) {
		s.git = git
	}
}

func WithCatalog(catalog *catalog.Catalog) Option {
	return func(s *Server) {
		s.catalog = catalog
	}
}

func WithCache(cache *appcache.Service) Option {
	return func(s *Server) {
		s.cache = cache
	}
}

func NewServer(opts ...Option) *Server {
	s := &Server{
		serverInfo: protocol.ImplementationInfo{Name: "gollem-appserver"},
		optOut:     make(map[string]struct{}),
		catalog:    catalog.NewDefault(),
		cache:      appcache.NewService(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// HandleJSON handles one raw JSON-RPC request or notification. Requests return
// a response payload and true. Notifications return false and no payload.
func (s *Server) HandleJSON(ctx context.Context, data []byte) ([]byte, bool, error) {
	var probe struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, false, err
	}
	if len(probe.ID) > 0 {
		var req protocol.Request
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, false, err
		}
		resp := s.HandleRequest(ctx, req)
		out, err := json.Marshal(resp)
		return out, true, err
	}
	var notification protocol.Notification
	if err := json.Unmarshal(data, &notification); err != nil {
		return nil, false, err
	}
	return nil, false, s.HandleNotification(ctx, notification)
}

// HandleRequest handles one client-to-server request and always returns a
// JSON-RPC response.
func (s *Server) HandleRequest(ctx context.Context, req protocol.Request) protocol.Response {
	if s == nil {
		return errorResponse(req.ID, rpcError(protocol.CodeInternalError, "server not configured", nil))
	}
	if req.Method == "initialize" {
		return s.handleInitialize(req)
	}
	if err := s.requireReady(); err != nil {
		return errorResponse(req.ID, err)
	}
	result, rpcErr := s.dispatch(ctx, req.Method, req.Params)
	if rpcErr != nil {
		return errorResponse(req.ID, rpcErr)
	}
	return resultResponse(req.ID, result)
}

// HandleNotification handles one client-to-server notification.
func (s *Server) HandleNotification(ctx context.Context, notification protocol.Notification) error {
	_ = ctx
	if s == nil {
		return rpcError(protocol.CodeInternalError, "server not configured", nil)
	}
	if notification.Method == "initialized" {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch s.state {
		case stateNew:
			return rpcError(protocol.CodeInvalidRequest, "initialize required before initialized notification", nil)
		case stateInitialized:
			s.state = stateReady
			return nil
		case stateReady:
			return nil
		default:
			return rpcError(protocol.CodeInternalError, "invalid connection state", nil)
		}
	}
	if err := s.requireReady(); err != nil {
		return err
	}
	if !protocol.IsKnownMethod(notification.Method) {
		return protocol.MethodUnavailableError(notification.Method)
	}
	return protocol.MethodUnavailableErrorWithReason(notification.Method, "notification handler is not implemented in this Gollem build")
}

// NotificationEnabled reports whether the connected client opted out of a
// server notification method.
func (s *Server) NotificationEnabled(method string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, disabled := s.optOut[method]
	return !disabled
}

func (s *Server) handleInitialize(req protocol.Request) protocol.Response {
	var params protocol.InitializeParams
	if rpcErr := decodeParams(req.Params, &params); rpcErr != nil {
		return errorResponse(req.ID, rpcErr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != stateNew {
		return errorResponse(req.ID, rpcError(protocol.CodeInvalidRequest, "initialize has already completed", nil))
	}
	s.clientInfo = params.ClientInfo
	s.optOut = make(map[string]struct{}, len(params.Capabilities.OptOutNotificationMethods))
	for _, method := range params.Capabilities.OptOutNotificationMethods {
		if method != "" {
			s.optOut[method] = struct{}{}
		}
	}
	s.state = stateInitialized
	return resultResponse(req.ID, protocol.DefaultInitializeResponse(s.serverInfo))
}

func (s *Server) requireReady() *protocol.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.state {
	case stateNew:
		return rpcError(protocol.CodeInvalidRequest, "initialize required before other requests", nil)
	case stateInitialized:
		return rpcError(protocol.CodeInvalidRequest, "initialized notification required before other requests", nil)
	case stateReady:
		return nil
	default:
		return rpcError(protocol.CodeInternalError, "invalid connection state", nil)
	}
}

func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	switch method {
	case "thread/list":
		return s.handleThreadList(ctx, params)
	case "thread/read":
		return s.handleThreadRead(ctx, params)
	case "thread/fork":
		return s.handleThreadFork(ctx, params)
	case "thread/archive":
		return s.handleThreadStatus(ctx, params, method, store.ThreadArchived)
	case "thread/unarchive":
		return s.handleThreadStatus(ctx, params, method, store.ThreadActive)
	case "thread/delete":
		return s.handleThreadStatus(ctx, params, method, store.ThreadDeleted)
	case "thread/turns/list":
		return s.handleThreadTurnsList(ctx, params)
	case "thread/items/list":
		return s.handleThreadItemsList(ctx, params)
	case "model/list":
		return s.handleModelList(params)
	case "modelProvider/capabilities/read", "provider/capabilities/read":
		return s.handleProviderCapabilities(params)
	case "provider/list":
		return s.handleProviderList(params)
	case "tool/list":
		return s.handleToolList(params)
	case "cache/stats":
		return s.handleCacheStats()
	case "cache/benchmark":
		return s.handleCacheBenchmark(params)
	case "fs/readFile":
		return s.handleFSReadFile(ctx, params)
	case "fs/writeFile":
		return s.handleFSWriteFile(ctx, params)
	case "fs/createDirectory":
		return s.handleFSCreateDirectory(ctx, params)
	case "fs/readDirectory":
		return s.handleFSReadDirectory(ctx, params)
	case "fs/getMetadata":
		return s.handleFSMetadata(ctx, params)
	case "fs/remove":
		return s.handleFSRemove(ctx, params)
	case "fs/copy":
		return s.handleFSCopy(ctx, params)
	case "command/exec":
		return s.handleProcessStart(ctx, method, params, true)
	case "command/exec/write", "process/writeStdin":
		return s.handleProcessWriteStdin(ctx, method, params)
	case "command/exec/resize", "process/resizePty":
		return s.handleProcessResize(ctx, method, params)
	case "command/exec/terminate":
		return s.handleProcessTerminate(ctx, params)
	case "process/spawn":
		return s.handleProcessStart(ctx, method, params, false)
	case "process/kill":
		return s.handleProcessKill(ctx, params)
	case "git/status":
		return s.handleGitStatus(ctx)
	case "git/diff":
		return s.handleGitDiff(ctx, params)
	case "git/commit":
		return s.handleGitCommit(ctx, params)
	case "git/worktree/list":
		return s.handleGitWorktreeList(ctx)
	case "git/worktree/create":
		return s.handleGitWorktreeCreate(ctx, params)
	default:
		if protocol.IsKnownMethod(method) {
			return nil, protocol.MethodUnavailableError(method)
		}
		return nil, protocol.MethodUnavailableError(method)
	}
}

func (s *Server) handleThreadList(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	st, rpcErr := s.requireStore("thread/list")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params threadListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	threads, err := st.ListThreads(ctx, store.ThreadFilter{
		Statuses:       params.Statuses,
		IncludeDeleted: params.IncludeDeleted,
		Limit:          params.Limit,
	})
	if err != nil {
		return nil, mapError("thread/list", err)
	}
	return map[string]any{"threads": threads}, nil
}

func (s *Server) handleThreadRead(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	st, rpcErr := s.requireStore("thread/read")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params threadReadParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	threadID := params.threadID()
	if threadID == "" {
		return nil, invalidParams("threadId is required", nil)
	}
	thread, err := st.GetThread(ctx, threadID)
	if err != nil {
		return nil, mapError("thread/read", err)
	}
	result := threadReadResult{Thread: thread}
	if boolDefault(params.IncludeTurns, true) {
		turns, err := st.ListTurns(ctx, store.TurnFilter{ThreadID: threadID, Limit: params.Limit})
		if err != nil {
			return nil, mapError("thread/read", err)
		}
		result.Turns = turns
	}
	if boolDefault(params.IncludeItems, true) {
		items, err := st.ListItems(ctx, store.ItemFilter{ThreadID: threadID, AfterSeq: params.AfterSeq, Limit: params.Limit})
		if err != nil {
			return nil, mapError("thread/read", err)
		}
		result.Items = items
	}
	return result, nil
}

func (s *Server) handleThreadFork(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	st, rpcErr := s.requireStore("thread/fork")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params threadForkParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	sourceID := firstNonEmpty(params.SourceThreadID, params.ThreadID, params.ID)
	if sourceID == "" {
		return nil, invalidParams("sourceThreadId or threadId is required", nil)
	}
	thread, err := st.ForkThread(ctx, store.ForkThreadRequest{
		SourceThreadID: sourceID,
		Title:          params.Title,
		Metadata:       params.Metadata,
		IncludeItems:   params.IncludeItems,
	})
	if err != nil {
		return nil, mapError("thread/fork", err)
	}
	return map[string]any{"thread": thread}, nil
}

func (s *Server) handleThreadStatus(ctx context.Context, raw json.RawMessage, method string, status store.ThreadStatus) (any, *protocol.Error) {
	st, rpcErr := s.requireStore(method)
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params threadIDParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	threadID := params.threadID()
	if threadID == "" {
		return nil, invalidParams("threadId is required", nil)
	}
	var (
		thread *store.Thread
		err    error
	)
	switch status {
	case store.ThreadArchived:
		thread, err = st.ArchiveThread(ctx, threadID)
	case store.ThreadActive:
		thread, err = st.UnarchiveThread(ctx, threadID)
	case store.ThreadDeleted:
		thread, err = st.DeleteThread(ctx, threadID)
	default:
		return nil, invalidParams("unsupported thread status", nil)
	}
	if err != nil {
		return nil, mapError(method, err)
	}
	return map[string]any{"thread": thread}, nil
}

func (s *Server) handleThreadTurnsList(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	st, rpcErr := s.requireStore("thread/turns/list")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params threadTurnsListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	if params.ThreadID == "" {
		return nil, invalidParams("threadId is required", nil)
	}
	turns, err := st.ListTurns(ctx, store.TurnFilter{ThreadID: params.ThreadID, Statuses: params.Statuses, Limit: params.Limit})
	if err != nil {
		return nil, mapError("thread/turns/list", err)
	}
	return map[string]any{"turns": turns}, nil
}

func (s *Server) handleThreadItemsList(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	st, rpcErr := s.requireStore("thread/items/list")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params threadItemsListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	if params.ThreadID == "" {
		return nil, invalidParams("threadId is required", nil)
	}
	items, err := st.ListItems(ctx, store.ItemFilter{
		ThreadID: params.ThreadID,
		TurnID:   params.TurnID,
		AfterSeq: params.AfterSeq,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, mapError("thread/items/list", err)
	}
	return map[string]any{"items": items}, nil
}

func (s *Server) handleModelList(raw json.RawMessage) (any, *protocol.Error) {
	var params catalog.ModelListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := s.requireCatalog().ListModels(params)
	if err != nil {
		return nil, invalidParams("invalid model/list params", err)
	}
	return result, nil
}

func (s *Server) handleProviderList(raw json.RawMessage) (any, *protocol.Error) {
	var params catalog.ProviderListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	return s.requireCatalog().ListProviders(params), nil
}

func (s *Server) handleProviderCapabilities(raw json.RawMessage) (any, *protocol.Error) {
	var params catalog.CapabilitiesReadParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	providerID := firstNonEmpty(params.ProviderID, params.Provider, params.ModelProvider)
	caps, err := s.requireCatalog().ProviderCapabilities(providerID)
	if err != nil {
		return nil, invalidParams("invalid provider capabilities params", err)
	}
	return caps, nil
}

func (s *Server) handleToolList(raw json.RawMessage) (any, *protocol.Error) {
	var params catalog.ToolListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	return catalog.ListTools(params, catalog.ToolServices{
		Filesystem: s.fs != nil,
		Process:    s.process != nil,
		Git:        s.git != nil,
		Cache:      s.cache != nil,
	}), nil
}

func (s *Server) handleFSReadFile(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	fsSvc, rpcErr := s.requireFS("fs/readFile")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params pathParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	if params.Path == "" {
		return nil, invalidParams("path is required", nil)
	}
	file, err := fsSvc.ReadFile(ctx, params.Path)
	if err != nil {
		return nil, mapError("fs/readFile", err)
	}
	content, encoding := encodeContent(file.Content)
	return fileContentResult{
		Path:             file.Path,
		Content:          content,
		Encoding:         encoding,
		Size:             file.Size,
		Mode:             uint32(file.Mode),
		ModTime:          file.ModTime,
		ContentTruncated: false,
	}, nil
}

func (s *Server) handleFSWriteFile(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	fsSvc, rpcErr := s.requireFS("fs/writeFile")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params writeFileParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	if params.Path == "" {
		return nil, invalidParams("path is required", nil)
	}
	content, err := decodeContent(params.Content, params.Encoding)
	if err != nil {
		return nil, invalidParams("invalid file content encoding", err)
	}
	if err := fsSvc.WriteFile(ctx, params.Path, content, iofs.FileMode(params.Mode)); err != nil {
		return nil, mapError("fs/writeFile", err)
	}
	return okResult(params.Path), nil
}

func (s *Server) handleFSCreateDirectory(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	fsSvc, rpcErr := s.requireFS("fs/createDirectory")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params pathParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	if params.Path == "" {
		return nil, invalidParams("path is required", nil)
	}
	if err := fsSvc.CreateDirectory(ctx, params.Path); err != nil {
		return nil, mapError("fs/createDirectory", err)
	}
	return okResult(params.Path), nil
}

func (s *Server) handleFSReadDirectory(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	fsSvc, rpcErr := s.requireFS("fs/readDirectory")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params pathParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	entries, err := fsSvc.ReadDirectory(ctx, params.Path)
	if err != nil {
		return nil, mapError("fs/readDirectory", err)
	}
	return map[string]any{"entries": entries}, nil
}

func (s *Server) handleFSMetadata(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	fsSvc, rpcErr := s.requireFS("fs/getMetadata")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params pathParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	if params.Path == "" {
		return nil, invalidParams("path is required", nil)
	}
	meta, err := fsSvc.Metadata(ctx, params.Path)
	if err != nil {
		return nil, mapError("fs/getMetadata", err)
	}
	return metadataResult{Path: meta.Path, IsDir: meta.IsDir, Size: meta.Size, Mode: uint32(meta.Mode), ModTime: meta.ModTime}, nil
}

func (s *Server) handleFSRemove(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	fsSvc, rpcErr := s.requireFS("fs/remove")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params pathParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	if params.Path == "" {
		return nil, invalidParams("path is required", nil)
	}
	if err := fsSvc.Remove(ctx, params.Path); err != nil {
		return nil, mapError("fs/remove", err)
	}
	return okResult(params.Path), nil
}

func (s *Server) handleFSCopy(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	fsSvc, rpcErr := s.requireFS("fs/copy")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params copyParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	src := firstNonEmpty(params.Source, params.SourcePath, params.Path)
	dst := firstNonEmpty(params.Destination, params.DestinationPath)
	if src == "" || dst == "" {
		return nil, invalidParams("source and destination are required", nil)
	}
	if err := fsSvc.Copy(ctx, src, dst); err != nil {
		return nil, mapError("fs/copy", err)
	}
	return map[string]any{"ok": true, "source": src, "destination": dst}, nil
}

func (s *Server) handleProcessStart(ctx context.Context, method string, raw json.RawMessage, defaultShell bool) (any, *protocol.Error) {
	processSvc, rpcErr := s.requireProcess(method)
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params processStartParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	if params.TimeoutMillis < 0 {
		return nil, invalidParams("timeoutMillis must be non-negative", nil)
	}
	shell := boolDefault(params.Shell, defaultShell)
	snapshot, err := processSvc.Start(ctx, toolprocess.StartRequest{
		Command:        params.Command,
		Args:           params.Args,
		Shell:          shell,
		WorkDir:        params.WorkDir,
		Env:            params.Env,
		Timeout:        time.Duration(params.TimeoutMillis) * time.Millisecond,
		MaxOutputBytes: params.MaxOutputBytes,
	})
	if err != nil {
		return nil, mapError(method, err)
	}
	return map[string]any{"process": processSnapshotResultFrom(snapshot)}, nil
}

func (s *Server) handleProcessWriteStdin(ctx context.Context, method string, raw json.RawMessage) (any, *protocol.Error) {
	processSvc, rpcErr := s.requireProcess(method)
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params processWriteParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	id := firstNonEmpty(params.ID, params.ProcessID)
	if id == "" {
		return nil, invalidParams("id or processId is required", nil)
	}
	data, err := decodeContent(params.Data, params.Encoding)
	if err != nil {
		return nil, invalidParams("invalid stdin encoding", err)
	}
	if len(data) > 0 {
		if err := processSvc.WriteStdin(ctx, id, data); err != nil {
			return nil, mapError(method, err)
		}
	}
	if params.Close {
		if err := processSvc.CloseStdin(ctx, id); err != nil {
			return nil, mapError(method, err)
		}
	}
	return okResult(id), nil
}

func (s *Server) handleProcessResize(ctx context.Context, method string, raw json.RawMessage) (any, *protocol.Error) {
	processSvc, rpcErr := s.requireProcess(method)
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params processResizeParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	id := firstNonEmpty(params.ID, params.ProcessID)
	if id == "" {
		return nil, invalidParams("id or processId is required", nil)
	}
	if err := processSvc.ResizePTY(ctx, id, params.Cols, params.Rows); err != nil {
		return nil, mapError(method, err)
	}
	return okResult(id), nil
}

func (s *Server) handleProcessTerminate(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	processSvc, rpcErr := s.requireProcess("command/exec/terminate")
	if rpcErr != nil {
		return nil, rpcErr
	}
	id, rpcErr := decodeProcessID(raw)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if err := processSvc.Terminate(ctx, id); err != nil {
		return nil, mapError("command/exec/terminate", err)
	}
	return okResult(id), nil
}

func (s *Server) handleProcessKill(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	processSvc, rpcErr := s.requireProcess("process/kill")
	if rpcErr != nil {
		return nil, rpcErr
	}
	id, rpcErr := decodeProcessID(raw)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if err := processSvc.Kill(ctx, id); err != nil {
		return nil, mapError("process/kill", err)
	}
	return okResult(id), nil
}

func (s *Server) handleGitStatus(ctx context.Context) (any, *protocol.Error) {
	gitSvc, rpcErr := s.requireGit("git/status")
	if rpcErr != nil {
		return nil, rpcErr
	}
	status, err := gitSvc.Status(ctx)
	if err != nil {
		return nil, mapError("git/status", err)
	}
	return map[string]any{"status": status}, nil
}

func (s *Server) handleGitDiff(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	gitSvc, rpcErr := s.requireGit("git/diff")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params gitDiffParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	diff, err := gitSvc.Diff(ctx, toolgit.DiffRequest{Ref: params.Ref, Pathspecs: params.Pathspecs, Cached: params.Cached})
	if err != nil {
		return nil, mapError("git/diff", err)
	}
	return map[string]any{"diff": diff}, nil
}

func (s *Server) handleGitCommit(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	gitSvc, rpcErr := s.requireGit("git/commit")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params gitCommitParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := gitSvc.Commit(ctx, toolgit.CommitRequest{
		Message:    params.Message,
		All:        params.All,
		Pathspecs:  params.Pathspecs,
		AllowEmpty: params.AllowEmpty,
	})
	if err != nil {
		return nil, mapError("git/commit", err)
	}
	return map[string]any{"commit": result}, nil
}

func (s *Server) handleGitWorktreeList(ctx context.Context) (any, *protocol.Error) {
	gitSvc, rpcErr := s.requireGit("git/worktree/list")
	if rpcErr != nil {
		return nil, rpcErr
	}
	worktrees, err := gitSvc.WorktreeList(ctx)
	if err != nil {
		return nil, mapError("git/worktree/list", err)
	}
	return map[string]any{"worktrees": worktrees}, nil
}

func (s *Server) handleGitWorktreeCreate(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	gitSvc, rpcErr := s.requireGit("git/worktree/create")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params gitWorktreeCreateParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	worktree, err := gitSvc.WorktreeCreate(ctx, toolgit.WorktreeCreateRequest{
		Path:   params.Path,
		Branch: params.Branch,
		Base:   params.Base,
		Force:  params.Force,
	})
	if err != nil {
		return nil, mapError("git/worktree/create", err)
	}
	return map[string]any{"worktree": worktree}, nil
}

func (s *Server) requireStore(method string) (store.Store, *protocol.Error) {
	if s.store == nil {
		return nil, protocol.MethodUnavailableErrorWithReason(method, "thread store is not configured")
	}
	return s.store, nil
}

func (s *Server) requireFS(method string) (*toolfs.Service, *protocol.Error) {
	if s.fs == nil {
		return nil, protocol.MethodUnavailableErrorWithReason(method, "filesystem service is not configured")
	}
	return s.fs, nil
}

func (s *Server) requireProcess(method string) (*toolprocess.Service, *protocol.Error) {
	if s.process == nil {
		return nil, protocol.MethodUnavailableErrorWithReason(method, "process service is not configured")
	}
	return s.process, nil
}

func (s *Server) requireGit(method string) (*toolgit.Service, *protocol.Error) {
	if s.git == nil {
		return nil, protocol.MethodUnavailableErrorWithReason(method, "git service is not configured")
	}
	return s.git, nil
}

func (s *Server) requireCatalog() *catalog.Catalog {
	if s.catalog == nil {
		return catalog.NewDefault()
	}
	return s.catalog
}

func decodeParams(raw json.RawMessage, out any) *protocol.Error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return invalidParams("invalid params", err)
	}
	return nil
}

func decodeProcessID(raw json.RawMessage) (string, *protocol.Error) {
	var params processIDParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return "", rpcErr
	}
	id := firstNonEmpty(params.ID, params.ProcessID)
	if id == "" {
		return "", invalidParams("id or processId is required", nil)
	}
	return id, nil
}

func resultResponse(id protocol.RequestID, result any) protocol.Response {
	data, err := json.Marshal(result)
	if err != nil {
		return errorResponse(id, rpcError(protocol.CodeInternalError, "marshal response result", err))
	}
	return protocol.Response{ID: id, Result: data}
}

func errorResponse(id protocol.RequestID, err *protocol.Error) protocol.Response {
	return protocol.Response{ID: id, Error: err}
}

func invalidParams(message string, err error) *protocol.Error {
	return rpcError(protocol.CodeInvalidParams, message, err)
}

func rpcError(code int, message string, err error) *protocol.Error {
	data := map[string]string{}
	if err != nil {
		data["reason"] = err.Error()
	}
	if message == "" {
		message = "app-server error"
	}
	var raw json.RawMessage
	if len(data) > 0 {
		raw, _ = json.Marshal(data)
	}
	return &protocol.Error{Code: code, Message: message, Data: raw}
}

func mapError(method string, err error) *protocol.Error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return rpcError(protocol.CodeInvalidRequest, "request context ended", err)
	case errors.Is(err, toolprocess.ErrPTYUnsupported):
		return protocol.MethodUnavailableErrorWithReason(method, "pty resize is not supported until a PTY backend is configured")
	case errors.Is(err, toolfs.ErrPathOutsideRoot),
		errors.Is(err, toolfs.ErrInvalidCopyDestination),
		errors.Is(err, toolfs.ErrRefusingRoot),
		errors.Is(err, toolgit.ErrPathOutsideRoot),
		errors.Is(err, toolgit.ErrInvalidPathspec),
		errors.Is(err, toolgit.ErrInvalidMessage),
		errors.Is(err, toolprocess.ErrEmptyCommand),
		errors.Is(err, toolprocess.ErrInvalidWorkDir),
		errors.Is(err, toolprocess.ErrProcessNotFound),
		errors.Is(err, toolprocess.ErrProcessNotRunning),
		errors.Is(err, store.ErrThreadNotFound),
		errors.Is(err, store.ErrTurnNotFound),
		errors.Is(err, store.ErrItemNotFound),
		errors.Is(err, store.ErrThreadDeleted):
		return invalidParams("invalid params", err)
	case errors.Is(err, toolfs.ErrApprovalDenied),
		errors.Is(err, toolgit.ErrApprovalDenied),
		errors.Is(err, toolprocess.ErrApprovalDenied):
		return rpcError(protocol.CodeInvalidRequest, "operation denied by approval policy", err)
	default:
		return rpcError(protocol.CodeInternalError, "internal app-server error", err)
	}
}

func encodeContent(data []byte) (string, string) {
	if utf8.Valid(data) {
		return string(data), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(data), "base64"
}

func decodeContent(content, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "", "utf-8", "utf8":
		return []byte(content), nil
	case "base64":
		return base64.StdEncoding.DecodeString(content)
	default:
		return nil, fmt.Errorf("unsupported encoding %q", encoding)
	}
}

func processSnapshotResultFrom(snapshot *toolprocess.Snapshot) processSnapshotResult {
	if snapshot == nil {
		return processSnapshotResult{}
	}
	stdout, stdoutEncoding := encodeContent(snapshot.Stdout)
	stderr, stderrEncoding := encodeContent(snapshot.Stderr)
	return processSnapshotResult{
		ID:             snapshot.ID,
		PID:            snapshot.PID,
		Command:        snapshot.Command,
		Args:           snapshot.Args,
		Shell:          snapshot.Shell,
		WorkDir:        snapshot.WorkDir,
		Status:         snapshot.Status,
		ExitCode:       snapshot.ExitCode,
		StartedAt:      snapshot.StartedAt,
		EndedAt:        snapshot.EndedAt,
		Error:          snapshot.Error,
		Stdout:         stdout,
		StdoutEncoding: stdoutEncoding,
		Stderr:         stderr,
		StderrEncoding: stderrEncoding,
	}
}

func okResult(path string) map[string]any {
	if path == "" {
		return map[string]any{"ok": true}
	}
	return map[string]any{"ok": true, "path": path}
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type threadListParams struct {
	Statuses       []store.ThreadStatus `json:"statuses,omitempty"`
	IncludeDeleted bool                 `json:"includeDeleted,omitempty"`
	Limit          int                  `json:"limit,omitempty"`
}

type threadIDParams struct {
	ID       string `json:"id,omitempty"`
	ThreadID string `json:"threadId,omitempty"`
}

func (p threadIDParams) threadID() string {
	return firstNonEmpty(p.ThreadID, p.ID)
}

type threadReadParams struct {
	ID           string `json:"id,omitempty"`
	ThreadID     string `json:"threadId,omitempty"`
	IncludeTurns *bool  `json:"includeTurns,omitempty"`
	IncludeItems *bool  `json:"includeItems,omitempty"`
	AfterSeq     int64  `json:"afterSeq,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

func (p threadReadParams) threadID() string {
	return firstNonEmpty(p.ThreadID, p.ID)
}

type threadReadResult struct {
	Thread *store.Thread `json:"thread"`
	Turns  []*store.Turn `json:"turns,omitempty"`
	Items  []*store.Item `json:"items,omitempty"`
}

type threadForkParams struct {
	ID             string         `json:"id,omitempty"`
	ThreadID       string         `json:"threadId,omitempty"`
	SourceThreadID string         `json:"sourceThreadId,omitempty"`
	Title          string         `json:"title,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	IncludeItems   bool           `json:"includeItems,omitempty"`
}

type threadTurnsListParams struct {
	ThreadID string             `json:"threadId"`
	Statuses []store.TurnStatus `json:"statuses,omitempty"`
	Limit    int                `json:"limit,omitempty"`
}

type threadItemsListParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId,omitempty"`
	AfterSeq int64  `json:"afterSeq,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type pathParams struct {
	Path string `json:"path,omitempty"`
}

type writeFileParams struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
	Mode     uint32 `json:"mode,omitempty"`
}

type copyParams struct {
	Path            string `json:"path,omitempty"`
	Source          string `json:"source,omitempty"`
	SourcePath      string `json:"sourcePath,omitempty"`
	Destination     string `json:"destination,omitempty"`
	DestinationPath string `json:"destinationPath,omitempty"`
}

type fileContentResult struct {
	Path             string    `json:"path"`
	Content          string    `json:"content"`
	Encoding         string    `json:"encoding"`
	Size             int64     `json:"size"`
	Mode             uint32    `json:"mode"`
	ModTime          time.Time `json:"modTime"`
	ContentTruncated bool      `json:"contentTruncated"`
}

type metadataResult struct {
	Path    string    `json:"path"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	Mode    uint32    `json:"mode"`
	ModTime time.Time `json:"modTime"`
}

type processStartParams struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Shell          *bool             `json:"shell,omitempty"`
	WorkDir        string            `json:"workDir,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutMillis  int64             `json:"timeoutMillis,omitempty"`
	MaxOutputBytes int               `json:"maxOutputBytes,omitempty"`
}

type processIDParams struct {
	ID        string `json:"id,omitempty"`
	ProcessID string `json:"processId,omitempty"`
}

type processWriteParams struct {
	ID        string `json:"id,omitempty"`
	ProcessID string `json:"processId,omitempty"`
	Data      string `json:"data,omitempty"`
	Encoding  string `json:"encoding,omitempty"`
	Close     bool   `json:"close,omitempty"`
}

type processResizeParams struct {
	ID        string `json:"id,omitempty"`
	ProcessID string `json:"processId,omitempty"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

type processSnapshotResult struct {
	ID             string             `json:"id"`
	PID            int                `json:"pid"`
	Command        string             `json:"command"`
	Args           []string           `json:"args,omitempty"`
	Shell          bool               `json:"shell"`
	WorkDir        string             `json:"workDir"`
	Status         toolprocess.Status `json:"status"`
	ExitCode       int                `json:"exitCode"`
	StartedAt      time.Time          `json:"startedAt"`
	EndedAt        time.Time          `json:"endedAt,omitempty"`
	Error          string             `json:"error,omitempty"`
	Stdout         string             `json:"stdout,omitempty"`
	StdoutEncoding string             `json:"stdoutEncoding,omitempty"`
	Stderr         string             `json:"stderr,omitempty"`
	StderrEncoding string             `json:"stderrEncoding,omitempty"`
}

type gitDiffParams struct {
	Ref       string   `json:"ref,omitempty"`
	Pathspecs []string `json:"pathspecs,omitempty"`
	Cached    bool     `json:"cached,omitempty"`
}

type gitCommitParams struct {
	Message    string   `json:"message"`
	All        bool     `json:"all,omitempty"`
	Pathspecs  []string `json:"pathspecs,omitempty"`
	AllowEmpty bool     `json:"allowEmpty,omitempty"`
}

type gitWorktreeCreateParams struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Base   string `json:"base,omitempty"`
	Force  bool   `json:"force,omitempty"`
}
