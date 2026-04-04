package mcp

import "net/http"

// RemoteOption configures remote MCP transports such as SSE and streamable HTTP.
type RemoteOption func(*remoteConfig)

// SSEOption configures the SSE client.
type SSEOption = RemoteOption

// HTTPClientOption configures the streamable HTTP client.
type HTTPClientOption = RemoteOption

type remoteConfig struct {
	httpClient *http.Client
	headers    map[string]string
}

func defaultRemoteConfig() remoteConfig {
	return remoteConfig{
		httpClient: http.DefaultClient,
	}
}

// WithHTTPClient sets a custom HTTP client for remote transports.
func WithHTTPClient(client *http.Client) RemoteOption {
	return func(c *remoteConfig) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithHeaders adds HTTP headers to remote MCP requests.
func WithHeaders(headers map[string]string) RemoteOption {
	cloned := cloneStringMap(headers)
	return func(c *remoteConfig) {
		if c.headers == nil {
			c.headers = make(map[string]string, len(cloned))
		}
		for k, v := range cloned {
			c.headers[k] = v
		}
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func applyHeaders(req *http.Request, headers map[string]string) {
	for key, value := range headers {
		req.Header.Set(key, value)
	}
}

func applyProtocolVersionHeader(req *http.Request, version string) {
	if version == "" {
		version = protocolVersion
	}
	req.Header.Set("MCP-Protocol-Version", version)
}
