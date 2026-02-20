package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// mockToolSource is a test ToolSource implementation.
type mockToolSource struct {
	tools       []Tool
	toolResults map[string]*ToolResult
	listErr     error
	callErr     error
	closed      bool
}

func newMockToolSource() *mockToolSource {
	return &mockToolSource{
		toolResults: make(map[string]*ToolResult),
	}
}

func (m *mockToolSource) ListTools(_ context.Context) ([]Tool, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.tools, nil
}

func (m *mockToolSource) CallTool(_ context.Context, name string, _ map[string]any) (*ToolResult, error) {
	if m.callErr != nil {
		return nil, m.callErr
	}
	result, ok := m.toolResults[name]
	if !ok {
		return nil, errors.New("tool not found")
	}
	return result, nil
}

func (m *mockToolSource) Close() error {
	m.closed = true
	return nil
}

func TestManagerAddServer(t *testing.T) {
	mgr := NewManager()

	src := newMockToolSource()
	if err := mgr.AddServer("server1", src); err != nil {
		t.Fatalf("AddServer failed: %v", err)
	}

	// Duplicate should fail.
	if err := mgr.AddServer("server1", src); err == nil {
		t.Error("expected error for duplicate server name")
	}
}

func TestManagerRemoveServer(t *testing.T) {
	mgr := NewManager()

	src := newMockToolSource()
	mgr.AddServer("server1", src)

	if err := mgr.RemoveServer("server1"); err != nil {
		t.Fatalf("RemoveServer failed: %v", err)
	}

	if !src.closed {
		t.Error("expected source to be closed")
	}

	// Removing again should fail.
	if err := mgr.RemoveServer("server1"); err == nil {
		t.Error("expected error for unknown server")
	}
}

func TestManagerTools(t *testing.T) {
	mgr := NewManager()

	src1 := newMockToolSource()
	src1.tools = []Tool{
		{Name: "read_file", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "write_file", Description: "Write a file", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	src1.toolResults["read_file"] = &ToolResult{
		Content: []Content{{Type: "text", Text: "file contents"}},
	}

	src2 := newMockToolSource()
	src2.tools = []Tool{
		{Name: "search", Description: "Search the web", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	mgr.AddServer("filesystem", src1)
	mgr.AddServer("web", src2)

	ctx := context.Background()
	tools, err := mgr.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	// Check that tools have namespaced names.
	nameMap := make(map[string]bool)
	for _, tool := range tools {
		nameMap[tool.Definition.Name] = true
	}

	if !nameMap["filesystem__read_file"] {
		t.Error("expected filesystem__read_file")
	}
	if !nameMap["filesystem__write_file"] {
		t.Error("expected filesystem__write_file")
	}
	if !nameMap["web__search"] {
		t.Error("expected web__search")
	}
}

func TestManagerToolsCallHandler(t *testing.T) {
	mgr := NewManager()

	src := newMockToolSource()
	src.tools = []Tool{
		{Name: "echo", Description: "Echo", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	src.toolResults["echo"] = &ToolResult{
		Content: []Content{{Type: "text", Text: "echoed: hello"}},
	}

	mgr.AddServer("srv", src)

	ctx := context.Background()
	tools, err := mgr.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	// Call the handler.
	result, err := tools[0].Handler(ctx, nil, `{"text":"hello"}`)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if result != "echoed: hello" {
		t.Errorf("expected 'echoed: hello', got %v", result)
	}
}

func TestManagerToolsPartialFailure(t *testing.T) {
	mgr := NewManager()

	src1 := newMockToolSource()
	src1.tools = []Tool{
		{Name: "tool1", Description: "Tool 1", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	src2 := newMockToolSource()
	src2.listErr = errors.New("connection refused")

	mgr.AddServer("good", src1)
	mgr.AddServer("bad", src2)

	ctx := context.Background()
	tools, err := mgr.Tools(ctx)
	if err != nil {
		t.Fatalf("expected no error on partial failure, got: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool from good server, got %d", len(tools))
	}
	if tools[0].Definition.Name != "good__tool1" {
		t.Errorf("expected good__tool1, got %s", tools[0].Definition.Name)
	}
}

func TestManagerToolsAllFail(t *testing.T) {
	mgr := NewManager()

	src := newMockToolSource()
	src.listErr = errors.New("connection refused")
	mgr.AddServer("bad", src)

	ctx := context.Background()
	_, err := mgr.Tools(ctx)
	if err == nil {
		t.Error("expected error when all servers fail")
	}
}

func TestManagerParseToolName(t *testing.T) {
	mgr := NewManager()

	tests := []struct {
		input      string
		wantServer string
		wantTool   string
		wantOK     bool
	}{
		{"filesystem__read_file", "filesystem", "read_file", true},
		{"web__search", "web", "search", true},
		{"no_separator", "", "no_separator", false},
	}

	for _, tc := range tests {
		server, tool, ok := mgr.ParseToolName(tc.input)
		if ok != tc.wantOK {
			t.Errorf("ParseToolName(%q): ok = %v, want %v", tc.input, ok, tc.wantOK)
		}
		if server != tc.wantServer {
			t.Errorf("ParseToolName(%q): server = %q, want %q", tc.input, server, tc.wantServer)
		}
		if tool != tc.wantTool {
			t.Errorf("ParseToolName(%q): tool = %q, want %q", tc.input, tool, tc.wantTool)
		}
	}
}

func TestManagerCustomSeparator(t *testing.T) {
	mgr := NewManager(WithSeparator("::"))

	src := newMockToolSource()
	src.tools = []Tool{
		{Name: "tool1", Description: "Tool 1", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	mgr.AddServer("srv", src)

	ctx := context.Background()
	tools, err := mgr.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Definition.Name != "srv::tool1" {
		t.Errorf("expected srv::tool1, got %s", tools[0].Definition.Name)
	}

	server, tool, ok := mgr.ParseToolName("srv::tool1")
	if !ok {
		t.Error("expected ParseToolName to succeed")
	}
	if server != "srv" {
		t.Errorf("expected server 'srv', got %q", server)
	}
	if tool != "tool1" {
		t.Errorf("expected tool 'tool1', got %q", tool)
	}
}

func TestManagerServerNames(t *testing.T) {
	mgr := NewManager()

	mgr.AddServer("alpha", newMockToolSource())
	mgr.AddServer("beta", newMockToolSource())

	names := mgr.ServerNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 server names, got %d", len(names))
	}

	nameMap := make(map[string]bool)
	for _, n := range names {
		nameMap[n] = true
	}
	if !nameMap["alpha"] || !nameMap["beta"] {
		t.Errorf("expected alpha and beta, got %v", names)
	}
}

func TestManagerClose(t *testing.T) {
	mgr := NewManager()

	src1 := newMockToolSource()
	src2 := newMockToolSource()
	mgr.AddServer("srv1", src1)
	mgr.AddServer("srv2", src2)

	if err := mgr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !src1.closed {
		t.Error("expected src1 to be closed")
	}
	if !src2.closed {
		t.Error("expected src2 to be closed")
	}

	// Servers should be removed.
	names := mgr.ServerNames()
	if len(names) != 0 {
		t.Errorf("expected no servers after close, got %d", len(names))
	}
}

func TestManagerToolDescription(t *testing.T) {
	mgr := NewManager()

	src := newMockToolSource()
	src.tools = []Tool{
		{Name: "greet", Description: "Say hello", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	mgr.AddServer("myserver", src)

	ctx := context.Background()
	tools, err := mgr.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	expected := "[myserver] Say hello"
	if tools[0].Definition.Description != expected {
		t.Errorf("expected description %q, got %q", expected, tools[0].Definition.Description)
	}
}

func TestManagerEmptyTools(t *testing.T) {
	mgr := NewManager()

	ctx := context.Background()
	tools, err := mgr.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}
