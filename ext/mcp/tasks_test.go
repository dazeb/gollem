package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestTasksDiscoverAdvertisesExtension verifies the server advertises the
// tasks extension in discover when WithTasks is set, and omits it otherwise.
func TestTasksDiscoverAdvertisesExtension(t *testing.T) {
	without := NewServer()
	withoutRes := without.handleDiscover()
	if withoutRes.Capabilities.Extensions != nil {
		if _, ok := withoutRes.Capabilities.Extensions[ExtensionTasks]; ok {
			t.Fatalf("tasks advertised without WithTasks")
		}
	}

	with := NewServer(WithTasks())
	withRes := with.handleDiscover()
	if withRes.Capabilities.Extensions == nil {
		t.Fatal("expected extensions map")
	}
	if _, ok := withRes.Capabilities.Extensions[ExtensionTasks]; !ok {
		t.Fatalf("expected tasks extension to be advertised, got %+v", withRes.Capabilities.Extensions)
	}
}

// TestTasksServerReturnsTaskToDeclaringClient verifies a tool handler that
// calls CreateTask returns a CreateTaskResult (resultType "task") to a client
// that declared the tasks extension, and that tasks/get transitions
// working -> completed and returns the final ToolResult.
func TestTasksServerReturnsTaskToDeclaringClient(t *testing.T) {
	server := NewServer(WithTasks(), WithServerInfo(ServerInfo{Name: "tasks-test", Version: "1.0.0"}))
	server.AddTool(Tool{
		Name:        "long_running",
		Description: "returns a task then completes",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, rc *RequestContext, _ map[string]any) (*ToolResult, error) {
		ctr, err := rc.CreateTask(Task{
			Status:         TaskStatusWorking,
			PollIntervalMs: int64Ptr(20),
		})
		if err != nil {
			return nil, err
		}
		taskID := ctr.Task.TaskID
		// Simulate async completion: advance the task after a short delay.
		go func() {
			time.Sleep(40 * time.Millisecond)
			server.UpdateTask(taskID, func(t *Task) {
				t.Status = TaskStatusCompleted
				t.Result = mustRawJSON(tMarshal(textToolResult("done")))
			})
		}()
		return nil, nil
	})

	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, WithTasksExtension(ClientConfig{}))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	var observed []*Task
	result, err := client.CallToolAwait(ctx, "long_running", map[string]any{}, func(task *Task) {
		observed = append(observed, task)
	})
	if err != nil {
		t.Fatalf("CallToolAwait: %v", err)
	}
	if result == nil || result.TextContent() != "done" {
		t.Fatalf("unexpected tool result: %+v", result)
	}
	if len(observed) == 0 {
		t.Fatal("expected at least one poll observation")
	}
}

// TestTasksServerRefusesTaskToNonDeclaringClient verifies the server does not
// return a task to a client that did not declare the tasks extension:
// CreateTask returns an error and the handler falls back to a synchronous result.
func TestTasksServerRefusesTaskToNonDeclaringClient(t *testing.T) {
	server := NewServer(WithTasks(), WithServerInfo(ServerInfo{Name: "tasks-fallback", Version: "1.0.0"}))
	server.AddTool(Tool{
		Name:        "maybe_task",
		Description: "falls back when client lacks tasks",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, rc *RequestContext, _ map[string]any) (*ToolResult, error) {
		if _, err := rc.CreateTask(Task{Status: TaskStatusWorking}); err == nil {
			// Client declared the tasks extension; this test expects a fallback.
			return textToolResult("should-not-happen"), nil
		}
		// Client did not declare tasks; fall back to a synchronous result.
		return textToolResult("synchronous"), nil
	})

	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Client does NOT advertise the tasks extension.
	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, ClientConfig{})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	result, err := client.CallTool(ctx, "maybe_task", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if got := result.TextContent(); got != "synchronous" {
		t.Fatalf("expected synchronous fallback, got %q", got)
	}
}

// TestTasksInputRequiredAndUpdate verifies the input_required flow: a task
// reaches input_required, the client submits responses via tasks/update, the
// application consumes them and completes the task, and tasks/get reports
// completed with the final result.
func TestTasksInputRequiredAndUpdate(t *testing.T) {
	server := NewServer(WithTasks(), WithServerInfo(ServerInfo{Name: "tasks-input", Version: "1.0.0"}))
	server.AddTool(Tool{
		Name:        "need_input",
		Description: "task that needs input then completes",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, rc *RequestContext, _ map[string]any) (*ToolResult, error) {
		ctr, err := rc.CreateTask(Task{
			Status: TaskStatusWorking,
			InputRequests: InputRequests{
				"answer": {Method: "elicitation/create", Params: json.RawMessage(`{"message":"pick a number"}`)},
			},
		})
		if err != nil {
			return nil, err
		}
		taskID := ctr.Task.TaskID
		go func() {
			// Move to input_required so the client can see it.
			server.UpdateTask(taskID, func(t *Task) { t.Status = TaskStatusInputRequired })
			// Wait for the client to submit input responses.
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				entry, ok := server.tasks.get(taskID)
				if !ok {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				responses := entry.PendingInputs()
				if len(responses) > 0 {
					var answer string
					for _, raw := range responses {
						var el ElicitationResult
						if json.Unmarshal(raw, &el) == nil {
							if v, ok := el.Content["number"].(string); ok {
								answer = v
							}
						}
					}
					server.UpdateTask(taskID, func(t *Task) {
						t.Status = TaskStatusCompleted
						t.Result = mustRawJSON(tMarshal(textToolResult("got " + answer)))
					})
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
		return nil, nil
	})

	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, WithTasksExtension(ClientConfig{
		ElicitationHandler: func(_ context.Context, _ *ElicitationParams) (*ElicitationResult, error) {
			return &ElicitationResult{Action: "accept", Content: map[string]any{"number": "42"}}, nil
		},
	}))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	// Issue the call; expect an InputRequiredTaskError.
	result, err := client.CallToolAwait(ctx, "need_input", map[string]any{}, nil)
	if err == nil {
		t.Fatalf("expected InputRequiredTaskError, got result %+v", result)
	}
	var ire *InputRequiredTaskError
	if !errors.As(err, &ire) {
		t.Fatalf("expected *InputRequiredTaskError, got %T: %v", err, err)
	}
	taskID := ire.Task.TaskID

	// Fulfill the input request via tasks/update. The client's elicitation
	// handler produces the response payload keyed to "answer".
	respPayload, _ := json.Marshal(&ElicitationResult{Action: "accept", Content: map[string]any{"number": "42"}})
	if err := client.UpdateTask(ctx, taskID, InputResponses{"answer": respPayload}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	// Poll tasks/get until completed.
	deadline := time.Now().Add(3 * time.Second)
	var final *Task
	for time.Now().Before(deadline) {
		got, gerr := client.GetTask(ctx, taskID)
		if gerr != nil {
			t.Fatalf("GetTask: %v", gerr)
		}
		if got.Status == TaskStatusCompleted {
			final = got
			break
		}
		if got.Status == TaskStatusFailed || got.Status == TaskStatusCancelled {
			t.Fatalf("task reached %s: %+v", got.Status, got)
		}
		time.Sleep(15 * time.Millisecond)
	}
	if final == nil {
		t.Fatal("task did not complete in time")
	}
	var toolResult ToolResult
	if err := json.Unmarshal(final.Result, &toolResult); err != nil {
		t.Fatalf("parse final result: %v", err)
	}
	if toolResult.TextContent() != "got 42" {
		t.Fatalf("unexpected final result: %q", toolResult.TextContent())
	}
}

// TestTasksCancel verifies tasks/cancel acknowledges with an empty result and
// the task snapshot reflects the cancelled intent.
func TestTasksCancel(t *testing.T) {
	server := NewServer(WithTasks(), WithServerInfo(ServerInfo{Name: "tasks-cancel", Version: "1.0.0"}))
	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, WithTasksExtension(ClientConfig{}))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	// Create a task directly via the store through a tool handler.
	var taskID string
	var taskIDMu sync.Mutex
	server.AddTool(Tool{
		Name:        "cancellable",
		Description: "creates a cancellable task",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, rc *RequestContext, _ map[string]any) (*ToolResult, error) {
		ctr, err := rc.CreateTask(Task{Status: TaskStatusWorking})
		if err != nil {
			return nil, err
		}
		taskIDMu.Lock()
		taskID = ctr.Task.TaskID
		taskIDMu.Unlock()
		return nil, nil
	})

	// Drive the call with CallToolAwait; it will return InputRequiredTaskError
	// only if input_required, but here the task stays working, so we expect a
	// context deadline. Instead, use a goroutine and cancel via CancelTask.
	awaitErr := make(chan error, 1)
	go func() {
		_, e := client.CallToolAwait(ctx, "cancellable", map[string]any{}, nil)
		awaitErr <- e
	}()

	// Wait for the task id to be minted.
	deadline := time.Now().Add(2 * time.Second)
	for {
		taskIDMu.Lock()
		id := taskID
		taskIDMu.Unlock()
		if id != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for task id")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := client.CancelTask(ctx, taskID); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}

	got, err := client.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask after cancel: %v", err)
	}
	if got.Status != TaskStatusCancelled {
		t.Fatalf("expected cancelled status, got %q", got.Status)
	}

	cancel()
	<-awaitErr
}

// TestTasksGetNotFound verifies tasks/get on an unknown id returns -32602.
func TestTasksGetNotFound(t *testing.T) {
	server := NewServer(WithTasks(), WithServerInfo(ServerInfo{Name: "tasks-get", Version: "1.0.0"}))
	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, WithTasksExtension(ClientConfig{}))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	_, err = client.GetTask(ctx, "task_does_not_exist")
	if err == nil {
		t.Fatal("expected error for unknown task")
	}
	var rpcErr *jsonRPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *jsonRPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != jsonRPCCodeInvalidParams {
		t.Fatalf("expected -32602, got %d", rpcErr.Code)
	}
}

// TestTasksMethodNotEnabled verifies that when WithTasks is not set, tasks/get
// returns method not found.
func TestTasksMethodNotEnabled(t *testing.T) {
	server := NewServer()
	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, WithTasksExtension(ClientConfig{}))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	_, err = client.GetTask(ctx, "any")
	if err == nil {
		t.Fatal("expected error when tasks not enabled")
	}
	var rpcErr *jsonRPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *jsonRPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != jsonRPCCodeMethodNotFound {
		t.Fatalf("expected -32601, got %d: %s", rpcErr.Code, rpcErr.Message)
	}
}

func int64Ptr(v int64) *int64 { return &v }
