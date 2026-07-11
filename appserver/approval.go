package appserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
	toolgit "github.com/fugue-labs/gollem/appserver/tools/git"
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
	"github.com/fugue-labs/gollem/core"
)

var ErrApprovalRequestDenied = errors.New("appserver: approval request denied")

const (
	approvalThreadID                = "appserver"
	approvalTurnID                  = "appserver"
	approvalResponsePayloadMaxBytes = 64 * 1024
)

// ApprovalService bridges synchronous tool approval hooks to app-server
// server-to-client approval requests resolved directly or by approval/respond.
type ApprovalService struct {
	mu                     sync.Mutex
	counter                int64
	pending                map[string]pendingApproval
	fileChangeSessionPaths map[string]struct{}
	requests               *RequestQueue
}

type pendingApproval struct {
	ch                      chan approvalResolution
	threadID                string
	turnID                  string
	method                  string
	sessionTargets          []string
	allowedCommandDecisions []string
}

type approvalResolution struct {
	Approved bool
	Message  string
}

type approvalResponseResult struct {
	RequestID  string
	ThreadID   string
	TurnID     string
	CancelTurn bool
	ch         chan approvalResolution
	resolution approvalResolution
}

func (r approvalResponseResult) resolve() {
	if r.ch != nil {
		r.ch <- r.resolution
	}
}

type approvalRequestBase struct {
	protocol.ApprovalRequestBase
	requestID string
}

type approvalRespondParams = protocol.ApprovalRespondParams
type fileChangeApprovalParams = protocol.FileChangeApprovalRequestParams
type commandApprovalParams = protocol.CommandExecutionApprovalRequestParams
type permissionsApprovalParams = protocol.PermissionsApprovalRequestParams

type approvalRespondResult struct {
	protocol.ApprovalRespondResult
	threadID string
}

func NewApprovalService() *ApprovalService {
	return &ApprovalService{
		pending:                make(map[string]pendingApproval),
		fileChangeSessionPaths: make(map[string]struct{}),
		requests:               NewRequestQueue(),
	}
}

func (s *ApprovalService) setRequestQueue(q *RequestQueue) {
	if s == nil || q == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = q
}

func (s *ApprovalService) RequestSignal() <-chan struct{} {
	if s == nil || s.requests == nil {
		return nil
	}
	return s.requests.Signal()
}

func (s *ApprovalService) DrainRequests() []protocol.Request {
	if s == nil || s.requests == nil {
		return nil
	}
	return s.requests.Drain()
}

func (s *ApprovalService) FilesystemApproval(ctx context.Context, op toolfs.Operation) error {
	sessionTargets := fileChangeApprovalSessionTargets(op)
	if s.fileChangeSessionApproved(sessionTargets) {
		return nil
	}
	reason := fmt.Sprintf("Approve filesystem %s for %s", op.Kind, op.Path)
	base := s.base(ctx, reason)
	params := protocol.FileChangeApprovalRequestParams{
		ApprovalRequestBase: base.ApprovalRequestBase,
		GrantRoot:           nil,
		Operation:           string(op.Kind),
		Path:                op.Path,
		Destination:         op.Destination,
		Destructive:         op.Destructive,
	}
	return s.requestApproval(ctx, "item/fileChange/requestApproval", base.requestID, params, sessionTargets, nil)
}

func (s *ApprovalService) ProcessApproval(ctx context.Context, op toolprocess.Operation) error {
	command := strings.TrimSpace(strings.Join(append([]string{op.Command}, op.Args...), " "))
	reason := fmt.Sprintf("Approve process %s", op.Kind)
	base := s.base(ctx, reason)
	allowedDecisions := []string{
		protocol.CommandExecutionApprovalAccept,
		protocol.CommandExecutionApprovalDecline,
		protocol.CommandExecutionApprovalCancel,
	}
	availableDecisions, err := protocol.NewCommandExecutionApprovalDecisions(allowedDecisions...)
	if err != nil {
		return fmt.Errorf("build command approval decisions: %w", err)
	}
	params := protocol.CommandExecutionApprovalRequestParams{
		ApprovalRequestBase: base.ApprovalRequestBase,
		Command:             command,
		Args:                append([]string(nil), op.Args...),
		CWD:                 op.WorkDir,
		Operation:           string(op.Kind),
		Signal:              op.Signal,
		Destructive:         op.Destructive,
		AvailableDecisions:  availableDecisions,
	}
	return s.requestApproval(ctx, "item/commandExecution/requestApproval", base.requestID, params, nil, allowedDecisions)
}

func (s *ApprovalService) GitApproval(ctx context.Context, op toolgit.Operation) error {
	reason := fmt.Sprintf("Approve git %s", op.Kind)
	cwd := op.Path
	if cwd == "" {
		cwd = "."
	}
	requestBase := s.base(ctx, reason)
	params := protocol.PermissionsApprovalRequestParams{
		ApprovalRequestBase: requestBase.ApprovalRequestBase,
		CWD:                 cwd,
		Operation:           string(op.Kind),
		Path:                op.Path,
		Branch:              op.Branch,
		Base:                op.Base,
		Message:             op.Message,
		Pathspecs:           append([]string(nil), op.Pathspecs...),
		Permissions: map[string]any{
			"kind":      "git",
			"operation": string(op.Kind),
			"mutating":  op.Mutating,
		},
	}
	return s.requestApproval(ctx, "item/permissions/requestApproval", requestBase.requestID, params, nil, nil)
}

func (s *ApprovalService) MCPToolApproval(ctx context.Context, serverName, toolName string, args map[string]any) error {
	reason := fmt.Sprintf("Approve MCP tool %s on %s", toolName, serverName)
	argKeys := make([]string, 0, len(args))
	for key := range args {
		argKeys = append(argKeys, key)
	}
	slices.Sort(argKeys)
	base := s.base(ctx, reason)
	params := protocol.PermissionsApprovalRequestParams{
		ApprovalRequestBase: base.ApprovalRequestBase,
		CWD:                 ".",
		Operation:           "mcpToolCall",
		Permissions: map[string]any{
			"kind":         "mcp",
			"server":       serverName,
			"tool":         toolName,
			"argumentKeys": argKeys,
			"mutating":     true,
		},
	}
	return s.requestApproval(ctx, "item/permissions/requestApproval", base.requestID, params, nil, nil)
}

func (s *Server) handleApprovalRespond(raw json.RawMessage) (any, *protocol.Error) {
	if s == nil || s.approvals == nil {
		return nil, protocol.MethodUnavailableErrorWithReason("approval/respond", "approval service is not configured")
	}
	var params approvalRespondParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := s.approvals.Respond(params)
	if err != nil {
		return nil, invalidParams("invalid approval response", err)
	}
	s.PublishNotification("serverRequest/resolved", serverRequestResolvedParams{
		ThreadID:  firstNonEmpty(result.threadID, approvalThreadID),
		RequestID: protocol.NewStringID(result.RequestID),
	})
	return result, nil
}

func (s *ApprovalService) Respond(params approvalRespondParams) (approvalRespondResult, error) {
	if s == nil {
		return approvalRespondResult{}, errors.New("approval service is not configured")
	}
	requestID := strings.TrimSpace(firstNonEmpty(params.RequestID, params.ID))
	if requestID == "" {
		return approvalRespondResult{}, errors.New("requestId is required")
	}
	s.mu.Lock()
	pending, ok := s.pending[requestID]
	if ok {
		delete(s.pending, requestID)
	}
	s.mu.Unlock()
	if !ok {
		return approvalRespondResult{}, fmt.Errorf("approval request %q is not pending", requestID)
	}
	pending.ch <- approvalResolution{Approved: params.Approved, Message: params.Message}
	return approvalRespondResult{
		ApprovalRespondResult: protocol.ApprovalRespondResult{OK: true, RequestID: requestID, Approved: params.Approved},
		threadID:              pending.threadID,
	}, nil
}

func (s *ApprovalService) RespondResponse(resp protocol.Response) (approvalResponseResult, bool, error) {
	if s == nil {
		return approvalResponseResult{}, false, errors.New("approval service is not configured")
	}
	requestID := requestIDString(resp.ID)
	if requestID == "" {
		return approvalResponseResult{}, false, errors.New("response id is required")
	}

	s.mu.Lock()
	pending, ok := s.pending[requestID]
	if !ok {
		s.mu.Unlock()
		return approvalResponseResult{}, false, nil
	}
	result := approvalResponseResult{
		RequestID: requestID,
		ThreadID:  pending.threadID,
		TurnID:    pending.turnID,
		ch:        pending.ch,
	}
	resolution := approvalResolution{Message: "approval response was rejected"}
	var responseErr error
	if resp.Error != nil {
		if len(resp.Error.Message) > approvalResponsePayloadMaxBytes {
			responseErr = fmt.Errorf("%s error response exceeds %d bytes", pending.method, approvalResponsePayloadMaxBytes)
		} else {
			resolution.Message = resp.Error.Message
		}
	} else if len(resp.Result) > approvalResponsePayloadMaxBytes {
		responseErr = fmt.Errorf("%s response exceeds %d bytes", pending.method, approvalResponsePayloadMaxBytes)
	} else {
		switch pending.method {
		case "item/fileChange/requestApproval":
			var response protocol.FileChangeRequestApprovalResponse
			if err := decodeStrictApprovalResult(resp.Result, &response); err != nil {
				responseErr = fmt.Errorf("decode file-change approval response: %w", err)
			} else {
				switch response.Decision {
				case protocol.FileChangeApprovalAccept:
					resolution.Approved = true
					resolution.Message = ""
				case protocol.FileChangeApprovalAcceptForSession:
					resolution.Approved = true
					resolution.Message = ""
					if s.fileChangeSessionPaths == nil {
						s.fileChangeSessionPaths = make(map[string]struct{})
					}
					for _, target := range pending.sessionTargets {
						s.fileChangeSessionPaths[target] = struct{}{}
					}
				case protocol.FileChangeApprovalDecline:
					resolution.Message = "file change declined"
				case protocol.FileChangeApprovalCancel:
					resolution.Message = "file change canceled"
					result.CancelTurn = true
				default:
					responseErr = fmt.Errorf("unsupported file-change approval decision %q", response.Decision)
				}
			}
		case "item/commandExecution/requestApproval":
			var response protocol.CommandExecutionRequestApprovalResponse
			if err := decodeStrictApprovalResult(resp.Result, &response); err != nil {
				responseErr = fmt.Errorf("decode command-execution approval response: %w", err)
			} else {
				action := response.Decision.Action()
				if !slices.Contains(pending.allowedCommandDecisions, action) {
					responseErr = fmt.Errorf("command-execution approval decision %q was not offered", action)
				} else {
					switch action {
					case protocol.CommandExecutionApprovalAccept:
						resolution.Approved = true
						resolution.Message = ""
					case protocol.CommandExecutionApprovalDecline:
						resolution.Message = "command execution declined"
					case protocol.CommandExecutionApprovalCancel:
						resolution.Message = "command execution canceled"
						result.CancelTurn = true
					}
				}
			}
		default:
			responseErr = fmt.Errorf("direct response is not implemented for %s", pending.method)
		}
	}
	delete(s.pending, requestID)
	s.mu.Unlock()
	result.resolution = resolution
	return result, true, responseErr
}

func decodeStrictApprovalResult(data json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("approval response must contain one JSON value")
		}
		return err
	}
	return nil
}

type serverRequestResolvedParams = protocol.ServerRequestResolvedNotification

func (s *ApprovalService) base(ctx context.Context, reason string) approvalRequestBase {
	requestID := s.nextRequestID()
	runtimeContext := runtimeTurnContextFrom(ctx)
	threadID := firstNonEmpty(runtimeContext.ThreadID, approvalThreadID)
	turnID := firstNonEmpty(runtimeContext.TurnID, approvalTurnID)
	itemID := firstNonEmpty(runtimeApprovalItemIDFrom(ctx), core.ToolCallIDFromContext(ctx), requestID)
	return approvalRequestBase{
		requestID: requestID,
		ApprovalRequestBase: protocol.ApprovalRequestBase{
			ThreadID:    threadID,
			TurnID:      turnID,
			ItemID:      itemID,
			StartedAtMS: time.Now().UnixMilli(),
			Reason:      reason,
		},
	}
}

func (s *ApprovalService) nextRequestID() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	return fmt.Sprintf("approval-%d", s.counter)
}

func (s *ApprovalService) requestApproval(
	ctx context.Context,
	method string,
	requestID string,
	params any,
	sessionTargets []string,
	allowedCommandDecisions []string,
) error {
	if s == nil || s.requests == nil {
		return errors.New("approval service is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtimeContext := runtimeTurnContextFrom(ctx)
	pending := pendingApproval{
		ch:                      make(chan approvalResolution, 1),
		threadID:                firstNonEmpty(runtimeContext.ThreadID, approvalThreadID),
		turnID:                  firstNonEmpty(runtimeContext.TurnID, approvalTurnID),
		method:                  method,
		sessionTargets:          append([]string(nil), sessionTargets...),
		allowedCommandDecisions: append([]string(nil), allowedCommandDecisions...),
	}
	s.mu.Lock()
	if s.pending == nil {
		s.pending = make(map[string]pendingApproval)
	}
	s.pending[requestID] = pending
	s.mu.Unlock()

	s.requests.Publish(method, protocol.NewStringID(requestID), params)
	select {
	case resolution := <-pending.ch:
		if resolution.Approved {
			return nil
		}
		if strings.TrimSpace(resolution.Message) != "" {
			return fmt.Errorf("%w: %s", ErrApprovalRequestDenied, strings.TrimSpace(resolution.Message))
		}
		return ErrApprovalRequestDenied
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, requestID)
		s.mu.Unlock()
		return ctx.Err()
	}
}

func (s *ApprovalService) fileChangeSessionApproved(targets []string) bool {
	if s == nil || len(targets) == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, target := range targets {
		if _, ok := s.fileChangeSessionPaths[target]; !ok {
			return false
		}
	}
	return true
}

func fileChangeApprovalSessionTargets(op toolfs.Operation) []string {
	paths := []string{op.Path}
	if op.Kind == toolfs.OperationCopy {
		paths = []string{op.Destination}
	}
	targets := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		targets = append(targets, filepath.ToSlash(filepath.Clean(path)))
	}
	return targets
}
