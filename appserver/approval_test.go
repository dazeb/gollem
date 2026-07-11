package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
	toolgit "github.com/fugue-labs/gollem/appserver/tools/git"
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
)

func TestApprovalServiceFilesystemApprovalPublishesAndResolves(t *testing.T) {
	approvals := NewApprovalService()
	errCh := make(chan error, 1)
	go func() {
		errCh <- approvals.FilesystemApproval(context.Background(), toolfs.Operation{
			Kind:        toolfs.OperationWriteFile,
			Path:        "note.txt",
			Destructive: false,
		})
	}()

	req := waitForApprovalRequest(t, approvals)
	if req.Method != "item/fileChange/requestApproval" {
		t.Fatalf("approval method = %q", req.Method)
	}
	var params fileChangeApprovalParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("decode approval params: %v", err)
	}
	if params.Path != "note.txt" || params.Operation != "writeFile" {
		t.Fatalf("approval params = %#v", params)
	}

	result, err := approvals.Respond(approvalRespondParams{RequestID: "approval-1", Approved: true})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if !result.OK || !result.Approved {
		t.Fatalf("approval response result = %#v", result)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("approval returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approval wait did not resolve")
	}
}

func TestApprovalServiceDeniedResponse(t *testing.T) {
	approvals := NewApprovalService()
	errCh := make(chan error, 1)
	go func() {
		errCh <- approvals.FilesystemApproval(context.Background(), toolfs.Operation{
			Kind: toolfs.OperationRemove,
			Path: "note.txt",
		})
	}()

	req := waitForApprovalRequest(t, approvals)
	requestID, _ := req.ID.Value().(string)
	if _, err := approvals.Respond(approvalRespondParams{RequestID: requestID, Approved: false, Message: "not now"}); err != nil {
		t.Fatalf("Respond denied: %v", err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrApprovalRequestDenied) {
			t.Fatalf("approval error = %v, want ErrApprovalRequestDenied", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approval wait did not resolve")
	}
}

func TestServerResolvesDirectFileChangeApprovalResponses(t *testing.T) {
	tests := []struct {
		name      string
		decision  protocol.FileChangeApprovalDecision
		response  *protocol.Error
		malformed bool
		rawResult string
		wantOK    bool
		wantError bool
	}{
		{name: "accept", decision: protocol.FileChangeApprovalAccept, wantOK: true},
		{name: "decline", decision: protocol.FileChangeApprovalDecline},
		{name: "cancel", decision: protocol.FileChangeApprovalCancel},
		{name: "client error", response: &protocol.Error{Code: protocol.CodeInternalError, Message: "handler failed"}},
		{name: "malformed", malformed: true, wantError: true},
		{name: "unknown field", rawResult: `{"decision":"accept","extra":true}`, wantError: true},
		{name: "oversized", rawResult: `{"decision":"accept","padding":"` + strings.Repeat("x", approvalResponsePayloadMaxBytes) + `"}`, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			approvals := NewApprovalService()
			server := readyServer(WithApprovalService(approvals))
			errCh := make(chan error, 1)
			go func() {
				errCh <- approvals.FilesystemApproval(context.Background(), toolfs.Operation{
					Kind: toolfs.OperationWriteFile,
					Path: "note.txt",
				})
			}()
			req := waitForApprovalRequest(t, approvals)
			resp := protocol.Response{ID: req.ID, Error: tc.response}
			if tc.response == nil {
				if tc.rawResult != "" {
					resp.Result = json.RawMessage(tc.rawResult)
				} else if tc.malformed {
					resp.Result = json.RawMessage(`{"decision":"unknown"}`)
				} else {
					resp.Result = mustApprovalJSON(t, protocol.FileChangeRequestApprovalResponse{Decision: tc.decision})
				}
			}
			err := server.HandleResponse(context.Background(), resp)
			if (err != nil) != tc.wantError {
				t.Fatalf("HandleResponse error = %v, want error %t", err, tc.wantError)
			}
			select {
			case approvalErr := <-errCh:
				if tc.wantOK && approvalErr != nil {
					t.Fatalf("approval error = %v", approvalErr)
				}
				if !tc.wantOK && !errors.Is(approvalErr, ErrApprovalRequestDenied) {
					t.Fatalf("approval error = %v, want denied", approvalErr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("direct approval response did not resolve")
			}
			events := waitForNotificationSet(t, server, "serverRequest/resolved")
			if len(events) != 1 {
				t.Fatalf("resolved events = %#v", events)
			}
			requestID, _ := req.ID.Value().(string)
			if _, err := approvals.Respond(approvalRespondParams{RequestID: requestID, Approved: true}); err == nil {
				t.Fatal("legacy response unexpectedly resolved an already completed request")
			}
		})
	}
}

func TestFileChangeAcceptForSessionCachesOnlyTheSameTarget(t *testing.T) {
	approvals := NewApprovalService()
	server := readyServer(WithApprovalService(approvals))
	errCh := make(chan error, 1)
	go func() {
		errCh <- approvals.FilesystemApproval(context.Background(), toolfs.Operation{
			Kind: toolfs.OperationWriteFile,
			Path: "notes/../note.txt",
		})
	}()
	req := waitForApprovalRequest(t, approvals)
	if err := server.HandleResponse(context.Background(), protocol.Response{
		ID: req.ID,
		Result: mustApprovalJSON(t, protocol.FileChangeRequestApprovalResponse{
			Decision: protocol.FileChangeApprovalAcceptForSession,
		}),
	}); err != nil {
		t.Fatalf("HandleResponse: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("first approval: %v", err)
	}
	if err := approvals.FilesystemApproval(context.Background(), toolfs.Operation{
		Kind: toolfs.OperationWriteFile,
		Path: "note.txt",
	}); err != nil {
		t.Fatalf("cached same-target approval: %v", err)
	}
	if requests := approvals.DrainRequests(); len(requests) != 0 {
		t.Fatalf("same-target approval emitted requests: %#v", requests)
	}

	go func() {
		errCh <- approvals.FilesystemApproval(context.Background(), toolfs.Operation{
			Kind: toolfs.OperationWriteFile,
			Path: "other.txt",
		})
	}()
	other := waitForApprovalRequest(t, approvals)
	otherID, _ := other.ID.Value().(string)
	if _, err := approvals.Respond(approvalRespondParams{RequestID: otherID, Approved: false}); err != nil {
		t.Fatalf("cleanup response: %v", err)
	}
	if err := <-errCh; !errors.Is(err, ErrApprovalRequestDenied) {
		t.Fatalf("different-target approval = %v, want denied", err)
	}
}

func TestServerResolvesDirectCommandExecutionApprovalResponses(t *testing.T) {
	tests := []struct {
		name      string
		result    string
		response  *protocol.Error
		wantOK    bool
		wantError bool
	}{
		{name: "accept", result: `{"decision":"accept"}`, wantOK: true},
		{name: "decline", result: `{"decision":"decline"}`},
		{name: "cancel", result: `{"decision":"cancel"}`},
		{name: "client error", response: &protocol.Error{Code: protocol.CodeInternalError, Message: "handler failed"}},
		{name: "oversized client error", response: &protocol.Error{Code: protocol.CodeInternalError, Message: strings.Repeat("x", approvalResponsePayloadMaxBytes+1)}, wantError: true},
		{name: "session not offered", result: `{"decision":"acceptForSession"}`, wantError: true},
		{name: "exec amendment not offered", result: `{"decision":{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["git","status"]}}}`, wantError: true},
		{name: "network amendment not offered", result: `{"decision":{"applyNetworkPolicyAmendment":{"network_policy_amendment":{"host":"example.com","action":"allow"}}}}`, wantError: true},
		{name: "unknown decision", result: `{"decision":"approve"}`, wantError: true},
		{name: "unknown field", result: `{"decision":"accept","extra":true}`, wantError: true},
		{name: "second object", result: `{"decision":"accept"} {}`, wantError: true},
		{name: "second scalar", result: `{"decision":"accept"} true`, wantError: true},
		{name: "oversized", result: `{"decision":"accept","padding":"` + strings.Repeat("x", approvalResponsePayloadMaxBytes) + `"}`, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			approvals := NewApprovalService()
			server := readyServer(WithApprovalService(approvals))
			errCh := make(chan error, 1)
			go func() {
				errCh <- approvals.ProcessApproval(context.Background(), toolprocess.Operation{
					Kind:    toolprocess.OperationStart,
					Command: "true",
				})
			}()
			req := waitForApprovalRequest(t, approvals)
			if req.Method != "item/commandExecution/requestApproval" {
				t.Fatalf("approval method = %q", req.Method)
			}
			var params protocol.CommandExecutionApprovalRequestParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatalf("decode command approval params: %v", err)
			}
			var actions []string
			for _, decision := range params.AvailableDecisions {
				actions = append(actions, decision.Action())
			}
			if strings.Join(actions, ",") != "accept,decline,cancel" {
				t.Fatalf("available decisions = %#v", actions)
			}
			resp := protocol.Response{ID: req.ID, Error: tc.response, Result: json.RawMessage(tc.result)}
			err := server.HandleResponse(context.Background(), resp)
			if (err != nil) != tc.wantError {
				t.Fatalf("HandleResponse error = %v, want error %t", err, tc.wantError)
			}
			approvalErr := <-errCh
			if tc.wantOK {
				if approvalErr != nil {
					t.Fatalf("approval error = %v", approvalErr)
				}
			} else if !errors.Is(approvalErr, ErrApprovalRequestDenied) {
				t.Fatalf("approval error = %v, want denied", approvalErr)
			}
			requestID, _ := req.ID.Value().(string)
			if _, err := approvals.Respond(approvalRespondParams{RequestID: requestID, Approved: true}); err == nil {
				t.Fatal("legacy response unexpectedly resolved an already completed request")
			}
		})
	}
}

func TestDirectResponseForPermissionsApprovalRemainsFailClosed(t *testing.T) {
	approvals := NewApprovalService()
	server := readyServer(WithApprovalService(approvals))
	errCh := make(chan error, 1)
	go func() {
		errCh <- approvals.GitApproval(context.Background(), toolgit.Operation{
			Kind:     toolgit.OperationCommit,
			Message:  "commit",
			Mutating: true,
		})
	}()
	req := waitForApprovalRequest(t, approvals)
	if req.Method != "item/permissions/requestApproval" {
		t.Fatalf("approval method = %q", req.Method)
	}
	err := server.HandleResponse(context.Background(), protocol.Response{
		ID:     req.ID,
		Result: json.RawMessage(`{"permissions":{},"scope":"turn"}`),
	})
	if err == nil {
		t.Fatal("unbound permissions response unexpectedly succeeded")
	}
	if approvalErr := <-errCh; !errors.Is(approvalErr, ErrApprovalRequestDenied) {
		t.Fatalf("permissions approval = %v, want denied", approvalErr)
	}
}

func mustApprovalJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal approval response: %v", err)
	}
	return data
}

func TestApprovalServicePublishesProcessAndGitApprovalRequests(t *testing.T) {
	cases := []struct {
		name   string
		method string
		start  func(*ApprovalService, chan<- error)
	}{
		{
			name:   "process",
			method: "item/commandExecution/requestApproval",
			start: func(approvals *ApprovalService, errCh chan<- error) {
				go func() {
					errCh <- approvals.ProcessApproval(context.Background(), toolprocess.Operation{
						Kind:    toolprocess.OperationStart,
						Command: "printf",
						Args:    []string{"ok"},
					})
				}()
			},
		},
		{
			name:   "git",
			method: "item/permissions/requestApproval",
			start: func(approvals *ApprovalService, errCh chan<- error) {
				go func() {
					errCh <- approvals.GitApproval(context.Background(), toolgit.Operation{
						Kind:     toolgit.OperationCommit,
						Message:  "commit changes",
						Mutating: true,
					})
				}()
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			approvals := NewApprovalService()
			errCh := make(chan error, 1)
			tc.start(approvals, errCh)
			req := waitForApprovalRequest(t, approvals)
			if req.Method != tc.method {
				t.Fatalf("approval method = %q, want %q", req.Method, tc.method)
			}
			requestID, _ := req.ID.Value().(string)
			if _, err := approvals.Respond(approvalRespondParams{RequestID: requestID, Approved: true}); err != nil {
				t.Fatalf("Respond: %v", err)
			}
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("approval returned error: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("approval wait did not resolve")
			}
		})
	}
}

func waitForApprovalRequest(t *testing.T, approvals *ApprovalService) protocol.Request {
	t.Helper()
	select {
	case <-approvals.RequestSignal():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval request")
	}
	requests := approvals.DrainRequests()
	if len(requests) != 1 {
		t.Fatalf("approval requests = %#v", requests)
	}
	return requests[0]
}
