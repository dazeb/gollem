package ui

import (
	"bytes"
	"net/http"
	"strings"
	"time"
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

func waitForRunMutation(run *RunRecord, before RunView, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		after := run.Snapshot()
		if runViewChanged(before, after) {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
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
