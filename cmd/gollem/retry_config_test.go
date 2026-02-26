package main

import (
	"testing"
	"time"
)

func TestBuildRetryConfig_DefaultBuckets(t *testing.T) {
	cfg := buildRetryConfig("openai", "gpt-5.2-codex", 4*time.Minute)
	if cfg.MaxRetries != 1 {
		t.Fatalf("short timeout max retries = %d, want 1", cfg.MaxRetries)
	}

	cfg = buildRetryConfig("openai", "gpt-5.2-codex", 10*time.Minute)
	if cfg.MaxRetries != 2 {
		t.Fatalf("mid timeout max retries = %d, want 2", cfg.MaxRetries)
	}

	cfg = buildRetryConfig("openai", "gpt-5.2-codex", 30*time.Minute)
	if cfg.MaxRetries != 3 {
		t.Fatalf("long timeout max retries = %d, want 3", cfg.MaxRetries)
	}
}

func TestBuildRetryConfig_GeminiVertexBoost(t *testing.T) {
	cfg := buildRetryConfig("vertexai", "gemini-3.1-pro-preview", 30*time.Minute)
	if cfg.MaxRetries != 6 {
		t.Fatalf("gemini long timeout max retries = %d, want 6", cfg.MaxRetries)
	}
	if cfg.MaxBackoff != 12*time.Second {
		t.Fatalf("gemini max backoff = %v, want 12s", cfg.MaxBackoff)
	}
}

func TestBuildRetryConfig_NonGeminiVertexNoBoost(t *testing.T) {
	cfg := buildRetryConfig("vertexai", "text-bison", 30*time.Minute)
	if cfg.MaxRetries != 3 {
		t.Fatalf("non-gemini vertex max retries = %d, want 3", cfg.MaxRetries)
	}
}

func TestDeriveGeminiMaxOutputTokens(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("GOLLEM_GEMINI_MAX_OUTPUT_TOKENS", "")
		if got := deriveGeminiMaxOutputTokens(); got != 12000 {
			t.Fatalf("default max output = %d, want 12000", got)
		}
	})

	t.Run("override", func(t *testing.T) {
		t.Setenv("GOLLEM_GEMINI_MAX_OUTPUT_TOKENS", "8000")
		if got := deriveGeminiMaxOutputTokens(); got != 8000 {
			t.Fatalf("override max output = %d, want 8000", got)
		}
	})

	t.Run("clamp low", func(t *testing.T) {
		t.Setenv("GOLLEM_GEMINI_MAX_OUTPUT_TOKENS", "100")
		if got := deriveGeminiMaxOutputTokens(); got != 1024 {
			t.Fatalf("low clamp max output = %d, want 1024", got)
		}
	})

	t.Run("clamp high", func(t *testing.T) {
		t.Setenv("GOLLEM_GEMINI_MAX_OUTPUT_TOKENS", "999999")
		if got := deriveGeminiMaxOutputTokens(); got != 40000 {
			t.Fatalf("high clamp max output = %d, want 40000", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Setenv("GOLLEM_GEMINI_MAX_OUTPUT_TOKENS", "nope")
		if got := deriveGeminiMaxOutputTokens(); got != 12000 {
			t.Fatalf("invalid fallback max output = %d, want 12000", got)
		}
	})
}
