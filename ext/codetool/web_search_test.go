package codetool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWebSearchFormatsResults(t *testing.T) {
	tool := WebSearch(func(_ context.Context, query string, numResults int) ([]WebSearchResult, error) {
		if query != "golang io reader" || numResults != 3 {
			t.Fatalf("query=%q numResults=%d", query, numResults)
		}
		return []WebSearchResult{{Title: "io.Reader", URL: "https://pkg.go.dev/io", Snippet: "Reader docs"}}, nil
	})
	n := 3
	got := call(t, tool, `{"query":"golang io reader","num_results":3}`)
	if !strings.Contains(got, "io.Reader") || !strings.Contains(got, "pkg.go.dev/io") || !strings.Contains(got, "Reader docs") {
		t.Fatalf("unexpected output: %s", got)
	}
	_ = n
}

func TestWebSearchEmptyQueryRetry(t *testing.T) {
	tool := WebSearch(func(_ context.Context, _ string, _ int) ([]WebSearchResult, error) { return nil, nil })
	if err := callErr(t, tool, `{"query":"   "}`); err == nil {
		t.Fatal("expected retry error")
	}
}

func TestWebSearchNoResults(t *testing.T) {
	tool := WebSearch(func(_ context.Context, _ string, _ int) ([]WebSearchResult, error) { return nil, nil })
	if got := call(t, tool, `{"query":"nothing"}`); got != "No results found." {
		t.Fatalf("got %q", got)
	}
}

func TestWebSearchClampsNumResults(t *testing.T) {
	tool := WebSearch(func(_ context.Context, _ string, numResults int) ([]WebSearchResult, error) {
		if numResults != 20 {
			t.Fatalf("numResults=%d, want 20", numResults)
		}
		return nil, nil
	})
	_ = call(t, tool, `{"query":"q","num_results":99}`)
}

func TestWebSearchPropagatesError(t *testing.T) {
	tool := WebSearch(func(_ context.Context, _ string, _ int) ([]WebSearchResult, error) { return nil, errors.New("boom") })
	if err := callErr(t, tool, `{"query":"q"}`); err == nil || !strings.Contains(err.Error(), "web search failed") {
		t.Fatalf("err=%v", err)
	}
}
