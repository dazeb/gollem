package ui

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/ext/agui"
)

type bufferedResponseWriter struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{header: make(http.Header)}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.body.Write(data)
}

func (w *bufferedResponseWriter) WriteHeader(statusCode int) {
	if w.statusCode != 0 {
		return
	}
	w.statusCode = statusCode
}

func copyHeader(dst, src http.Header) {
	for key := range dst {
		dst.Del(key)
	}
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHTMXRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("HX-Request")), "true")
}

func waitForRunMutation(run *RunRecord, before RunView, action agui.Action, timeout time.Duration) RunView {
	deadline := time.Now().Add(timeout)
	settleWindow := 60 * time.Millisecond
	lastStable := before
	lastChangeAt := time.Now()

	for {
		after := run.Snapshot()
		if runViewChanged(lastStable, after) {
			lastStable = after
			lastChangeAt = time.Now()
		}
		if runViewReflectsAction(before, after, action) && time.Since(lastChangeAt) >= settleWindow {
			return after
		}
		if time.Now().After(deadline) {
			return after
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func runViewReflectsAction(before, after RunView, action agui.Action) bool {
	switch strings.TrimSpace(action.Type) {
	case agui.ActionApproveToolCall, agui.ActionDenyToolCall:
		toolCallID := strings.TrimSpace(action.ToolCallID)
		if toolCallID == "" {
			return runViewChanged(before, after)
		}
		beforeApprovals := pendingApprovalsByID(before.PendingApprovals)
		if _, existed := beforeApprovals[toolCallID]; !existed {
			return runViewChanged(before, after)
		}
		afterApprovals := pendingApprovalsByID(after.PendingApprovals)
		_, stillPending := afterApprovals[toolCallID]
		return !stillPending
	case agui.ActionAbortSession:
		status := strings.TrimSpace(after.Status)
		return status == "aborted" || status == "failed" || after.StatusView.IsTerminal
	default:
		return runViewChanged(before, after)
	}
}

func runViewChanged(before, after RunView) bool {
	if before.Status != after.Status || !before.UpdatedAt.Equal(after.UpdatedAt) {
		return true
	}
	if len(before.PendingApprovals) != len(after.PendingApprovals) {
		return true
	}
	beforeApprovals := pendingApprovalsByID(before.PendingApprovals)
	afterApprovals := pendingApprovalsByID(after.PendingApprovals)
	if len(beforeApprovals) != len(afterApprovals) {
		return true
	}
	for key, beforeApproval := range beforeApprovals {
		afterApproval, ok := afterApprovals[key]
		if !ok || beforeApproval != afterApproval {
			return true
		}
	}
	return false
}
