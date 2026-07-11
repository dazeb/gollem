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
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
)

func TestServeJSONLinesStdioFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fsSvc, err := toolfs.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := NewServer(WithFilesystem(fsSvc))
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

	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client","version":"1.0.0"}}}`)
	writeInputLine(t, inW, `{"method":"initialized"}`)

	var initResp protocol.Response
	initLine := readOutputLine(t, scanner)
	if err := json.Unmarshal([]byte(initLine), &initResp); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("init error: %v", initResp.Error)
	}
	assertNoJSONRPCMember(t, initLine)

	writeInputLine(t, inW, `{"id":"write","method":"fs/writeFile","params":{"path":"note.txt","content":"hello"}}`)
	seenWrite := false
	seenChanged := false
	for !seenWrite || !seenChanged {
		line := readOutputLine(t, scanner)
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
			var changedParams fileChangedParams
			if err := json.Unmarshal(changed.Params, &changedParams); err != nil {
				t.Fatalf("decode fs changed params: %v", err)
			}
			if changedParams.Path != "note.txt" || changedParams.Operation != "writeFile" {
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

	writeInputLine(t, inW, `{"id":"read","method":"fs/readFile","params":{"path":"note.txt"}}`)
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	var readResp protocol.Response
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &readResp); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if readResp.Error != nil {
		t.Fatalf("read error: %v", readResp.Error)
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
	if err := <-errCh; err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
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
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client","version":"1.0.0"}}}`)
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

func TestServeJSONLinesDirectFileChangeApprovalResponseFlow(t *testing.T) {
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
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"direct-approval-client","version":"1.0.0"}}}`)
	writeInputLine(t, inW, `{"method":"initialized"}`)
	writeInputLine(t, inW, `{"id":"write","method":"fs/writeFile","params":{"path":"approved.txt","content":"ok"}}`)

	var initResp protocol.Response
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &initResp); err != nil || initResp.Error != nil {
		t.Fatalf("initialize response = %#v, error=%v", initResp, err)
	}
	var approvalReq protocol.Request
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &approvalReq); err != nil {
		t.Fatalf("decode approval request: %v", err)
	}
	requestID, _ := approvalReq.ID.Value().(string)
	if approvalReq.Method != "item/fileChange/requestApproval" || requestID == "" {
		t.Fatalf("approval request = %#v", approvalReq)
	}
	writeInputLine(t, inW, `{"id":"`+requestID+`","result":{"decision":"accept"}}`)
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}

	seenWrite := false
	seenResolved := false
	seenChanged := false
	for scanner.Scan() {
		var envelope struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			t.Fatalf("decode output %q: %v", scanner.Text(), err)
		}
		seenWrite = seenWrite || envelope.ID == "write"
		seenResolved = seenResolved || envelope.Method == "serverRequest/resolved"
		seenChanged = seenChanged || envelope.Method == "fs/changed"
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	if !seenWrite || !seenResolved || !seenChanged {
		t.Fatalf("outputs: write=%t resolved=%t changed=%t", seenWrite, seenResolved, seenChanged)
	}
}

func TestServeJSONLinesDirectCommandApprovalResponseFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	approvals := NewApprovalService()
	server := NewServer(WithApprovalService(approvals))
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
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"command-approval-client","version":"1.0.0"}}}`)
	writeInputLine(t, inW, `{"method":"initialized"}`)
	var initResp protocol.Response
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &initResp); err != nil || initResp.Error != nil {
		t.Fatalf("initialize response = %#v, error=%v", initResp, err)
	}

	approvalResult := make(chan error, 1)
	go func() {
		approvalResult <- approvals.ProcessApproval(ctx, toolprocess.Operation{
			Kind:    toolprocess.OperationStart,
			Command: "printf",
			Args:    []string{"ok"},
		})
	}()
	var approvalReq protocol.Request
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &approvalReq); err != nil {
		t.Fatalf("decode command approval request: %v", err)
	}
	requestID, _ := approvalReq.ID.Value().(string)
	if approvalReq.Method != "item/commandExecution/requestApproval" || requestID == "" {
		t.Fatalf("command approval request = %#v", approvalReq)
	}
	var params protocol.CommandExecutionApprovalRequestParams
	if err := json.Unmarshal(approvalReq.Params, &params); err != nil {
		t.Fatalf("decode command approval params: %v", err)
	}
	if len(params.AvailableDecisions) != 3 || params.AvailableDecisions[0].Action() != protocol.CommandExecutionApprovalAccept {
		t.Fatalf("available command decisions = %#v", params.AvailableDecisions)
	}
	writeInputLine(t, inW, `{"id":"`+requestID+`","result":{"decision":"accept"}}`)
	if err := <-approvalResult; err != nil {
		t.Fatalf("command approval result: %v", err)
	}
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}

	seenResolved := false
	for scanner.Scan() {
		var envelope struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			t.Fatalf("decode output %q: %v", scanner.Text(), err)
		}
		seenResolved = seenResolved || envelope.Method == "serverRequest/resolved"
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	if !seenResolved {
		t.Fatal("missing command approval resolved notification")
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
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client","version":"1.0.0"}}}`)
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

func TestServeJSONLinesDaemonStopFlushesResponse(t *testing.T) {
	server := NewServer(WithDaemonService(NewDaemonService(WithDaemonVersion("test-version"))))
	input := strings.Join([]string{
		`{"id":"init","method":"initialize","params":{"clientInfo":{"name":"test-client","version":"1.0.0"}}}`,
		`{"method":"initialized"}`,
		`{"id":"stop","method":"daemon/stop","params":{"reason":"test"}}`,
		"",
	}, "\n")
	var output bytes.Buffer
	if err := ServeJSONLines(context.Background(), server, strings.NewReader(input), &output); err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d, want 2\n%s", len(lines), output.String())
	}
	var stopResp protocol.Response
	if err := json.Unmarshal([]byte(lines[1]), &stopResp); err != nil {
		t.Fatalf("decode stop response: %v", err)
	}
	if stopResp.Error != nil {
		t.Fatalf("daemon/stop error: %v", stopResp.Error)
	}
	var stop DaemonStopResult
	if err := json.Unmarshal(stopResp.Result, &stop); err != nil {
		t.Fatalf("decode stop result: %v", err)
	}
	if !stop.OK || !stop.Stopping || stop.Status.Status != "stopping" {
		t.Fatalf("stop result = %#v", stop)
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
