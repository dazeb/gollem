package codetool

// Regression tests for three lsp.go bugs:
//  1. applyWorkspaceEdit treated LSP UTF-16 character offsets as byte
//     offsets, corrupting files with multibyte runes on rename/code_action.
//  2. lspOutline misparsed flat SymbolInformation[] responses — every
//     symbol was reported at line 1 because the payload also unmarshals
//     cleanly into []lspDocumentSymbol with a zero Range.
//  3. startServer leaked a zombie process when the initialize handshake
//     failed (kill without a corresponding Wait to reap the child).

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUTF16OffsetToByte(t *testing.T) {
	// "héllo 🚀 wörld old": é and ö are 1 UTF-16 unit / 2 bytes,
	// 🚀 is 2 UTF-16 units (surrogate pair) / 4 bytes.
	line := "héllo 🚀 wörld old"
	tests := []struct {
		name     string
		line     string
		utf16Off int
		want     int
	}{
		{"zero", line, 0, 0},
		{"ascii prefix", "hello", 3, 3},
		{"after accented rune", line, 2, 3},                     // "hé" = 2 units, 3 bytes
		{"before emoji", line, 6, 7},                            // "héllo " = 6 units, 7 bytes
		{"after emoji", line, 8, 11},                            // 🚀 = 2 units, 4 bytes
		{"start of old", line, 15, 19},                          // "héllo 🚀 wörld " = 15 units, 19 bytes
		{"end of old", line, 18, 22},                            // full line = 18 units, 22 bytes
		{"clamped past end", line, 100, len(line)},              // offsets past EOL clamp
		{"mid surrogate pair rounds to next rune", line, 7, 11}, // malformed input never splits mid-rune
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := utf16OffsetToByte(tt.line, tt.utf16Off); got != tt.want {
				t.Errorf("utf16OffsetToByte(%q, %d) = %d, want %d", tt.line, tt.utf16Off, got, tt.want)
			}
		})
	}
}

// TestApplyWorkspaceEditUTF16Offsets verifies that edits on a line with
// multibyte runes (é, ö: 2 bytes/1 unit; 🚀: 4 bytes/2 units) before the
// edit range splice at the correct byte position. Before the fix the raw
// UTF-16 offsets were used as byte indices, landing the edit 4 bytes early.
func TestApplyWorkspaceEditUTF16Offsets(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "lsp-utf16-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("héllo 🚀 wörld old\nsecond line\n")
	tmpFile.Close()

	uri := fileURI(tmpFile.Name())

	// Pipes so ensureFileOpen doesn't panic on nil stdin.
	clientRead, clientWrite, _ := os.Pipe()
	defer clientRead.Close()
	defer clientWrite.Close()

	srv := &lspServer{
		stdin:       clientWrite,
		lang:        "go",
		openFiles:   make(map[string]fileState),
		diagnostics: make(map[string][]lspDiagnostic),
	}

	// "old" spans UTF-16 units [15, 18) on line 0 (byte range [19, 22)).
	edit := &lspWorkspaceEdit{
		Changes: map[string][]lspTextEdit{
			uri: {
				{
					Range: lspRange{
						Start: lspPosition{Line: 0, Character: 15},
						End:   lspPosition{Line: 0, Character: 18},
					},
					NewText: "renamed",
				},
			},
		},
	}

	totalEdits, _, err := applyWorkspaceEdit(edit, srv, "/tmp")
	if err != nil {
		t.Fatalf("applyWorkspaceEdit: %v", err)
	}
	if totalEdits != 1 {
		t.Errorf("expected 1 edit, got %d", totalEdits)
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	want := "héllo 🚀 wörld renamed\nsecond line\n"
	if string(data) != want {
		t.Errorf("file content corrupted by UTF-16/byte offset mismatch:\ngot:  %q\nwant: %q", string(data), want)
	}
}

// TestLspOutlineFlatSymbolInformation verifies that a flat
// SymbolInformation[] response (servers without hierarchical documentSymbol
// support) reports real line numbers. Before the fix the payload was
// accepted by the []lspDocumentSymbol parse — its `location` field was
// silently dropped — so every symbol printed as L1.
func TestLspOutlineFlatSymbolInformation(t *testing.T) {
	clientRead, serverWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	serverRead, clientWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer clientRead.Close()
	defer serverWrite.Close()
	defer serverRead.Close()
	defer clientWrite.Close()

	srv := &lspServer{
		stdin:       clientWrite,
		lang:        "go",
		workDir:     "/project",
		openFiles:   map[string]fileState{},
		diagnostics: map[string][]lspDiagnostic{},
		responses:   make(chan jsonrpcResponse, 16),
		readDone:    make(chan struct{}),
	}

	go srv.readLoop(bufio.NewReaderSize(clientRead, 64*1024))

	// Flat SymbolInformation[]: line numbers live under `location`, and
	// there is no `range`/`selectionRange` at the top level.
	flatSymbols := `[
		{"name":"Foo","kind":12,"location":{"uri":"file:///project/main.go","range":{"start":{"line":41,"character":0},"end":{"line":50,"character":1}}}},
		{"name":"Bar","kind":5,"location":{"uri":"file:///project/main.go","range":{"start":{"line":7,"character":0},"end":{"line":9,"character":1}}}}
	]`

	go func() {
		readMessage(bufio.NewReader(serverRead)) // discard the request

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  json.RawMessage(flatSymbols),
		}
		respJSON, _ := json.Marshal(resp)
		writeMessage(serverWrite, respJSON)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := lspOutline(ctx, srv, "/project/main.go", "/project")
	if err != nil {
		t.Fatalf("lspOutline error: %v", err)
	}
	if !strings.Contains(result, "L42: Foo [function]") {
		t.Errorf("expected 'L42: Foo [function]' in output, got:\n%s", result)
	}
	if !strings.Contains(result, "L8: Bar [class]") {
		t.Errorf("expected 'L8: Bar [class]' in output, got:\n%s", result)
	}
	if strings.Contains(result, "L1:") {
		t.Errorf("symbols reported at line 1 — flat response parsed as DocumentSymbol[]:\n%s", result)
	}
}

// TestStartServerReapsProcessOnInitializeFailure verifies that a server
// killed because the initialize handshake failed is reaped (waited on)
// rather than left as a zombie. Before the fix the cmd.Wait goroutine was
// only started after a successful handshake, so the SIGKILLed child stayed
// a zombie for the life of the parent process.
func TestStartServerReapsProcessOnInitializeFailure(t *testing.T) {
	dir := t.TempDir()

	// Fake "LSP server" that records its PID then sleeps without ever
	// speaking LSP, so the initialize handshake times out via ctx.
	pidFile := filepath.Join(dir, "server.pid")
	script := filepath.Join(dir, "fake-lsp.sh")
	content := fmt.Sprintf("#!/bin/sh\necho $$ > %q\nexec sleep 60\n", pidFile)
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	const lang = "zombie-regression-test"
	serverConfigs[lang] = []lspServerConfig{{command: script}}
	defer delete(serverConfigs, lang)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if _, err := startServer(ctx, lang, dir); err == nil {
		t.Fatal("expected startServer to fail when the handshake times out")
	}

	// The server wrote its PID before sleeping; read it back.
	var pid int
	deadline := time.Now().Add(2 * time.Second)
	for {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, err = strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("fake server never wrote its pid file")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Once reaped, signal 0 fails with ESRCH. A zombie still "exists",
	// so without the Wait the process would be found indefinitely.
	deadline = time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return // reaped — no zombie
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still exists after kill — zombie not reaped", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
