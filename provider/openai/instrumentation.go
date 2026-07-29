package openai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// RequestTrace is the secret-safe per-request latency record delivered to a
// RequestObserver. It measures timing phases and sizes only; it never carries
// access/refresh/ID tokens, account IDs, authorization URLs, callback queries,
// device codes, request content, or raw provider error bodies.
//
// All durations are measured from StartedAt. Fields are zero when the
// corresponding phase did not occur (for example TimeToFirstToken is zero for a
// non-streaming request that the caller never drained incrementally).
type RequestTrace struct {
	// Transport is "http" or "websocket".
	Transport string
	// Model is the model the provider was configured to request.
	Model string
	// StartedAt is when the physical provider request began (before token
	// refresh, header construction, and the network round-trip).
	StartedAt time.Time
	// TotalDuration spans StartedAt through response finalization (or the
	// returned error). It therefore includes any refresh, upload, backend
	// queueing/reasoning, streaming, and retry/backoff time spent inside this
	// single provider call. Retry gaps between separate provider calls appear
	// as separate RequestTrace records.
	TotalDuration time.Duration

	// RequestBytes is the marshaled request body size in bytes. Input content
	// is not included.
	RequestBytes int
	// InputItems is the count of top-level input items sent to the provider.
	InputItems int

	// TokenRefreshInvoked reports whether the configured TokenRefresher was
	// called for this request.
	TokenRefreshInvoked bool
	// TokenRefreshDuration is how long the refresher took. A non-trivial value
	// combined with TokenRefreshInvoked indicates network I/O occurred; a value
	// near zero typically indicates the refresh was elided because the token
	// was still valid.
	TokenRefreshDuration time.Duration

	// TimeToHeaders is the time from StartedAt until response headers arrived
	// (HTTP) or the WebSocket dial+handshake completed. Zero if the request
	// failed before headers.
	TimeToHeaders time.Duration
	// TimeToFirstEvent is the time until the first SSE/WebSocket event was
	// observed. Zero if no event arrived.
	TimeToFirstEvent time.Duration
	// TimeToFirstToken is the time until the first text or tool delta was
	// produced. Zero if it never occurred (for example a request that errored
	// before any content).
	TimeToFirstToken time.Duration
	// TimeToTerminal is the time until the terminal response event. Zero if the
	// request failed before a terminal event.
	TimeToTerminal time.Duration

	// HTTPStatus is the HTTP status code (HTTP transport) or the inferred
	// websocket error status. Zero when unknown.
	HTTPStatus int
	// ErrorClassification is the sanitized provider error marker (for example
	// "rate_limited", "previous_response_not_found") produced by
	// classifyProviderError, or "" on success. It never contains raw provider
	// bodies.
	ErrorClassification string
	// RetryAfter is the Retry-After duration parsed from a 429 response, if any.
	RetryAfter time.Duration
	// ErrorClass is a coarse typed-error category ("", "http", "identity",
	// "transport", "context") on failure. It never contains err.Error().
	ErrorClass string

	// WebSocketConnectionReused reports whether an existing WebSocket
	// connection was reused rather than dialing a new one. HTTP transport sets
	// this to false.
	WebSocketConnectionReused bool
	// PreviousResponseIDReused reports whether the request was sent as a
	// continuation on top of a previous response id (Responses API).
	PreviousResponseIDReused bool
	// PromptCacheKeyActive reports whether a prompt cache key was applied to
	// the request.
	PromptCacheKeyActive bool
	// PromptCacheKeyFingerprint is a short, non-reversible fingerprint
	// (sha256 prefix) of the prompt cache key in use, or "" if none. It lets a
	// downstream aggregator verify cross-request and cross-instance cache
	// continuity without ever exposing the key itself.
	PromptCacheKeyFingerprint string
}

// RequestObserver receives one RequestTrace per physical provider request. It
// is invoked from the request goroutine; nil means no instrumentation (fully
// no-op, zero overhead). The implementation must be safe to call from the
// provider's request goroutine.
type RequestObserver func(trace RequestTrace)

// WithRequestObserver installs a callback that receives a secret-safe
// per-request latency trace. The trace covers token refresh, request upload,
// time to headers/first-event/first-token/terminal, sanitized error
// classification, and continuation/cache-continuity signals — but never
// credentials, request content, or raw provider error bodies.
//
// It fires for both the HTTP and WebSocket transports. When unset, the
// provider incurs no instrumentation overhead.
func WithRequestObserver(obs RequestObserver) Option {
	return func(p *Provider) {
		p.requestObserver = obs
	}
}

// DefaultStderrRequestObserver returns a RequestObserver that prints a tagged,
// human-readable summary of each trace to os.Stderr.
func DefaultStderrRequestObserver() RequestObserver {
	return func(t RequestTrace) {
		fmt.Fprintln(os.Stderr, formatRequestTrace(t))
	}
}

// formatRequestTrace renders a single trace as a compact, grep-friendly line
// block. Exported via DefaultStderrRequestObserver; kept here for testability.
func formatRequestTrace(t RequestTrace) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[gollem-openai-trace] transport=%s model=%s started=%s",
		t.Transport, t.Model, t.StartedAt.Format(time.RFC3339Nano))
	if t.RequestBytes > 0 || t.InputItems > 0 {
		fmt.Fprintf(&b, " req_bytes=%d input_items=%d", t.RequestBytes, t.InputItems)
	}
	fmt.Fprintf(&b, " total=%s", t.TotalDuration)
	if t.TokenRefreshInvoked {
		fmt.Fprintf(&b, " refresh=%s", t.TokenRefreshDuration)
	}
	if t.TimeToHeaders > 0 {
		fmt.Fprintf(&b, " ttfb=%s", t.TimeToHeaders)
	}
	if t.TimeToFirstEvent > 0 {
		fmt.Fprintf(&b, " first_event=%s", t.TimeToFirstEvent)
	}
	if t.TimeToFirstToken > 0 {
		fmt.Fprintf(&b, " first_token=%s", t.TimeToFirstToken)
	}
	if t.TimeToTerminal > 0 {
		fmt.Fprintf(&b, " terminal=%s", t.TimeToTerminal)
	}
	if t.HTTPStatus != 0 {
		fmt.Fprintf(&b, " status=%d", t.HTTPStatus)
	}
	if t.ErrorClassification != "" {
		fmt.Fprintf(&b, " err=%s", t.ErrorClassification)
	}
	if t.RetryAfter > 0 {
		fmt.Fprintf(&b, " retry_after=%s", t.RetryAfter)
	}
	if t.ErrorClass != "" {
		fmt.Fprintf(&b, " err_class=%s", t.ErrorClass)
	}
	if t.WebSocketConnectionReused {
		fmt.Fprintf(&b, " ws_reused=true")
	}
	if t.PreviousResponseIDReused {
		fmt.Fprintf(&b, " prev_response_reused=true")
	}
	if t.PromptCacheKeyActive {
		fmt.Fprintf(&b, " cache_key=%s", t.PromptCacheKeyFingerprint)
	}
	return b.String()
}

// requestInstrumentation accumulates timing for a single physical provider
// request. A nil pointer is a no-op everywhere it is consulted, so unobserved
// requests incur zero allocation and zero clock reads on the hot path.
//
// It is per-request (created at the top of Request/RequestStream) rather than
// a shared provider field, so concurrent requests do not race on it.
type requestInstrumentation struct {
	obs      RequestObserver
	trace    RequestTrace
	start    time.Time
	tokenSt  time.Time
	finished bool

	headersRecorded    bool
	firstEventRecorded bool
	firstTokenRecorded bool
	terminalRecorded   bool
}

// newRequestInstrumentation returns a ready accumulator, or nil when there is
// no observer. Callers should keep the nil result rather than wrapping it.
func newRequestInstrumentation(obs RequestObserver, transport, model string) *requestInstrumentation {
	if obs == nil {
		return nil
	}
	now := time.Now()
	return &requestInstrumentation{
		obs:   obs,
		start: now,
		trace: RequestTrace{
			Transport: transport,
			Model:     model,
			StartedAt: now,
		},
	}
}

func (ri *requestInstrumentation) setRequestShape(bytes int, items int) {
	if ri == nil {
		return
	}
	ri.trace.RequestBytes = bytes
	ri.trace.InputItems = items
}

// setTransport overrides the recorded physical transport. Used by
// requestStreamViaResponses, which always uses HTTP SSE even when the provider
// is configured for websocket (the websocket transport exposes no streaming
// interface), so such traces are correctly labeled "http".
func (ri *requestInstrumentation) setTransport(transport string) {
	if ri == nil {
		return
	}
	ri.trace.Transport = transport
}

func (ri *requestInstrumentation) beginTokenRefresh() {
	if ri == nil {
		return
	}
	ri.trace.TokenRefreshInvoked = true
	ri.tokenSt = time.Now()
}

func (ri *requestInstrumentation) endTokenRefresh() {
	if ri == nil || ri.tokenSt.IsZero() {
		return
	}
	ri.trace.TokenRefreshDuration = time.Since(ri.tokenSt)
}

func (ri *requestInstrumentation) recordHeaders(status int) {
	if ri == nil || ri.headersRecorded {
		return
	}
	ri.headersRecorded = true
	ri.trace.TimeToHeaders = time.Since(ri.start)
	ri.trace.HTTPStatus = status
}

func (ri *requestInstrumentation) recordFirstEvent() {
	if ri == nil || ri.firstEventRecorded {
		return
	}
	ri.firstEventRecorded = true
	ri.trace.TimeToFirstEvent = time.Since(ri.start)
}

func (ri *requestInstrumentation) recordFirstToken() {
	if ri == nil || ri.firstTokenRecorded {
		return
	}
	ri.firstTokenRecorded = true
	ri.trace.TimeToFirstToken = time.Since(ri.start)
}

func (ri *requestInstrumentation) recordTerminal() {
	if ri == nil || ri.terminalRecorded {
		return
	}
	ri.terminalRecorded = true
	ri.trace.TimeToTerminal = time.Since(ri.start)
}

// recordTerminalFailure records the terminal timing for an in-band Responses
// failure/incomplete outcome together with its sanitized classification. The
// model argument carries the sanitized provider marker. It is the terminal
// counterpart of recordError for outcomes that the backend signals as a
// definitive event rather than a transport error.
func (ri *requestInstrumentation) recordTerminalFailure(classification string) {
	if ri == nil {
		return
	}
	ri.recordTerminal()
	if classification != "" {
		ri.trace.ErrorClassification = classification
	}
	if ri.trace.ErrorClass == "" {
		ri.trace.ErrorClass = "transport"
	}
}

// resetForRetry clears the transient phase markers and any error from a failed
// attempt so a reconnect-retry produces a trace that reflects the retrying
// physical request (whose outcome may succeed). TotalDuration and StartedAt
// are preserved so the trace still spans the full caller-visible request,
// including the retry gap; the WebSocketConnectionReused flag is cleared
// because the retry dials a fresh connection.
func (ri *requestInstrumentation) resetForRetry() {
	if ri == nil {
		return
	}
	ri.headersRecorded = false
	ri.firstEventRecorded = false
	ri.firstTokenRecorded = false
	ri.terminalRecorded = false
	ri.trace.TimeToHeaders = 0
	ri.trace.TimeToFirstEvent = 0
	ri.trace.TimeToFirstToken = 0
	ri.trace.TimeToTerminal = 0
	ri.trace.HTTPStatus = 0
	ri.trace.ErrorClassification = ""
	ri.trace.ErrorClass = ""
	ri.trace.RetryAfter = 0
	ri.trace.WebSocketConnectionReused = false
}

func (ri *requestInstrumentation) markWebSocketReused(reused bool) {
	if ri == nil {
		return
	}
	ri.trace.WebSocketConnectionReused = reused
}

func (ri *requestInstrumentation) markPreviousResponseIDReused(reused bool) {
	if ri == nil {
		return
	}
	ri.trace.PreviousResponseIDReused = reused
}

func (ri *requestInstrumentation) markCacheKey(key string) {
	if ri == nil {
		return
	}
	if key == "" {
		return
	}
	ri.trace.PromptCacheKeyActive = true
	ri.trace.PromptCacheKeyFingerprint = cacheKeyFingerprint(key)
}

func (ri *requestInstrumentation) recordRetryAfter(d time.Duration) {
	if ri == nil {
		return
	}
	ri.trace.RetryAfter = d
}

// recordErrorClassifies sets the sanitized error fields. The raw error is never
// stored or serialized; only coarse categories derived from its type plus the
// classification already attached to a ModelHTTPError.
func (ri *requestInstrumentation) recordError(err error) {
	if ri == nil || err == nil {
		return
	}
	ri.trace.ErrorClass = errorClass(err)
	var httpErr *core.ModelHTTPError
	if errors.As(err, &httpErr) {
		// Body already holds the sanitized classification marker.
		ri.trace.ErrorClassification = httpErr.Body
		if ri.trace.HTTPStatus == 0 {
			ri.trace.HTTPStatus = httpErr.StatusCode
		}
		if httpErr.RetryAfter > 0 {
			ri.trace.RetryAfter = httpErr.RetryAfter
		}
	}
}

// finish delivers the trace to the observer exactly once.
func (ri *requestInstrumentation) finish() {
	if ri == nil || ri.finished {
		return
	}
	ri.finished = true
	ri.trace.TotalDuration = time.Since(ri.start)
	ri.obs(ri.trace)
}

// errorClass returns a coarse, credential-free category for an error.
func errorClass(err error) string {
	if err == nil {
		return ""
	}
	var httpErr *core.ModelHTTPError
	if errors.As(err, &httpErr) {
		return "http"
	}
	var identityErr *ModelIdentityError
	if errors.As(err, &identityErr) {
		return "identity"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "context"
	}
	return "transport"
}

// cacheKeyFingerprint returns a short, non-reversible sha256 prefix for a
// prompt cache key. The full key is never exposed through instrumentation.
func cacheKeyFingerprint(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:8]
}

// classifyHTTPResponse records the sanitized outcome of an HTTP round-trip from
// the response status and the already-classified error body. It is shared by
// the HTTP and WebSocket paths.
func (ri *requestInstrumentation) classifyHTTPResponse(status int, classification string) {
	if ri == nil {
		return
	}
	ri.trace.HTTPStatus = status
	if classification != "" {
		ri.trace.ErrorClassification = classification
	}
}
