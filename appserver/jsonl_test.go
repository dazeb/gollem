package appserver

import (
	"bufio"
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
	if len(lines) != 4 {
		t.Fatalf("output lines = %d, want 4\n%s", len(lines), output.String())
	}
	var initResp protocol.Response
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("init error: %v", initResp.Error)
	}
	assertNoJSONRPCMember(t, lines[0])

	var changed protocol.Notification
	if err := json.Unmarshal([]byte(lines[2]), &changed); err != nil {
		t.Fatalf("decode fs changed notification: %v", err)
	}
	if changed.Method != "fs/changed" {
		t.Fatalf("notification method = %q, want fs/changed", changed.Method)
	}
	var changedParams fileChangedParams
	if err := json.Unmarshal(changed.Params, &changedParams); err != nil {
		t.Fatalf("decode fs changed params: %v", err)
	}
	if changedParams.Path != "note.txt" || changedParams.Operation != "writeFile" {
		t.Fatalf("fs changed params = %#v", changedParams)
	}

	var readResp protocol.Response
	if err := json.Unmarshal([]byte(lines[3]), &readResp); err != nil {
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

func TestServeJSONLinesApprovalRespondFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	approvals := NewApprovalService()
	fsSvc, err := toolfs.NewService(t.TempDir(), toolfs.WithApproval(approvals.FilesystemApproval))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := NewServer(WithFilesystem(fsSvc), WithApprovalService(approvals))
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := ServeJSONLines(ctx, server, inR, outW)
		if err != nil {
			_ = outW.CloseWithError(err)
		} else {
			_ = outW.Close()
		}
		errCh <- err
	}()
	scanner := bufio.NewScanner(outR)
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client"}}}`)
	writeInputLine(t, inW, `{"method":"initialized"}`)
	writeInputLine(t, inW, `{"id":"write","method":"fs/writeFile","params":{"path":"approved.txt","content":"ok"}}`)

	var initResp protocol.Response
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &initResp); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("init error: %v", initResp.Error)
	}
	var approvalReq protocol.Request
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &approvalReq); err != nil {
		t.Fatalf("decode approval request: %v", err)
	}
	if approvalReq.Method != "item/fileChange/requestApproval" {
		t.Fatalf("approval method = %q", approvalReq.Method)
	}
	requestID, _ := approvalReq.ID.Value().(string)
	if requestID == "" {
		t.Fatalf("approval request id = %#v", approvalReq.ID.Value())
	}
	writeInputLine(t, inW, `{"id":"approve","method":"approval/respond","params":{"requestId":"`+requestID+`","approved":true}}`)
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}

	seenIDs := map[string]bool{}
	seenMethods := map[string]bool{}
	for scanner.Scan() {
		var envelope struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		line := scanner.Text()
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("decode output line %q: %v", line, err)
		}
		if id, ok := envelope.ID.(string); ok {
			seenIDs[id] = true
		}
		if envelope.Method != "" {
			seenMethods[envelope.Method] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	for _, id := range []string{"approve", "write"} {
		if !seenIDs[id] {
			t.Fatalf("missing response id %q in %#v", id, seenIDs)
		}
	}
	for _, method := range []string{"serverRequest/resolved", "fs/changed"} {
		if !seenMethods[method] {
			t.Fatalf("missing notification method %q in %#v", method, seenMethods)
		}
	}
}

func TestServeJSONLinesBackpressureAllowsApprovalRespond(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	approvals := NewApprovalService()
	fsSvc, err := toolfs.NewService(t.TempDir(), toolfs.WithApproval(approvals.FilesystemApproval))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := NewServer(WithFilesystem(fsSvc), WithApprovalService(approvals), WithRequestSchedulerLimit(1))
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := ServeJSONLines(ctx, server, inR, outW)
		if err != nil {
			_ = outW.CloseWithError(err)
		} else {
			_ = outW.Close()
		}
		errCh <- err
	}()
	scanner := bufio.NewScanner(outR)
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client"}}}`)
	var initResp protocol.Response
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &initResp); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("init error: %v", initResp.Error)
	}
	writeInputLine(t, inW, `{"method":"initialized"}`)
	writeInputLine(t, inW, `{"id":"write1","method":"fs/writeFile","params":{"path":"one.txt","content":"one"}}`)
	writeInputLine(t, inW, `{"id":"write2","method":"fs/writeFile","params":{"path":"two.txt","content":"two"}}`)

	var requestID string
	var sawOverload bool
	for range 2 {
		line := readOutputLine(t, scanner)
		var envelope struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Error  *protocol.Error `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("decode early output %q: %v", line, err)
		}
		switch {
		case envelope.Method != "":
			var approvalReq protocol.Request
			if err := json.Unmarshal([]byte(line), &approvalReq); err != nil {
				t.Fatalf("decode approval request from %q: %v", line, err)
			}
			if approvalReq.Method != "item/fileChange/requestApproval" {
				t.Fatalf("approval request method = %q", approvalReq.Method)
			}
			requestID, _ = approvalReq.ID.Value().(string)
		case envelope.Error != nil:
			if envelope.Error.Code != protocol.CodeOverloaded {
				t.Fatalf("early error response = %#v", envelope.Error)
			}
			sawOverload = true
		default:
			t.Fatalf("unexpected early output: %q", line)
		}
	}
	if requestID == "" {
		t.Fatal("did not receive approval request")
	}
	if !sawOverload {
		t.Fatal("did not receive overload response")
	}

	writeInputLine(t, inW, `{"id":"approve","method":"approval/respond","params":{"requestId":"`+requestID+`","approved":true}}`)
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}

	seenIDs := map[string]bool{}
	seenMethods := map[string]bool{}
	for scanner.Scan() {
		var envelope struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		line := scanner.Text()
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("decode output line %q: %v", line, err)
		}
		if id, ok := envelope.ID.(string); ok {
			seenIDs[id] = true
		}
		if envelope.Method != "" {
			seenMethods[envelope.Method] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	for _, id := range []string{"approve", "write1"} {
		if !seenIDs[id] {
			t.Fatalf("missing response id %q in %#v", id, seenIDs)
		}
	}
	for _, method := range []string{"serverRequest/resolved", "fs/changed"} {
		if !seenMethods[method] {
			t.Fatalf("missing notification method %q in %#v", method, seenMethods)
		}
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

func writeInputLine(t *testing.T, writer *io.PipeWriter, line string) {
	t.Helper()
	if _, err := io.WriteString(writer, line+"\n"); err != nil {
		t.Fatalf("write input line: %v", err)
	}
}

func readOutputLine(t *testing.T, scanner *bufio.Scanner) string {
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
