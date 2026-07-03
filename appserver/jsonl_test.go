package appserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
)

func TestServeJSONLinesStdioFlow(t *testing.T) {
	fsSvc, err := toolfs.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := NewServer(WithFilesystem(fsSvc))
	input := strings.Join([]string{
		`{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client"}}}`,
		`{"method":"initialized"}`,
		`{"id":"write","method":"fs/writeFile","params":{"path":"note.txt","content":"hello"}}`,
		`{"id":"read","method":"fs/readFile","params":{"path":"note.txt"}}`,
		"",
	}, "\n")

	var output bytes.Buffer
	if err := ServeJSONLines(context.Background(), server, strings.NewReader(input), &output); err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("output lines = %d, want 3\n%s", len(lines), output.String())
	}
	var initResp protocol.Response
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("init error: %v", initResp.Error)
	}
	assertNoJSONRPCMember(t, lines[0])

	var readResp protocol.Response
	if err := json.Unmarshal([]byte(lines[2]), &readResp); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	var read struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(readResp.Result, &read); err != nil {
		t.Fatalf("decode read result: %v", err)
	}
	if read.Content != "hello" || read.Encoding != "utf-8" {
		t.Fatalf("read result = %#v", read)
	}
}

func TestServeJSONLinesWritesTransportErrors(t *testing.T) {
	server := NewServer()
	var output bytes.Buffer
	err := ServeJSONLines(context.Background(), server, strings.NewReader("{not json}\n"), &output)
	if err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	var resp struct {
		ID    any             `json:"id"`
		Error *protocol.Error `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.ID != nil || resp.Error == nil || resp.Error.Code != protocol.CodeInvalidRequest {
		t.Fatalf("transport error = %#v", resp)
	}
}

func TestServeJSONLinesReturnsOnContextCancel(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeJSONLines(ctx, NewServer(), reader, &bytes.Buffer{})
	}()
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ServeJSONLines error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeJSONLines did not return after context cancellation")
	}
}

func assertNoJSONRPCMember(t *testing.T, line string) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if _, ok := envelope["jsonrpc"]; ok {
		t.Fatalf("response included jsonrpc member: %s", line)
	}
}
