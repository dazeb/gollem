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
	"flag"
	"fmt"
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

	for _, transport := range transports {
		for _, n := range turns {
			if err := runOnce(ctx, creds, model, transport, n); err != nil {
				fmt.Printf("# %s turns=%d ERROR: %v\n", transport, n, err)
			}
		}
	}
}

func runOnce(ctx context.Context, creds *openai.Credentials, model, transport string, historyTurns int) error {
	var (
		mu    sync.Mutex
		trace oai.RequestTrace
	)
	obs := func(t oai.RequestTrace) {
		mu.Lock()
		trace = t
		mu.Unlock()
	}

	// tokenRefresher mirrors how a downstream ChatGPT-auth consumer wires the
	// OAuth credential lifecycle into the provider: refresh if near expiry, then
	// persist the rotated credentials atomically.
	tokenRefresher := func() (string, error) {
		refreshed, err := openai.RefreshIfNeeded(creds)
		if err != nil {
			return "", err
		}
		if refreshed != creds {
			creds = refreshed
		}
		return creds.AccessToken, nil
	}

	opts := []oai.Option{
		oai.WithChatGPTAuth(creds.AccessToken, creds.AccountID),
		oai.WithModel(model),
		oai.WithTokenRefresher(tokenRefresher),
		oai.WithRequestObserver(obs),
	}
	if transport == "websocket" {
		opts = append(opts, oai.WithTransport("websocket"))
	}

	provider := oai.New(opts...)
	defer provider.Close()

	msgs := buildHistory(historyTurns)

	switch transport {
	case "http", "websocket":
		resp, err := provider.Request(ctx, msgs, nil, nil)
		if err != nil {
			return err
		}
		_ = resp
	case "stream":
		stream, err := provider.RequestStream(ctx, msgs, nil, nil)
		if err != nil {
			return err
		}
		for {
			if _, err := stream.Next(); err != nil {
				break
			}
		}
		_ = stream.Close()
	default:
		return fmt.Errorf("unknown transport %q", transport)
	}

	mu.Lock()
	defer mu.Unlock()
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
	return nil
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
