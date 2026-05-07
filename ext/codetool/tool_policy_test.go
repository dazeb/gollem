package codetool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestToolPolicyPrepareReadOnlyFiltersMutatingTools(t *testing.T) {
	prepare := toolPolicyPrepare("read-only")
	if prepare == nil {
		t.Fatal("expected read-only tool policy prepare")
	}
	tools := []core.ToolDefinition{
		{Name: "view"},
		{Name: "write"},
		{Name: "edit"},
		{Name: "multi_edit"},
		{Name: "bash"},
		{Name: "bash_status"},
		{Name: "bash_kill"},
		{Name: "execute_code"},
		{Name: "grep"},
		{Name: "delegate"},
		{Name: "spawn_teammate"},
		{Name: "shutdown_teammate"},
		{Name: "task_create"},
		{Name: "task_list"},
		{Name: "task_get"},
		{Name: "task_fail_current"},
	}

	filtered := prepare(context.Background(), &core.RunContext{}, tools)
	for _, tool := range filtered {
		switch tool.Name {
		case "write", "edit", "multi_edit", "bash", "bash_status", "bash_kill", "execute_code", "delegate", "spawn_teammate", "shutdown_teammate", "task_create", "task_fail_current":
			t.Fatalf("mutating tool %q should be filtered: %+v", tool.Name, filtered)
		}
	}
	if !hasToolDefinition(filtered, "view") || !hasToolDefinition(filtered, "grep") ||
		!hasToolDefinition(filtered, "task_list") || !hasToolDefinition(filtered, "task_get") {
		t.Fatalf("read-only tools should remain: %+v", filtered)
	}
}

func TestToolPolicyReadOnlyDeniesHiddenToolAtExecution(t *testing.T) {
	dir := t.TempDir()
	writeTool := mustTool(t, AllTools(WithWorkDir(dir), WithToolPolicy("read-only")), "write")
	model := core.NewTestModel(
		core.ToolCallResponseWithID("write", `{"path":"blocked.txt","content":"blocked"}`, "call-write"),
		core.TextResponse("done"),
	)
	agent := core.NewAgent[string](model,
		core.WithTools[string](writeTool),
		core.WithToolsPrepare[string](toolPolicyPrepare("read-only")),
	)

	result, err := agent.Run(context.Background(), "try to write")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("output = %q, want done", result.Output)
	}
	if _, err := os.Stat(filepath.Join(dir, "blocked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blocked write created file or returned unexpected stat error: %v", err)
	}

	calls := model.Calls()
	if len(calls) < 2 {
		t.Fatalf("expected retry after denied hidden tool call, got %d model call(s)", len(calls))
	}
	if hasToolDefinition(calls[0].Parameters.FunctionTools, "write") {
		t.Fatalf("write schema should be hidden from read-only request: %+v", calls[0].Parameters.FunctionTools)
	}
	if !messagesContainRetryPrompt(calls[1].Messages, "read-only tool policy") {
		t.Fatalf("second model call should receive read-only denial retry prompt: %+v", calls[1].Messages)
	}
}

func hasToolDefinition(tools []core.ToolDefinition, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func mustTool(t *testing.T, tools []core.Tool, name string) core.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Definition.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return core.Tool{}
}

func messagesContainRetryPrompt(messages []core.ModelMessage, want string) bool {
	for _, msg := range messages {
		req, ok := msg.(core.ModelRequest)
		if !ok {
			continue
		}
		for _, part := range req.Parts {
			retry, ok := part.(core.RetryPromptPart)
			if ok && strings.Contains(retry.Content, want) {
				return true
			}
		}
	}
	return false
}
