package codetool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// FetchURLFunc fetches a URL and returns extracted text content.
type FetchURLFunc func(ctx context.Context, rawURL string) (string, error)

// FetchURLParams are the parameters for the fetch_url tool.
type FetchURLParams struct {
	URL string `json:"url" jsonschema:"description=The URL to fetch (must be http:// or https://)"`
}

const maxFetchResponseBytes = 100 * 1024

var lookupHost = net.LookupHost

// ValidateFetchURLSafety rejects unsafe URLs before fetching them.
func ValidateFetchURLSafety(rawURL string) error {
	return validateURLSafety(rawURL)
}

// FetchURL creates a tool that fetches web page content.
func FetchURL(fetchFn FetchURLFunc) core.Tool {
	if fetchFn == nil {
		return core.Tool{}
	}
	return core.FuncTool[FetchURLParams](
		"fetch_url",
		"Fetch and read content from a web URL. Returns extracted text. Only use URLs provided by the user or found in local files. Rejects localhost and private network URLs.",
		func(ctx context.Context, params FetchURLParams) (string, error) {
			if strings.TrimSpace(params.URL) == "" {
				return "", &core.ModelRetryError{Message: "url must not be empty"}
			}
			if err := validateURLSafety(params.URL); err != nil {
				return "", &core.ModelRetryError{Message: err.Error()}
			}
			content, err := fetchFn(ctx, params.URL)
			if err != nil {
				return "", fmt.Errorf("fetch failed: %w", err)
			}
			if len(content) > maxFetchResponseBytes {
				content = content[:maxFetchResponseBytes] + fmt.Sprintf("\n\n... [truncated at %d bytes]", maxFetchResponseBytes)
			}
			return content, nil
		},
	)
}

func validateURLSafety(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q: only http and https are allowed", u.Scheme)
	}
	hostname := u.Hostname()
	lower := strings.ToLower(hostname)
	if lower == "" {
		return errors.New("url must include a hostname")
	}
	if lower == "localhost" || strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") || lower == "metadata.google.internal" {
		return fmt.Errorf("rejected local/private hostname: %s", hostname)
	}
	if ip := net.ParseIP(hostname); ip != nil && isPrivateIP(ip) {
		return fmt.Errorf("rejected private/loopback IP: %s", hostname)
	}
	ips, err := lookupHost(hostname)
	if err == nil {
		for _, ipStr := range ips {
			if ip := net.ParseIP(ipStr); ip != nil && isPrivateIP(ip) {
				return fmt.Errorf("hostname %s resolves to private IP %s", hostname, ipStr)
			}
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsUnspecified()
}
