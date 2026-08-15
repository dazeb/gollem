package anthropic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// RequestTrace is a secret-safe record for one physical Anthropic HTTP
// request. It contains timing, request size, normalized usage, and sanitized
// error markers only; it never contains credentials, request content, headers,
// endpoint URLs, or raw provider error bodies.
type RequestTrace struct {
	Transport     string
	Model         string
	StartedAt     time.Time
	TotalDuration time.Duration

	RequestBytes int
	InputItems   int

	TimeToHeaders    time.Duration
	TimeToFirstEvent time.Duration
	TimeToFirstToken time.Duration
	TimeToTerminal   time.Duration

	HTTPStatus          int
	ErrorClassification string
	RetryAfter          time.Duration
	ErrorClass          string
	CacheReadTokens     int
	CacheWriteTokens    int
}

// RequestObserver receives one trace per completed, failed, or closed
// Anthropic provider request. It runs in the request/streaming goroutine and
// must be safe for concurrent use.
type RequestObserver func(RequestTrace)

// DefaultStderrRequestObserver returns a compact, secret-safe trace renderer.
func DefaultStderrRequestObserver() RequestObserver {
	return func(trace RequestTrace) {
		fmt.Fprintln(os.Stderr, formatRequestTrace(trace))
	}
}

func formatRequestTrace(trace RequestTrace) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[gollem-anthropic-trace] transport=%s model=%s started=%s total=%s", trace.Transport, trace.Model, trace.StartedAt.Format(time.RFC3339Nano), trace.TotalDuration)
	if trace.RequestBytes > 0 || trace.InputItems > 0 {
		fmt.Fprintf(&b, " req_bytes=%d input_items=%d", trace.RequestBytes, trace.InputItems)
	}
	if trace.TimeToHeaders > 0 {
		fmt.Fprintf(&b, " ttfb=%s", trace.TimeToHeaders)
	}
	if trace.TimeToFirstEvent > 0 {
		fmt.Fprintf(&b, " first_event=%s", trace.TimeToFirstEvent)
	}
	if trace.TimeToFirstToken > 0 {
		fmt.Fprintf(&b, " first_token=%s", trace.TimeToFirstToken)
	}
	if trace.TimeToTerminal > 0 {
		fmt.Fprintf(&b, " terminal=%s", trace.TimeToTerminal)
	}
	if trace.HTTPStatus != 0 {
		fmt.Fprintf(&b, " status=%d", trace.HTTPStatus)
	}
	if trace.ErrorClassification != "" {
		fmt.Fprintf(&b, " err=%s", trace.ErrorClassification)
	}
	if trace.RetryAfter > 0 {
		fmt.Fprintf(&b, " retry_after=%s", trace.RetryAfter)
	}
	if trace.ErrorClass != "" {
		fmt.Fprintf(&b, " err_class=%s", trace.ErrorClass)
	}
	if trace.CacheReadTokens > 0 || trace.CacheWriteTokens > 0 {
		fmt.Fprintf(&b, " cache_read=%d cache_write=%d", trace.CacheReadTokens, trace.CacheWriteTokens)
	}
	return b.String()
}

type requestInstrumentation struct {
	observer RequestObserver
	trace    RequestTrace
	started  time.Time
	finished bool

	headersRecorded    bool
	firstEventRecorded bool
	firstTokenRecorded bool
	terminalRecorded   bool
}

func newRequestInstrumentation(observer RequestObserver, model string) *requestInstrumentation {
	if observer == nil {
		return nil
	}
	now := time.Now()
	return &requestInstrumentation{
		observer: observer,
		started:  now,
		trace:    RequestTrace{Transport: "http", Model: model, StartedAt: now},
	}
}

func (ri *requestInstrumentation) setRequestShape(bytes, items int) {
	if ri == nil {
		return
	}
	ri.trace.RequestBytes = bytes
	ri.trace.InputItems = items
}

func (ri *requestInstrumentation) recordHeaders(status int) {
	if ri == nil || ri.headersRecorded {
		return
	}
	ri.headersRecorded = true
	ri.trace.HTTPStatus = status
	ri.trace.TimeToHeaders = instrumentationDurationSince(ri.started)
}

func (ri *requestInstrumentation) recordFirstEvent() {
	if ri == nil || ri.firstEventRecorded {
		return
	}
	ri.firstEventRecorded = true
	ri.trace.TimeToFirstEvent = instrumentationDurationSince(ri.started)
}

func (ri *requestInstrumentation) recordFirstToken() {
	if ri == nil || ri.firstTokenRecorded {
		return
	}
	ri.firstTokenRecorded = true
	ri.trace.TimeToFirstToken = instrumentationDurationSince(ri.started)
}

func (ri *requestInstrumentation) recordTerminal() {
	if ri == nil || ri.terminalRecorded {
		return
	}
	ri.terminalRecorded = true
	ri.trace.TimeToTerminal = instrumentationDurationSince(ri.started)
}

func (ri *requestInstrumentation) recordTerminalFailure(classification string) {
	if ri == nil {
		return
	}
	ri.recordTerminal()
	ri.trace.ErrorClassification = classification
	ri.trace.ErrorClass = "transport"
}

func (ri *requestInstrumentation) recordUsage(usage core.Usage) {
	if ri == nil {
		return
	}
	ri.trace.CacheReadTokens = usage.CacheReadTokens
	ri.trace.CacheWriteTokens = usage.CacheWriteTokens
}

func (ri *requestInstrumentation) classifyHTTPResponse(status int, classification string) {
	if ri == nil {
		return
	}
	ri.trace.HTTPStatus = status
	ri.trace.ErrorClassification = classification
}

func (ri *requestInstrumentation) recordError(err error) {
	if ri == nil || err == nil {
		return
	}
	ri.trace.ErrorClass = anthropicTraceErrorClass(err)
	var httpErr *core.ModelHTTPError
	if errors.As(err, &httpErr) {
		if ri.trace.HTTPStatus == 0 {
			ri.trace.HTTPStatus = httpErr.StatusCode
		}
		if httpErr.Body != "" {
			ri.trace.ErrorClassification = httpErr.Body
		}
		ri.trace.RetryAfter = httpErr.RetryAfter
	}
}

func (ri *requestInstrumentation) finish() {
	if ri == nil || ri.finished {
		return
	}
	ri.finished = true
	ri.trace.TotalDuration = instrumentationDurationSince(ri.started)
	ri.observer(ri.trace)
}

func instrumentationDurationSince(start time.Time) time.Duration {
	if elapsed := time.Since(start); elapsed > 0 {
		return elapsed
	}
	return time.Nanosecond
}

func anthropicTraceErrorClass(err error) string {
	var httpErr *core.ModelHTTPError
	if errors.As(err, &httpErr) {
		return "http"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "context"
	}
	return "transport"
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
