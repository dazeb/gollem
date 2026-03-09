package codetool

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// WebSearchResult is a single search result.
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// WebSearchFunc performs a web search and returns results.
// The application provides the concrete implementation (Google, Brave, Exa, etc.).
type WebSearchFunc func(ctx context.Context, query string, numResults int) ([]WebSearchResult, error)

// WebSearchParams are the parameters for the web_search tool.
type WebSearchParams struct {
	Query      string `json:"query" jsonschema:"description=Search query string"`
	NumResults *int   `json:"num_results,omitempty" jsonschema:"description=Number of results to return (default 5\\, max 20)"`
}

const (
	defaultSearchResults = 5
	maxSearchResults     = 20
)

// WebSearch creates a tool that searches the web.
func WebSearch(searchFn WebSearchFunc) core.Tool {
	if searchFn == nil {
		return core.Tool{}
	}
	return core.FuncTool[WebSearchParams](
		"web_search",
		"Search the web for documentation, error solutions, API references, or current information. "+
			"Returns titles, URLs, and snippets. Use specific, targeted queries.",
		func(ctx context.Context, params WebSearchParams) (string, error) {
			if strings.TrimSpace(params.Query) == "" {
				return "", &core.ModelRetryError{Message: "query must not be empty"}
			}
			n := defaultSearchResults
			if params.NumResults != nil && *params.NumResults > 0 {
				n = min(*params.NumResults, maxSearchResults)
			}

			results, err := searchFn(ctx, params.Query, n)
			if err != nil {
				return "", fmt.Errorf("web search failed: %w", err)
			}
			if len(results) == 0 {
				return "No results found.", nil
			}

			var b strings.Builder
			for i, r := range results {
				fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.Title, r.URL)
				if r.Snippet != "" {
					fmt.Fprintf(&b, "   %s\n", r.Snippet)
				}
				b.WriteString("\n")
			}
			return b.String(), nil
		},
	)
}
