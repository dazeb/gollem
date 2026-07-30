// Example openai-latency is a reproducible harness for investigating latency in
// ChatGPT OAuth-backed OpenAI requests (issue #242).
//
// It loads ChatGPT subscription credentials (auth/openai), builds one provider
// per transport with WithChatGPTAuth + WithTokenRefresher(RefreshIfNeeded) +
// WithRequestObserver, runs a fixed prompt across a growing conversation
// history, and prints a secret-safe per-request phase-breakdown table.
//
// Usage:
//
//	# Log in once to populate ~/.golem/auth.json (chatgpt subscription path):
//	#   (use your own ChatGPT Plus/Pro/Team account)
//
//	OPENAI_MODEL=gpt-5.3-codex go run ./examples/openai-latency
//
// Environment:
//
//	OPENAI_MODEL            model to request (default gpt-5.3-codex)
//	OPENAI_AUTH_PATH        path to auth.json (default ~/.golem/auth.json)
//	OPENAI_TRANSPORTS       comma list: http,websocket,stream (default http,stream,websocket)
//	OPENAI_HISTORY_TURNS    comma list of history sizes (default 1,5,15)
//
// No access/refresh/ID tokens, account IDs, authorization URLs, or raw provider
// error bodies are ever printed — only sizes, timings, and sanitized markers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/auth/openai"
	"github.com/fugue-labs/gollem/core"
	oai "github.com/fugue-labs/gollem/provider/openai"
)

const defaultModel = "gpt-5.3-codex"

func main() {
	authPath := envOr("OPENAI_AUTH_PATH", "")
	model := envOr("OPENAI_MODEL", defaultModel)
	transports := splitCSV(envOr("OPENAI_TRANSPORTS", "http,stream,websocket"))
	turns := parseIntCSV(envOr("OPENAI_HISTORY_TURNS", "1,5,15"))
	flag.Parse()

	creds, err := loadCredentials(authPath)
	if err != nil {
		log.Fatalf("loading ChatGPT credentials: %v\n(log in first, or set OPENAI_AUTH_PATH)", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fmt.Printf("# ChatGPT OAuth latency investigation — model=%s account_claim=%s\n\n", model, redacted(creds.AccountID))
	fmt.Println("# Phases are measured from request start. No credentials are printed.")
	fmt.Println("# transport | turns | refresh | ttfb | first_event | first_token | terminal | total | ws_reused | prev_reused | cache | status | err")
	fmt.Println("# --------- | ----- | ------- | ---- | ----------- | ----------- | -------- | ----- | --------- | ----------- | ----- | ------ | ---")

	// credsHolder carries the live OAuth credentials across every run so that a
	// token rotation performed by one transport's refresher persists to the next
	// run (and the next transport). Without this each run would start from the
	// original credentials and re-pay refresh latency (or fail on a rotated
	// refresh token).
	credsHolder := &credentialHolder{creds: creds}

	for _, transport := range transports {
		// Build ONE provider per transport and reuse it across all history
		// sizes. This is essential to the experiment: only a reused provider
		// keeps its prompt-cache key, websocket connection, and continuation
		// state, so ws_reused / prev_reused / cache-continuity are measurable.
		mp, err := buildProvider(model, transport, credsHolder)
		if err != nil {
			fmt.Printf("# %s ERROR building provider: %v\n", transport, err)
			continue
		}
		for _, n := range turns {
			if err := runOnce(ctx, mp, transport, n); err != nil {
				fmt.Printf("# %s turns=%d ERROR: %v\n", transport, n, err)
			}
		}
		_ = mp.provider.Close()
	}
}

// credentialHolder is a tiny concurrency-safe holder for the live OAuth
// credentials shared across all transport runs.
type credentialHolder struct {
	mu    sync.Mutex
	creds *openai.Credentials
}

func (h *credentialHolder) get() *openai.Credentials {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.creds
}

func (h *credentialHolder) set(c *openai.Credentials) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.creds = c
}

// measuredProvider pairs a provider with a slot capturing its most recent trace
// so runOnce can print a row per request without a package global.
type measuredProvider struct {
	provider *oai.Provider
	mu       sync.Mutex
	trace    oai.RequestTrace
	hasTrace bool
}

func (m *measuredProvider) record(t oai.RequestTrace) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trace = t
	m.hasTrace = true
}

func (m *measuredProvider) last() (oai.RequestTrace, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.trace, m.hasTrace
}

func buildProvider(model, transport string, h *credentialHolder) (*measuredProvider, error) {
	creds := h.get()
	mp := &measuredProvider{}

	// tokenRefresher mirrors how a downstream ChatGPT-auth consumer wires the
	// OAuth credential lifecycle into the provider: refresh if near expiry, then
	// persist the rotated credentials back into the shared holder so every
	// subsequent run (any transport) sees them.
	tokenRefresher := func() (string, error) {
		current := h.get()
		refreshed, err := openai.RefreshIfNeeded(current)
		if err != nil {
			return "", err
		}
		if refreshed != current {
			h.set(refreshed)
		}
		return refreshed.AccessToken, nil
	}

	opts := []oai.Option{
		oai.WithChatGPTAuth(creds.AccessToken, creds.AccountID),
		oai.WithModel(model),
		oai.WithTokenRefresher(tokenRefresher),
		oai.WithRequestObserver(mp.record),
	}
	if transport == "websocket" {
		opts = append(opts, oai.WithTransport("websocket"))
	}

	mp.provider = oai.New(opts...)
	return mp, nil
}

func runOnce(ctx context.Context, mp *measuredProvider, transport string, historyTurns int) error {
	msgs := buildHistory(historyTurns)

	var runErr error
	switch transport {
	case "http", "websocket":
		resp, err := mp.provider.Request(ctx, msgs, nil, nil)
		if err != nil {
			runErr = err
		}
		_ = resp
	case "stream":
		stream, err := mp.provider.RequestStream(ctx, msgs, nil, nil)
		if err != nil {
			runErr = err
			break
		}
		// io.EOF is the normal terminal sentinel; any other error is a real
		// transport/provider failure and must be surfaced, not swallowed.
		for {
			if _, err := stream.Next(); err != nil {
				if !errors.Is(err, io.EOF) {
					runErr = err
				}
				break
			}
		}
		_ = stream.Close()
	default:
		return fmt.Errorf("unknown transport %q", transport)
	}

	trace, _ := mp.last()
	fmt.Printf("%-9s | %5d | %7s | %4s | %11s | %11s | %8s | %5s | %9t | %11t | %5s | %6d | %s\n",
		transport, historyTurns,
		trace.TokenRefreshDuration.Round(time.Millisecond),
		trace.TimeToHeaders.Round(time.Millisecond),
		trace.TimeToFirstEvent.Round(time.Millisecond),
		trace.TimeToFirstToken.Round(time.Millisecond),
		trace.TimeToTerminal.Round(time.Millisecond),
		trace.TotalDuration.Round(time.Millisecond),
		trace.WebSocketConnectionReused,
		trace.PreviousResponseIDReused,
		trace.PromptCacheKeyFingerprint,
		trace.HTTPStatus,
		trace.ErrorClassification,
	)
	return runErr
}

// buildHistory constructs a synthetic append-only conversation of the given
// number of turns. Each turn is a user prompt plus an assistant reply so the
// history grows deterministically.
func buildHistory(turns int) []core.ModelMessage {
	var msgs []core.ModelMessage
	for i := range turns {
		msgs = append(msgs, core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: fmt.Sprintf("Turn %d: briefly summarize what you did so far.", i+1)},
			},
		})
		msgs = append(msgs, core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.TextPart{Content: fmt.Sprintf("On turn %d I restated the goal and checked prior context.", i+1)},
			},
		})
	}
	msgs = append(msgs, core.ModelRequest{
		Parts: []core.ModelRequestPart{
			core.UserPromptPart{Content: "Reply with the single word: done"},
		},
	})
	return msgs
}

func loadCredentials(path string) (*openai.Credentials, error) {
	if path != "" {
		return openai.LoadCredentialsFrom(path)
	}
	return openai.LoadCredentials()
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseIntCSV(s string) []int {
	parts := splitCSV(s)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		var n int
		_, _ = fmt.Sscanf(p, "%d", &n)
		if n > 0 {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return []int{1}
	}
	return out
}

// redacted confirms an account was loaded without exposing it: prints the first
// two and last two characters with a mask in between.
func redacted(accountID string) string {
	if accountID == "" {
		return "<none>"
	}
	if len(accountID) <= 4 {
		return "****"
	}
	return accountID[:2] + "…" + accountID[len(accountID)-2:]
}
