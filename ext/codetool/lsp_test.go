package codetool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestLanguageForFile(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"main.go", "go"},
		{"script.py", "python"},
		{"script.pyi", "python"},
		{"app.ts", "typescript"},
		{"app.tsx", "typescript"},
		{"app.js", "javascript"},
		{"app.jsx", "javascript"},
		{"lib.rs", "rust"},
		{"main.c", "c"},
		{"main.h", "c"},
		{"main.cpp", "cpp"},
		{"main.cc", "cpp"},
		{"main.hpp", "cpp"},
		{"Main.hs", "haskell"},
		{"Main.lhs", "haskell"},
		{"Main.java", "java"},
		{"app.rb", "ruby"},
		{"init.lua", "lua"},
		{"main.zig", "zig"},
		{"Program.cs", "csharp"},
		{"README.md", ""},
		{"data.json", ""},
		{"Makefile", ""},
	}
	for _, tt := range tests {
		got := languageForFile(tt.name)
		if got != tt.want {
			t.Errorf("languageForFile(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestFileURI(t *testing.T) {
	got := fileURI("/home/user/project/main.go")
	if got != "file:///home/user/project/main.go" {
		t.Errorf("fileURI = %q, want file:///home/user/project/main.go", got)
	}
}

func TestURIToPath(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///home/user/main.go", "/home/user/main.go"},
		{"file:///Users/trevor/project/lib.rs", "/Users/trevor/project/lib.rs"},
		{"/plain/path", "/plain/path"},
	}
	for _, tt := range tests {
		got := uriToPath(tt.uri)
		if got != tt.want {
			t.Errorf("uriToPath(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	payload := `{"jsonrpc":"2.0","id":1,"method":"test"}`
	err := writeMessage(&buf, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	expected := "Content-Length: 40\r\n\r\n" + payload
	if got != expected {
		t.Errorf("writeMessage:\ngot:  %q\nwant: %q", got, expected)
	}
}

func TestReadMessage(t *testing.T) {
	payload := `{"jsonrpc":"2.0","id":1,"result":null}`
	msg := "Content-Length: 38\r\n\r\n" + payload

	reader := bufio.NewReader(strings.NewReader(msg))
	body, err := readMessage(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != payload {
		t.Errorf("readMessage:\ngot:  %q\nwant: %q", string(body), payload)
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	original := map[string]any{
		"jsonrpc": "2.0",
		"id":      float64(42),
		"method":  "textDocument/definition",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///test.go"},
			"position":     map[string]any{"line": float64(10), "character": float64(5)},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeMessage(&buf, data); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(&buf)
	body, err := readMessage(reader)
	if err != nil {
		t.Fatal(err)
	}

	var roundTripped map[string]any
	if err := json.Unmarshal(body, &roundTripped); err != nil {
		t.Fatal(err)
	}

	if roundTripped["method"] != "textDocument/definition" {
		t.Errorf("method = %v, want textDocument/definition", roundTripped["method"])
	}
	if roundTripped["id"] != float64(42) {
		t.Errorf("id = %v, want 42", roundTripped["id"])
	}
}

func TestReadMessageMultipleHeaders(t *testing.T) {
	// LSP allows multiple headers before the blank line.
	payload := `{"test":true}`
	msg := "Content-Length: 13\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n" + payload

	reader := bufio.NewReader(strings.NewReader(msg))
	body, err := readMessage(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != payload {
		t.Errorf("got %q, want %q", string(body), payload)
	}
}

func TestReadMessageMissingContentLength(t *testing.T) {
	msg := "Content-Type: text/plain\r\n\r\n{}"
	reader := bufio.NewReader(strings.NewReader(msg))
	_, err := readMessage(reader)
	if err == nil {
		t.Error("expected error for missing Content-Length")
	}
}

func TestFindServerConfig(t *testing.T) {
	// This test checks the lookup logic, not actual binary availability.
	_, err := findServerConfig("unknown_language_xyz")
	if err == nil {
		t.Error("expected error for unknown language")
	}
	if !strings.Contains(err.Error(), "no language server configured") {
		t.Errorf("unexpected error: %v", err)
	}

	// Check that all configured languages have at least one config with an install hint.
	for lang, configs := range serverConfigs {
		if len(configs) == 0 {
			t.Errorf("language %q has empty config list", lang)
		}
		for _, cfg := range configs {
			if cfg.command == "" {
				t.Errorf("language %q has config with empty command", lang)
			}
			if cfg.installHint == "" {
				t.Errorf("language %q command %q has empty install hint", lang, cfg.command)
			}
		}
	}
}

func TestSymbolKindName(t *testing.T) {
	tests := []struct {
		kind int
		want string
	}{
		{5, "class"},
		{6, "method"},
		{12, "function"},
		{13, "variable"},
		{23, "struct"},
		{999, "symbol"}, // unknown
	}
	for _, tt := range tests {
		got := symbolKindName(tt.kind)
		if got != tt.want {
			t.Errorf("symbolKindName(%d) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestFormatLocations(t *testing.T) {
	locs := []lspLocation{
		{
			URI:   "file:///project/src/main.go",
			Range: lspRange{Start: lspPosition{Line: 41, Character: 4}},
		},
		{
			URI:   "file:///project/src/lib.go",
			Range: lspRange{Start: lspPosition{Line: 10, Character: 0}},
		},
	}
	result := formatLocations(locs, "/project", 10)
	// Should contain relative paths.
	if !strings.Contains(result, "src/main.go:42") {
		t.Errorf("expected src/main.go:42 in output, got: %s", result)
	}
	if !strings.Contains(result, "src/lib.go:11") {
		t.Errorf("expected src/lib.go:11 in output, got: %s", result)
	}
}

func TestFormatLocationsMaxResults(t *testing.T) {
	var locs []lspLocation
	for i := 0; i < 20; i++ {
		locs = append(locs, lspLocation{
			URI:   "file:///project/f.go",
			Range: lspRange{Start: lspPosition{Line: i}},
		})
	}
	result := formatLocations(locs, "/project", 5)
	if !strings.Contains(result, "and 15 more results") {
		t.Errorf("expected truncation notice, got: %s", result)
	}
}

func TestFormatDiagnostics(t *testing.T) {
	diags := []lspDiagnostic{
		{
			Range:    lspRange{Start: lspPosition{Line: 9, Character: 4}},
			Severity: 1,
			Message:  "undefined: foo",
		},
		{
			Range:    lspRange{Start: lspPosition{Line: 15, Character: 0}},
			Severity: 2,
			Message:  "unused variable bar",
		},
	}
	result := formatDiagnostics(diags, "main.go")
	if !strings.Contains(result, "main.go:10:5: error: undefined: foo") {
		t.Errorf("expected error diagnostic, got: %s", result)
	}
	if !strings.Contains(result, "main.go:16:1: warning: unused variable bar") {
		t.Errorf("expected warning diagnostic, got: %s", result)
	}
}

func TestFormatDiagnosticsEmpty(t *testing.T) {
	result := formatDiagnostics(nil, "main.go")
	if !strings.Contains(result, "No diagnostics") {
		t.Errorf("expected 'No diagnostics', got: %s", result)
	}
}

func TestFormatHover(t *testing.T) {
	t.Run("markup_content", func(t *testing.T) {
		result := json.RawMessage(`{"contents":{"kind":"plaintext","value":"func main()"},"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":4}}}`)
		got := formatHover(result)
		if !strings.Contains(got, "func main()") {
			t.Errorf("expected 'func main()', got: %s", got)
		}
	})

	t.Run("string_content", func(t *testing.T) {
		result := json.RawMessage(`{"contents":"simple hover text"}`)
		got := formatHover(result)
		if got != "simple hover text" {
			t.Errorf("expected 'simple hover text', got: %s", got)
		}
	})

	t.Run("null_result", func(t *testing.T) {
		got := formatHover(json.RawMessage(`null`))
		if !strings.Contains(got, "No hover") {
			t.Errorf("expected 'No hover', got: %s", got)
		}
	})
}

func TestFormatSymbols(t *testing.T) {
	symbols := []lspSymbolInfo{
		{
			Name: "processData",
			Kind: 12, // function
			Location: lspLocation{
				URI:   "file:///project/src/process.go",
				Range: lspRange{Start: lspPosition{Line: 41, Character: 0}},
			},
		},
		{
			Name: "Config",
			Kind: 23, // struct
			Location: lspLocation{
				URI:   "file:///project/src/config.go",
				Range: lspRange{Start: lspPosition{Line: 5, Character: 0}},
			},
		},
	}
	result := formatSymbols(symbols, "/project")
	if !strings.Contains(result, "processData [function]") {
		t.Errorf("expected processData [function], got: %s", result)
	}
	if !strings.Contains(result, "Config [struct]") {
		t.Errorf("expected Config [struct], got: %s", result)
	}
}

func TestFormatSymbolsEmpty(t *testing.T) {
	got := formatSymbols(nil, "/project")
	if !strings.Contains(got, "No symbols found") {
		t.Errorf("expected 'No symbols found', got: %s", got)
	}
}

func TestLSPParamsValidation(t *testing.T) {
	// Test that the LSP tool rejects invalid params via ModelRetryError.
	lspTool := LSP(WithWorkDir("/tmp"))

	ctx := context.Background()

	t.Run("empty_method", func(t *testing.T) {
		_, err := lspTool.Handler(ctx, &core.RunContext{}, `{"file":"test.go"}`)
		if err == nil {
			t.Fatal("expected error for empty method")
		}
		var retryErr *core.ModelRetryError
		if ok := isModelRetryError(err, &retryErr); !ok {
			t.Errorf("expected ModelRetryError, got %T: %v", err, err)
		}
	})

	t.Run("empty_file", func(t *testing.T) {
		_, err := lspTool.Handler(ctx, &core.RunContext{}, `{"method":"definition"}`)
		if err == nil {
			t.Fatal("expected error for empty file")
		}
	})

	t.Run("unknown_method", func(t *testing.T) {
		_, err := lspTool.Handler(ctx, &core.RunContext{}, `{"method":"unknown","file":"test.go"}`)
		if err == nil {
			t.Fatal("expected error for unknown method")
		}
	})

	t.Run("unsupported_extension", func(t *testing.T) {
		_, err := lspTool.Handler(ctx, &core.RunContext{}, `{"method":"definition","file":"test.xyz"}`)
		if err == nil {
			t.Fatal("expected error for unsupported extension")
		}
	})
}

func TestReadLoopHandlesServerRequests(t *testing.T) {
	// Simulate a server that sends a request (e.g., client/registerCapability)
	// before delivering the response. The readLoop should auto-respond to the
	// server request and route the actual response to the channel.

	// Create pipes to simulate server stdin/stdout.
	clientRead, serverWrite, _ := os.Pipe()  // server writes, readLoop reads
	serverRead, clientWrite, _ := os.Pipe()  // readLoop writes (via respondToServerRequest), server reads
	defer clientRead.Close()
	defer serverWrite.Close()
	defer serverRead.Close()
	defer clientWrite.Close()

	srv := &lspServer{
		stdin:     clientWrite,
		openFiles: make(map[string]fileState),
		responses: make(chan jsonrpcResponse, 16),
		readDone:  make(chan struct{}),
	}

	// Start readLoop.
	go srv.readLoop(bufio.NewReaderSize(clientRead, 64*1024))

	// Simulate server sending a request (client/registerCapability).
	serverReq := `{"jsonrpc":"2.0","id":100,"method":"client/registerCapability","params":{}}`
	writeMessage(serverWrite, []byte(serverReq))

	// Simulate server sending our response.
	serverResp := `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`
	writeMessage(serverWrite, []byte(serverResp))

	// We should receive the response on the channel.
	select {
	case resp := <-srv.responses:
		if resp.ID != 1 {
			t.Errorf("expected response ID 1, got %d", resp.ID)
		}
		if resp.Error != nil {
			t.Errorf("unexpected error: %v", resp.Error)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Verify that readLoop sent a response to the server request.
	// Read from serverRead to see the reply.
	serverReader := bufio.NewReader(serverRead)
	body, err := readMessage(serverReader)
	if err != nil {
		t.Fatalf("reading server reply: %v", err)
	}
	var reply map[string]any
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatalf("parsing reply: %v", err)
	}
	// The reply should have id=100 and result=null.
	if id, ok := reply["id"].(float64); !ok || id != 100 {
		t.Errorf("expected reply id=100, got %v", reply["id"])
	}
	if reply["result"] != nil {
		t.Errorf("expected null result, got %v", reply["result"])
	}
}

func TestReadLoopSkipsNotifications(t *testing.T) {
	clientRead, serverWrite, _ := os.Pipe()
	_, clientWrite, _ := os.Pipe()
	defer clientRead.Close()
	defer serverWrite.Close()
	defer clientWrite.Close()

	srv := &lspServer{
		stdin:     clientWrite,
		openFiles: make(map[string]fileState),
		responses: make(chan jsonrpcResponse, 16),
		readDone:  make(chan struct{}),
	}

	go srv.readLoop(bufio.NewReaderSize(clientRead, 64*1024))

	// Send a notification (no id) followed by a response.
	notification := `{"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{}}`
	writeMessage(serverWrite, []byte(notification))

	response := `{"jsonrpc":"2.0","id":5,"result":null}`
	writeMessage(serverWrite, []byte(response))

	// We should only get the response, not the notification.
	select {
	case resp := <-srv.responses:
		if resp.ID != 5 {
			t.Errorf("expected response ID 5, got %d", resp.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestCallTimeout(t *testing.T) {
	// Test that call() properly times out when the server doesn't respond.
	clientRead, _, _ := os.Pipe() // server never writes anything
	_, clientWrite, _ := os.Pipe()
	defer clientRead.Close()
	defer clientWrite.Close()

	srv := &lspServer{
		stdin:     clientWrite,
		openFiles: make(map[string]fileState),
		responses: make(chan jsonrpcResponse, 16),
		readDone:  make(chan struct{}),
	}

	// Start readLoop that will block waiting for input.
	go srv.readLoop(bufio.NewReaderSize(clientRead, 64*1024))

	// Override timeout for test by using a context with deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := srv.call(ctx, "test/method", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected context deadline error, got: %v", err)
	}
}

// isModelRetryError checks if the error chain contains a ModelRetryError.
func isModelRetryError(err error, target **core.ModelRetryError) bool {
	for err != nil {
		if mre, ok := err.(*core.ModelRetryError); ok {
			if target != nil {
				*target = mre
			}
			return true
		}
		// Check wrapped errors.
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}
