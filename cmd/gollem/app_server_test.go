package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
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
		"--provider", "test",
		"--model", "test-model",
		"--location", "global",
		"--project", "test-project",
		"--allow-mutations",
	})
	if err != nil {
		t.Fatalf("parseAppServerFlags: %v", err)
	}
	if got.workDir != "/tmp/work" || got.storePath != "state/app.db" || got.gitRoot != "/tmp/repo" || got.worktreeRoot != "/tmp/worktrees" {
		t.Fatalf("flags = %#v", got)
	}
	if got.provider != "test" || got.modelName != "test-model" || got.location != "global" || got.project != "test-project" {
		t.Fatalf("runtime flags = %#v", got)
	}
	if !got.allowMutations || !got.stdio || !got.gitRootExplicit {
		t.Fatalf("boolean flags = %#v", got)
	}

	if _, err := parseAppServerFlags([]string{"--unknown"}); err == nil || !strings.Contains(err.Error(), "unknown app-server") {
		t.Fatalf("unknown flag error = %v", err)
	}

	network, err := parseAppServerFlags([]string{
		"--socket", filepath.Join(t.TempDir(), "gollem.sock"),
		"--websocket", "127.0.0.1:0",
		"--websocket-path", "/rpc",
	})
	if err != nil {
		t.Fatalf("parse network app-server flags: %v", err)
	}
	if network.stdio || network.socketPath == "" || network.websocketAddr != "127.0.0.1:0" || network.websocketPath != "/rpc" {
		t.Fatalf("network flags = %#v", network)
	}

	explicitStdio, err := parseAppServerFlags([]string{"--stdio=true", "--socket", filepath.Join(t.TempDir(), "gollem.sock")})
	if err != nil {
		t.Fatalf("parse explicit stdio app-server flags: %v", err)
	}
	if !explicitStdio.stdio || !explicitStdio.stdioExplicit {
		t.Fatalf("explicit stdio flags = %#v", explicitStdio)
	}

	if _, err := parseAppServerFlags([]string{"--stdio=false"}); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("missing transport error = %v", err)
	}
	if _, err := parseAppServerFlags([]string{"--websocket", "127.0.0.1:0", "--websocket-path", "rpc"}); err == nil || !strings.Contains(err.Error(), "must start with /") {
		t.Fatalf("invalid websocket path error = %v", err)
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

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := appserver.ServeJSONLines(context.Background(), server, inR, outW)
		if err != nil {
			_ = outW.CloseWithError(err)
		} else {
			_ = outW.Close()
		}
		errCh <- err
	}()
	scanner := bufio.NewScanner(outR)
	writeCLIInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client"}}}`)
	writeCLIInputLine(t, inW, `{"method":"initialized"}`)
	var initResp protocol.Response
	if err := json.Unmarshal([]byte(readCLIOutputLine(t, scanner)), &initResp); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("init error: %v", initResp.Error)
	}

	writeCLIInputLine(t, inW, `{"id":"write","method":"fs/writeFile","params":{"path":"ok.txt","content":"ok"}}`)
	seenWrite := false
	seenChanged := false
	for !seenWrite || !seenChanged {
		line := readCLIOutputLine(t, scanner)
		var envelope struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Error  *protocol.Error `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("decode output envelope %q: %v", line, err)
		}
		switch {
		case envelope.Method != "":
			if envelope.Method != "fs/changed" {
				t.Fatalf("notification method = %q, want fs/changed", envelope.Method)
			}
			var changed protocol.Notification
			if err := json.Unmarshal([]byte(line), &changed); err != nil {
				t.Fatalf("decode fs changed notification: %v", err)
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
			seenChanged = true
		case envelope.ID == "write":
			if envelope.Error != nil {
				t.Fatalf("write error: %v", envelope.Error)
			}
			seenWrite = true
		default:
			t.Fatalf("unexpected output line: %q", line)
		}
	}

	writeCLIInputLine(t, inW, `{"id":"read","method":"fs/readFile","params":{"path":"ok.txt"}}`)
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	var read protocol.Response
	if err := json.Unmarshal([]byte(readCLIOutputLine(t, scanner)), &read); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if read.Error != nil {
		t.Fatalf("read error: %v", read.Error)
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
	if err := <-errCh; err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
}

func TestCLIAppServerServesUnixSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not supported on windows")
	}
	workDir := t.TempDir()
	socketDir, err := os.MkdirTemp("/tmp", "gollem-sock-*")
	if err != nil {
		t.Fatalf("create socket temp dir: %v", err)
	}
	defer os.RemoveAll(socketDir)
	socketPath := filepath.Join(socketDir, "gollem.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- serveAppServerUnixSocket(ctx, appServerFlags{
			workDir:        workDir,
			storePath:      ":memory:",
			socketPath:     socketPath,
			allowMutations: true,
		})
	}()

	var conn net.Conn
	err = nil
	dialer := net.Dialer{}
	for range 100 {
		conn, err = dialer.DialContext(ctx, "unix", socketPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial app-server socket: %v", err)
	}
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	writeCLIInputLine(t, conn, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"socket-test"}}}`)
	writeCLIInputLine(t, conn, `{"method":"initialized"}`)
	writeCLIInputLine(t, conn, `{"id":"status","method":"daemon/status","params":{}}`)

	var initResp protocol.Response
	if err := json.Unmarshal([]byte(readCLIOutputLine(t, scanner)), &initResp); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("initialize error: %v", initResp.Error)
	}
	var statusResp protocol.Response
	if err := json.Unmarshal([]byte(readCLIOutputLine(t, scanner)), &statusResp); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if statusResp.Error != nil {
		t.Fatalf("daemon/status error: %v", statusResp.Error)
	}
	var status appserver.DaemonStatus
	if err := json.Unmarshal(statusResp.Result, &status); err != nil {
		t.Fatalf("decode daemon/status: %v", err)
	}
	if status.Transport != "socket" {
		t.Fatalf("daemon/status transport = %q, want socket", status.Transport)
	}

	writeCLIInputLine(t, conn, `{"id":"stop","method":"daemon/stop","params":{"reason":"socket test"}}`)
	var stopResp protocol.Response
	if err := json.Unmarshal([]byte(readCLIOutputLine(t, scanner)), &stopResp); err != nil {
		t.Fatalf("decode stop response: %v", err)
	}
	if stopResp.Error != nil {
		t.Fatalf("daemon/stop error: %v", stopResp.Error)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close socket: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("serveAppServerUnixSocket: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("serveAppServerUnixSocket did not exit after daemon/stop")
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

	responses := map[string]protocol.Response{}
	for _, line := range lines[1:] {
		var resp protocol.Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("decode response line %q: %v", line, err)
		}
		id, _ := resp.ID.Value().(string)
		if id == "" {
			t.Fatalf("response missing string id: %#v", resp)
		}
		responses[id] = resp
	}
	providersResp := responses["providers"]
	var providers catalog.ProviderListResponse
	if err := json.Unmarshal(providersResp.Result, &providers); err != nil {
		t.Fatalf("decode provider/list result: %v", err)
	}
	if len(providers.Data) == 0 {
		t.Fatal("provider/list returned no providers")
	}

	modelsResp := responses["models"]
	var models catalog.ModelListResponse
	if err := json.Unmarshal(modelsResp.Result, &models); err != nil {
		t.Fatalf("decode model/list result: %v", err)
	}
	if len(models.Data) != 2 {
		t.Fatalf("model/list returned %d models, want 2", len(models.Data))
	}

	toolsResp := responses["tools"]
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
	if !containsTool(tools.Data, "turn-runtime") {
		t.Fatalf("tool/list missing turn runtime tool: %#v", tools.Data)
	}
}

func TestCLIAppServerThreadStartUsesRuntimeProviderFlag(t *testing.T) {
	server, cleanup, err := newCLIAppServer(appServerFlags{
		workDir:   t.TempDir(),
		storePath: ":memory:",
		provider:  "test",
		stdio:     true,
	})
	if err != nil {
		t.Fatalf("newCLIAppServer: %v", err)
	}
	defer cleanup()
	readyCLIAppServer(t, server)

	resp := server.HandleRequest(context.Background(), protocol.Request{
		ID:     protocol.NewStringID("start"),
		Method: "thread/start",
		Params: json.RawMessage(`{"prompt":"hello from cli runtime"}`),
	})
	if resp.Error != nil {
		t.Fatalf("thread/start error: %v", resp.Error)
	}
	waitCLINotification(t, server, "turn/completed")
}

func TestCLIAppServerServesDaemonLifecycleMethods(t *testing.T) {
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
	readyCLIAppServer(t, server)

	statusResp := server.HandleRequest(context.Background(), protocol.Request{
		ID:     protocol.NewStringID("status"),
		Method: "daemon/status",
	})
	if statusResp.Error != nil {
		t.Fatalf("daemon/status error: %v", statusResp.Error)
	}
	var status appserver.DaemonStatus
	if err := json.Unmarshal(statusResp.Result, &status); err != nil {
		t.Fatalf("decode daemon/status result: %v", err)
	}
	if status.Status != "running" || status.Name != "gollem-appserver" || status.WorkDir != workDir || status.StorePath != ":memory:" || status.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("daemon/status = %#v", status)
	}

	versionResp := server.HandleRequest(context.Background(), protocol.Request{
		ID:     protocol.NewStringID("version"),
		Method: "daemon/version",
	})
	if versionResp.Error != nil {
		t.Fatalf("daemon/version error: %v", versionResp.Error)
	}
	var version appserver.DaemonVersion
	if err := json.Unmarshal(versionResp.Result, &version); err != nil {
		t.Fatalf("decode daemon/version result: %v", err)
	}
	if version.Name != "gollem-appserver" || version.ProtocolVersion != protocol.ProtocolVersion || version.GoVersion == "" {
		t.Fatalf("daemon/version = %#v", version)
	}
}

func TestCLIAppServerDaemonStatusReportsTransport(t *testing.T) {
	server, cleanup, err := newCLIAppServerWithTransport(appServerFlags{
		workDir:   t.TempDir(),
		storePath: ":memory:",
	}, "websocket")
	if err != nil {
		t.Fatalf("newCLIAppServerWithTransport: %v", err)
	}
	defer cleanup()
	readyCLIAppServer(t, server)

	statusResp := server.HandleRequest(context.Background(), protocol.Request{
		ID:     protocol.NewStringID("status"),
		Method: "daemon/status",
	})
	if statusResp.Error != nil {
		t.Fatalf("daemon/status error: %v", statusResp.Error)
	}
	var status appserver.DaemonStatus
	if err := json.Unmarshal(statusResp.Result, &status); err != nil {
		t.Fatalf("decode daemon/status result: %v", err)
	}
	if status.Transport != "websocket" {
		t.Fatalf("daemon/status transport = %q, want websocket", status.Transport)
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

func waitCLINotification(t *testing.T, server *appserver.Server, method string) {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-server.NotificationSignal():
			for _, notification := range server.DrainNotifications() {
				if notification.Method == method {
					return
				}
			}
		case <-timeout:
			t.Fatalf("timed out waiting for notification %q", method)
		}
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

func writeCLIInputLine(t *testing.T, writer io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(writer, line+"\n"); err != nil {
		t.Fatalf("write input line: %v", err)
	}
}

func readCLIOutputLine(t *testing.T, scanner *bufio.Scanner) string {
	t.Helper()
	type result struct {
		line string
		ok   bool
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		ok := scanner.Scan()
		ch <- result{line: scanner.Text(), ok: ok, err: scanner.Err()}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("scan output: %v", got.err)
		}
		if !got.ok {
			t.Fatal("output stream closed before expected line")
		}
		return got.line
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for output line")
		return ""
	}
}
