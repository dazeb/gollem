package openai

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type errAfterDataReader struct {
	data        []byte
	off         int
	err         error
	errReturned bool
}

func (r *errAfterDataReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		if r.errReturned {
			return 0, io.EOF
		}
		r.errReturned = true
		return 0, r.err
	}

	n := copy(p, r.data[r.off:])
	r.off += n
	if r.off >= len(r.data) && !r.errReturned {
		r.errReturned = true
		return n, r.err
	}
	return n, nil
}

func TestParseSSEResponses_IgnoresTransportErrorAfterTerminalEvent(t *testing.T) {
	transportErr := errors.New("stream error: stream ID 89; INTERNAL_ERROR; received from peer")
	body := io.NopCloser(&errAfterDataReader{
		data: []byte(`data: {"type":"response.completed","response":{"id":"resp_done","model":"gpt-5","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done-ok"}]}],"usage":{"input_tokens":4,"output_tokens":2}}}
`),
		err: transportErr,
	})

	p := New(WithModel("gpt-5"))
	got, err := p.parseSSEResponses(&http.Response{Body: body})
	if err != nil {
		t.Fatalf("parseSSEResponses returned error despite terminal response: %v", err)
	}
	if text := got.TextContent(); text != "done-ok" {
		t.Fatalf("response text = %q, want done-ok", text)
	}
	if got.Usage.InputTokens != 4 || got.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %+v", got.Usage)
	}
}

func TestParseSSEResponses_StillFailsWithoutTerminalEvent(t *testing.T) {
	transportErr := errors.New("stream error: stream ID 89; INTERNAL_ERROR; received from peer")
	body := io.NopCloser(&errAfterDataReader{
		data: []byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_partial\"}}\n"),
		err: transportErr,
	})

	p := New(WithModel("gpt-5"))
	_, err := p.parseSSEResponses(&http.Response{Body: body})
	if err == nil {
		t.Fatal("expected parseSSEResponses to fail without a terminal response event")
	}
	if !strings.Contains(err.Error(), "SSE read error") {
		t.Fatalf("expected SSE read error, got %v", err)
	}
	if !strings.Contains(err.Error(), "INTERNAL_ERROR") {
		t.Fatalf("expected transport error details to be preserved, got %v", err)
	}
}
