package appserver

import (
	"context"
	"encoding/json"
	"errors"
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
