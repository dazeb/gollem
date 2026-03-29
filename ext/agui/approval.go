package agui

import (
	"context"
	"fmt"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// ApprovalResult is the outcome of an approval request.
type ApprovalResult struct {
	Approved bool
	Message  string
}

// ApprovalBridge provides an async-compatible implementation of
// core.ToolApprovalFunc that blocks until Resolve() is called from
// the AGUI transport layer. This bridges the synchronous core approval
// interface with the event-driven AGUI model.
//
// Usage:
//
//	bridge := agui.NewApprovalBridge()
//	agent := core.NewAgent[T](model,
//	    core.WithToolApproval[T](bridge.ToolApprovalFunc()),
//	)
//
// When a tool requires approval, the bridge blocks the agent goroutine.
// The AGUI adapter surfaces the request as an event, and the UI calls
// bridge.Resolve() to unblock the agent.
//
// The returned ToolApprovalFunc is safe to reuse across multiple calls.
type ApprovalBridge struct {
	mu      sync.Mutex
	pending map[string]*pendingApproval

	// OnRequest is called when an approval is requested, before blocking.
	// It is called under the bridge lock, so Resolve() for the same tool
	// call ID will block until OnRequest returns. This guarantees event
	// ordering: the approval.requested event is emitted before any
	// resolution can occur.
	// If nil, the bridge still blocks — the adapter must use the event bus instead.
	OnRequest func(req ApprovalRequest)
}

type pendingApproval struct {
	ch   chan ApprovalResult
	done chan struct{} // closed when the waiter has consumed or abandoned the result
}

// ApprovalRequest describes a pending approval.
type ApprovalRequest struct {
	ToolCallID string
	ToolName   string
	ArgsJSON   string
}

// NewApprovalBridge creates a new approval bridge.
func NewApprovalBridge() *ApprovalBridge {
	return &ApprovalBridge{
		pending: make(map[string]*pendingApproval),
	}
}

// ToolApprovalFunc returns a core.ToolApprovalFunc that blocks until
// Resolve() is called for the corresponding tool call ID. The tool call
// ID is extracted from the context via core.ToolCallIDFromContext().
func (b *ApprovalBridge) ToolApprovalFunc() core.ToolApprovalFunc {
	return func(ctx context.Context, toolName, argsJSON string) (bool, error) {
		toolCallID := core.ToolCallIDFromContext(ctx)
		if toolCallID == "" {
			return false, fmt.Errorf("agui: no tool call ID in context for approval")
		}

		pa := &pendingApproval{
			ch:   make(chan ApprovalResult, 1),
			done: make(chan struct{}),
		}

		// Register cleanup BEFORE the OnRequest call so it runs even
		// if OnRequest panics (the defer is already on the stack).
		defer func() {
			close(pa.done)
			b.mu.Lock()
			delete(b.pending, toolCallID)
			b.mu.Unlock()
		}()

		b.mu.Lock()
		b.pending[toolCallID] = pa
		// Call OnRequest under lock to guarantee the event is emitted
		// before any Resolve() call for this tool call ID can proceed.
		func() {
			defer b.mu.Unlock()
			if b.OnRequest != nil {
				b.OnRequest(ApprovalRequest{
					ToolCallID: toolCallID,
					ToolName:   toolName,
					ArgsJSON:   argsJSON,
				})
			}
		}()

		select {
		case result := <-pa.ch:
			return result.Approved, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

// Resolve unblocks a pending approval request. Returns true if the
// tool call ID had a pending request and the result was delivered,
// false if it was already resolved, context-cancelled, or never existed.
func (b *ApprovalBridge) Resolve(toolCallID string, approved bool, message string) bool {
	b.mu.Lock()
	pa, ok := b.pending[toolCallID]
	b.mu.Unlock()

	if !ok {
		return false
	}

	result := ApprovalResult{Approved: approved, Message: message}

	// Use select to avoid blocking if the waiter already abandoned
	// (context cancelled). The channel has capacity 1 so this won't
	// block if the waiter hasn't read yet.
	select {
	case pa.ch <- result:
		// Sent successfully. Wait for the waiter to acknowledge
		// consumption so we can report success accurately.
		<-pa.done
		return true
	case <-pa.done:
		// Waiter already abandoned (context cancelled).
		return false
	}
}

// PendingCount returns the number of pending approval requests.
func (b *ApprovalBridge) PendingCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

// PendingIDs returns the tool call IDs of all pending approval requests.
func (b *ApprovalBridge) PendingIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]string, 0, len(b.pending))
	for id := range b.pending {
		ids = append(ids, id)
	}
	return ids
}
