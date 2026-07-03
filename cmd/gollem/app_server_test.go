package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appserver "github.com/fugue-labs/gollem/appserver"
	"github.com/fugue-labs/gollem/appserver/catalog"
	"github.com/fugue-labs/gollem/appserver/protocol"
)

func TestParseAppServerFlags(t *testing.T) {
	got, err := parseAppServerFlags([]string{
		"--workdir", "/tmp/work",
		"--store", "state/app.db",
		"--git-root", "/tmp/repo",
		"--worktree-root", "/tmp/worktrees",
		"--allow-mutations",
	})
	if err != nil {
		t.Fatalf("parseAppServerFlags: %v", err)
	}
	if got.workDir != "/tmp/work" || got.storePath != "state/app.db" || got.gitRoot != "/tmp/repo" || got.worktreeRoot != "/tmp/worktrees" {
		t.Fatalf("flags = %#v", got)
	}
	if !got.allowMutations || !got.stdio || !got.gitRootExplicit {
		t.Fatalf("boolean flags = %#v", got)
	}

	if _, err := parseAppServerFlags([]string{"--unknown"}); err == nil || !strings.Contains(err.Error(), "unknown app-server") {
		t.Fatalf("unknown flag error = %v", err)
	}
}

func TestCLIAppServerDefaultRequiresMutationApproval(t *testing.T) {
	server, cleanup, err := newCLIAppServer(appServerFlags{
		workDir:   t.TempDir(),
		storePath: ":memory:",
		stdio:     true,
	})
	if err != nil {
		t.Fatalf("newCLIAppServer: %v", err)
	}
	defer cleanup()
	readyCLIAppServer(t, server)

	respCh := make(chan protocol.Response, 1)
	go func() {
		respCh <- server.HandleRequest(context.Background(), protocol.Request{
			ID:     protocol.NewStringID("write"),
			Method: "fs/writeFile",
			Params: json.RawMessage(`{"path":"blocked.txt","content":"nope"}`),
		})
	}()
	select {
	case <-server.RequestSignal():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval request")
	}
	requests := server.DrainRequests()
	if len(requests) != 1 || requests[0].Method != "item/fileChange/requestApproval" {
		t.Fatalf("approval requests = %#v", requests)
	}
	requestID, _ := requests[0].ID.Value().(string)
	denyResp := server.HandleRequest(context.Background(), protocol.Request{
		ID:     protocol.NewStringID("deny"),
		Method: "approval/respond",
		Params: json.RawMessage(`{"requestId":"` + requestID + `","approved":false,"message":"denied in test"}`),
	})
	if denyResp.Error != nil {
		t.Fatalf("approval/respond error: %v", denyResp.Error)
	}
	var resp protocol.Response
	select {
	case resp = <-respCh:
	case <-time.After(2 * time.Second):
		t.Fatal("write request did not finish after denied approval")
	}
	if resp.Error == nil || resp.Error.Code != protocol.CodeInvalidRequest {
		t.Fatalf("write response error = %#v, want invalid request", resp.Error)
	}
}

func TestCLIAppServerServesStdioWhenMutationsAllowed(t *testing.T) {
	workDir := t.TempDir()
	server, cleanup, err := newCLIAppServer(appServerFlags{
		workDir:        workDir,
		storePath:      filepath.Join(workDir, "app.db"),
		stdio:          true,
		allowMutations: true,
	})
	if err != nil {
		t.Fatalf("newCLIAppServer: %v", err)
	}
	defer cleanup()

	input := strings.Join([]string{
		`{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client"}}}`,
		`{"method":"initialized"}`,
		`{"id":"write","method":"fs/writeFile","params":{"path":"ok.txt","content":"ok"}}`,
		`{"id":"read","method":"fs/readFile","params":{"path":"ok.txt"}}`,
		"",
	}, "\n")
	var output bytes.Buffer
	if err := appserver.ServeJSONLines(context.Background(), server, strings.NewReader(input), &output); err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("output lines = %d, want 4\n%s", len(lines), output.String())
	}
	var changed protocol.Notification
	if err := json.Unmarshal([]byte(lines[2]), &changed); err != nil {
		t.Fatalf("decode fs changed notification: %v", err)
	}
	if changed.Method != "fs/changed" {
		t.Fatalf("notification method = %q, want fs/changed", changed.Method)
	}
	var changedParams struct {
		Path      string `json:"path"`
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal(changed.Params, &changedParams); err != nil {
		t.Fatalf("decode fs changed params: %v", err)
	}
	if changedParams.Path != "ok.txt" || changedParams.Operation != "writeFile" {
		t.Fatalf("fs changed params = %#v", changedParams)
	}
	var read protocol.Response
	if err := json.Unmarshal([]byte(lines[3]), &read); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	var result struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(read.Result, &result); err != nil {
		t.Fatalf("decode read result: %v", err)
	}
	if result.Content != "ok" {
		t.Fatalf("content = %q, want ok", result.Content)
	}
}

func TestCLIAppServerServesCatalogMethods(t *testing.T) {
	workDir := t.TempDir()
	server, cleanup, err := newCLIAppServer(appServerFlags{
		workDir:   workDir,
		storePath: ":memory:",
		stdio:     true,
	})
	if err != nil {
		t.Fatalf("newCLIAppServer: %v", err)
	}
	defer cleanup()

	input := strings.Join([]string{
		`{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client"}}}`,
		`{"method":"initialized"}`,
		`{"id":"providers","method":"provider/list","params":{}}`,
		`{"id":"models","method":"model/list","params":{"limit":2}}`,
		`{"id":"tools","method":"tool/list","params":{"includeUnavailable":true}}`,
		"",
	}, "\n")
	var output bytes.Buffer
	if err := appserver.ServeJSONLines(context.Background(), server, strings.NewReader(input), &output); err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("output lines = %d, want 4\n%s", len(lines), output.String())
	}

	var providersResp protocol.Response
	if err := json.Unmarshal([]byte(lines[1]), &providersResp); err != nil {
		t.Fatalf("decode providers response: %v", err)
	}
	var providers catalog.ProviderListResponse
	if err := json.Unmarshal(providersResp.Result, &providers); err != nil {
		t.Fatalf("decode provider/list result: %v", err)
	}
	if len(providers.Data) == 0 {
		t.Fatal("provider/list returned no providers")
	}

	var modelsResp protocol.Response
	if err := json.Unmarshal([]byte(lines[2]), &modelsResp); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	var models catalog.ModelListResponse
	if err := json.Unmarshal(modelsResp.Result, &models); err != nil {
		t.Fatalf("decode model/list result: %v", err)
	}
	if len(models.Data) != 2 {
		t.Fatalf("model/list returned %d models, want 2", len(models.Data))
	}

	var toolsResp protocol.Response
	if err := json.Unmarshal([]byte(lines[3]), &toolsResp); err != nil {
		t.Fatalf("decode tools response: %v", err)
	}
	var tools catalog.ToolListResponse
	if err := json.Unmarshal(toolsResp.Result, &tools); err != nil {
		t.Fatalf("decode tool/list result: %v", err)
	}
	if !containsTool(tools.Data, "provider-catalog") {
		t.Fatalf("tool/list missing provider catalog tool: %#v", tools.Data)
	}
	if !containsTool(tools.Data, "cache") {
		t.Fatalf("tool/list missing cache tool: %#v", tools.Data)
	}
}

func readyCLIAppServer(t *testing.T, server *appserver.Server) {
	t.Helper()
	initResp := server.HandleRequest(context.Background(), protocol.Request{
		ID:     protocol.NewStringID("init"),
		Method: "initialize",
		Params: json.RawMessage(`{"clientInfo":{"name":"test-client"}}`),
	})
	if initResp.Error != nil {
		t.Fatalf("initialize: %v", initResp.Error)
	}
	if err := server.HandleNotification(context.Background(), protocol.Notification{Method: "initialized"}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
}

func containsTool(tools []catalog.Tool, id string) bool {
	for _, tool := range tools {
		if tool.ID == id {
			return true
		}
	}
	return false
}
