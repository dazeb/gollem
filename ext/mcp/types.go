package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// ProtocolVersion is the MCP revision implemented by this package.
	ProtocolVersion = "2026-07-28"
	protocolVersion = ProtocolVersion
	clientName      = "gollem"
	clientVersion   = "1.0.0"
)

// SupportedProtocolVersions lists every protocol revision this implementation
// accepts. The 2026-07-28 spec is a hard cut: only this revision is supported.
var SupportedProtocolVersions = []string{ProtocolVersion}

// Reserved _meta keys defined by the 2026-07-28 specification. Every request
// carries its protocol version and client capabilities inline (there is no
// initialize handshake), and results echo server identity.
const (
	MetaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	MetaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	MetaClientInfo         = "io.modelcontextprotocol/clientInfo"
	MetaServerInfo         = "io.modelcontextprotocol/serverInfo"
	MetaLogLevel           = "io.modelcontextprotocol/logLevel"
	MetaSubscriptionID     = "io.modelcontextprotocol/subscriptionId"

	// OpenTelemetry trace-context propagation keys (SEP-414).
	MetaTraceParent = "traceparent"
	MetaTraceState  = "tracestate"
	MetaBaggage     = "baggage"
)

// Extension identifiers negotiated via the extensions capability map.
const (
	ExtensionTasks = "io.modelcontextprotocol/tasks"
)

// ResultType discriminates ordinary results from multi round-trip interim
// results. Results from earlier-protocol servers that omit it are "complete".
const (
	ResultTypeComplete      = "complete"
	ResultTypeInputRequired = "input_required"
)

// CacheScope controls whether shared intermediaries may cache a result.
const (
	CacheScopePublic  = "public"
	CacheScopePrivate = "private"
)

// EmptyCapability is used for presence-only MCP capabilities.
type EmptyCapability struct{}

// ClientCapabilities describes the protocol surfaces exposed by an MCP client.
//
// Roots and Sampling are deprecated in 2026-07-28 (SEP-2577); new clients
// should not advertise them. Elicitation is now delivered via Multi Round-Trip
// Requests (MRTR) rather than a server-initiated request.
type ClientCapabilities struct {
	Roots        *RootsCapability          `json:"roots,omitempty"`
	Sampling     *ClientSamplingCapability `json:"sampling,omitempty"`
	Elicitation  *ElicitationCapability    `json:"elicitation,omitempty"`
	Experimental map[string]map[string]any `json:"experimental,omitempty"`
	// Extensions negotiates optional MCP extensions beyond the core protocol.
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// ClientSamplingCapability describes client-side sampling support.
type ClientSamplingCapability struct {
	Context *EmptyCapability `json:"context,omitempty"`
	Tools   *EmptyCapability `json:"tools,omitempty"`
}

// ElicitationCapability describes client-side elicitation support.
type ElicitationCapability struct {
	Form *EmptyCapability `json:"form,omitempty"`
	URL  *EmptyCapability `json:"url,omitempty"`
}

// RootsCapability describes client-side root listing support.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerCapabilities describes the protocol surfaces exposed by the server.
type ServerCapabilities struct {
	Tools        *ToolCapabilities     `json:"tools,omitempty"`
	Prompts      *PromptCapabilities   `json:"prompts,omitempty"`
	Resources    *ResourceCapabilities `json:"resources,omitempty"`
	Experimental map[string]any        `json:"experimental,omitempty"`
	// Extensions advertises optional MCP extensions supported by the server.
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// ToolCapabilities describes the server's tool support.
type ToolCapabilities struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptCapabilities describes the server's prompt support.
type PromptCapabilities struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourceCapabilities describes the server's resource support.
type ResourceCapabilities struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerInfo identifies the connected MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ImplementationInfo identifies an MCP implementation.
type ImplementationInfo = ServerInfo

// InitializeParams is sent by the client during initialize.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ImplementationInfo `json:"clientInfo"`
}

// InitializeResult is retained only as the payload shape shared with
// server/discover. The 2026-07-28 protocol removes the initialize handshake;
// clients learn server identity and capabilities from DiscoverResult or from
// the serverInfo echoed in each result's _meta.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      *ServerInfo        `json:"serverInfo,omitempty"`
	Instructions    string             `json:"instructions,omitempty"`
	Meta            map[string]any     `json:"_meta,omitempty"`
}

// DiscoverResult is returned by the required server/discover RPC. Clients MAY
// call it before any other request to select a protocol version up front, or
// use it as a backward-compatibility probe on STDIO.
type DiscoverResult struct {
	SupportedVersions []string           `json:"supportedVersions"`
	Capabilities      ServerCapabilities `json:"capabilities"`
	ServerInfo        *ServerInfo        `json:"serverInfo,omitempty"`
	Instructions      string             `json:"instructions,omitempty"`
	Meta              map[string]any     `json:"_meta,omitempty"`
	// CacheableResult carries the ttlMs/cacheScope hints; server/discover
	// supports caching per the specification.
	CacheableResult
}

// RequestMeta is the reserved _meta envelope carried by every stateless
// request. It replaces the initialize handshake: each request self-describes
// its protocol version, the client's capabilities, and the client's identity.
type RequestMeta struct {
	ProtocolVersion    string              `json:"io.modelcontextprotocol/protocolVersion,omitempty"`
	ClientCapabilities *ClientCapabilities `json:"io.modelcontextprotocol/clientCapabilities,omitempty"`
	ClientInfo         *ImplementationInfo `json:"io.modelcontextprotocol/clientInfo,omitempty"`
	LogLevel           string              `json:"io.modelcontextprotocol/logLevel,omitempty"`
}

// CacheableResult carries the freshness hints required on list and read
// results (SEP-2549). TTLMs is a caching hint in milliseconds; CacheScope is
// "public" or "private". Both are embedded into the wire result.
type CacheableResult struct {
	TTLMs      *int64 `json:"ttlMs,omitempty"`
	CacheScope string `json:"cacheScope,omitempty"`
}

// InputRequest is a single server-to-client request embedded in an
// InputRequiredResult. Method is one of "elicitation/create",
// "sampling/createMessage", or "roots/list"; Params is the corresponding
// request payload. It replaces the previous server-initiated request pattern.
type InputRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// InputRequests maps server-assigned identifiers to the requests a client must
// fulfill before retrying. Identifiers are unique within a single request.
type InputRequests map[string]InputRequest

// InputResponses maps the same identifiers to the client's results
// (ElicitResult, CreateMessageResult, or ListRootsResult), sent on retry.
type InputResponses map[string]json.RawMessage

// InputRequiredResult is returned with resultType "input_required" when a
// server needs additional information mid-request. The client fulfills each
// entry in InputRequests and retries the original call (with a new JSON-RPC
// id) carrying InputResponses and echoing RequestState verbatim. A server MUST
// include at least one of InputRequests or RequestState.
type InputRequiredResult struct {
	ResultType    string         `json:"resultType"`
	InputRequests InputRequests  `json:"inputRequests,omitempty"`
	RequestState  string         `json:"requestState,omitempty"`
	Meta          map[string]any `json:"_meta,omitempty"`
}

// MRTRContinuation carries the client's answers on the retry of an MRTR
// request. It is merged into the request params alongside the original
// arguments. RequestState is echoed verbatim from the InputRequiredResult.
type MRTRContinuation struct {
	InputResponses InputResponses `json:"inputResponses,omitempty"`
	RequestState   string         `json:"requestState,omitempty"`
}

// Root is a client-declared root URI visible to the server.
type Root struct {
	URI  string         `json:"uri"`
	Name string         `json:"name,omitempty"`
	Meta map[string]any `json:"_meta,omitempty"`
}

// ListRootsResult is returned from roots/list.
type ListRootsResult struct {
	Roots []Root         `json:"roots"`
	Meta  map[string]any `json:"_meta,omitempty"`
}

// Tool represents a tool definition from an MCP server.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Prompt represents a prompt definition from an MCP server.
type Prompt struct {
	Name         string           `json:"name"`
	OriginalName string           `json:"originalName,omitempty"`
	Description  string           `json:"description,omitempty"`
	Arguments    []PromptArgument `json:"arguments,omitempty"`
	Server       string           `json:"server,omitempty"`
}

// PromptArgument describes a prompt argument.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ModelHint suggests a model family or name for client-side selection.
type ModelHint struct {
	Name string `json:"name,omitempty"`
}

// ModelPreferences guides the client when selecting a model for sampling.
type ModelPreferences struct {
	Hints                []ModelHint `json:"hints,omitempty"`
	CostPriority         float64     `json:"costPriority,omitempty"`
	SpeedPriority        float64     `json:"speedPriority,omitempty"`
	IntelligencePriority float64     `json:"intelligencePriority,omitempty"`
}

// SamplingToolChoice controls tool use during sampling.
type SamplingToolChoice struct {
	Mode string `json:"mode,omitempty"`
}

// SamplingTool defines a tool the model may use during sampling.
type SamplingTool = Tool

// SamplingMessage is a single message in an MCP sampling request or response.
// The content field may be either a single content block or an array of blocks.
type SamplingMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Meta    map[string]any  `json:"_meta,omitempty"`
}

// CreateMessageParams is the request payload for sampling/createMessage.
type CreateMessageParams struct {
	Messages         []SamplingMessage   `json:"messages"`
	ModelPreferences *ModelPreferences   `json:"modelPreferences,omitempty"`
	SystemPrompt     string              `json:"systemPrompt,omitempty"`
	IncludeContext   string              `json:"includeContext,omitempty"`
	Temperature      *float64            `json:"temperature,omitempty"`
	MaxTokens        int                 `json:"maxTokens"`
	StopSequences    []string            `json:"stopSequences,omitempty"`
	Metadata         map[string]any      `json:"metadata,omitempty"`
	Tools            []SamplingTool      `json:"tools,omitempty"`
	ToolChoice       *SamplingToolChoice `json:"toolChoice,omitempty"`
	Meta             map[string]any      `json:"_meta,omitempty"`
}

// CreateMessageResult is returned by a client in response to sampling/createMessage.
type CreateMessageResult struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Model      string          `json:"model"`
	StopReason string          `json:"stopReason,omitempty"`
	Meta       map[string]any  `json:"_meta,omitempty"`
}

// ElicitationParams is the request payload for elicitation/create.
// When Mode is omitted, clients should treat it as form mode.
type ElicitationParams struct {
	Mode            string          `json:"mode,omitempty"`
	Message         string          `json:"message"`
	RequestedSchema json.RawMessage `json:"requestedSchema,omitempty"`
	ElicitationID   string          `json:"elicitationId,omitempty"`
	URL             string          `json:"url,omitempty"`
	Meta            map[string]any  `json:"_meta,omitempty"`
}

// ElicitationResult is returned by a client in response to elicitation/create.
type ElicitationResult struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// PromptMessage is a single message returned by prompts/get.
type PromptMessage struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

// PromptResult is the result of prompts/get.
type PromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
	Meta        map[string]any  `json:"_meta,omitempty"`
}

// Resource describes a resource exposed by an MCP server.
type Resource struct {
	URI         string         `json:"uri"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	MIMEType    string         `json:"mimeType,omitempty"`
	Size        int64          `json:"size,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
	Server      string         `json:"server,omitempty"`
}

// ResourceTemplate describes a templated resource URI.
type ResourceTemplate struct {
	URITemplate  string         `json:"uriTemplate"`
	Name         string         `json:"name,omitempty"`
	OriginalName string         `json:"originalName,omitempty"`
	Description  string         `json:"description,omitempty"`
	MIMEType     string         `json:"mimeType,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`
	Server       string         `json:"server,omitempty"`
}

// ResourceContents is a single entry returned by resources/read.
type ResourceContents struct {
	URI      string          `json:"uri"`
	MIMEType string          `json:"mimeType,omitempty"`
	Text     string          `json:"text,omitempty"`
	Blob     string          `json:"blob,omitempty"`
	Data     any             `json:"data,omitempty"`
	Meta     map[string]any  `json:"_meta,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// ReadResourceResult is the result of resources/read. The 2026-07-28 spec
// requires resources/read results to carry the cache freshness hints embedded
// via CacheableResult.
type ReadResourceResult struct {
	Contents []ResourceContents `json:"contents"`
	CacheableResult
	Meta map[string]any `json:"_meta,omitempty"`
}

// ToolResult represents the result of an MCP tool call.
type ToolResult struct {
	Content           []Content      `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
}

// Content represents a content block in an MCP response.
type Content struct {
	Type              string            `json:"type"`
	Text              string            `json:"text,omitempty"`
	MIMEType          string            `json:"mimeType,omitempty"`
	URI               string            `json:"uri,omitempty"`
	Blob              string            `json:"blob,omitempty"`
	Data              any               `json:"data,omitempty"`
	ID                string            `json:"id,omitempty"`
	Name              string            `json:"name,omitempty"`
	Input             json.RawMessage   `json:"input,omitempty"`
	ToolUseID         string            `json:"toolUseId,omitempty"`
	Resource          *ResourceContents `json:"resource,omitempty"`
	Content           []Content         `json:"content,omitempty"`
	StructuredContent any               `json:"structuredContent,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
	Annotations       map[string]any    `json:"annotations,omitempty"`
	Meta              map[string]any    `json:"_meta,omitempty"`
	Raw               json.RawMessage   `json:"-"`
}

func (c *Content) UnmarshalJSON(data []byte) error {
	type contentWire Content
	var decoded contentWire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = Content(decoded)
	c.Raw = append(c.Raw[:0], data...)
	return nil
}

func (c *ResourceContents) UnmarshalJSON(data []byte) error {
	type resourceContentsWire ResourceContents
	var decoded resourceContentsWire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ResourceContents(decoded)
	c.Raw = append(c.Raw[:0], data...)
	return nil
}

// TextContent returns the concatenated textual content from the tool result.
// If no text blocks are present, it falls back to structured content JSON.
func (r *ToolResult) TextContent() string {
	return joinContentText(r.Content, r.StructuredContent)
}

// TextContent returns the concatenated textual content from the resource.
func (r *ReadResourceResult) TextContent() string {
	if r == nil {
		return ""
	}
	parts := make([]string, 0, len(r.Contents))
	for _, content := range r.Contents {
		if text := content.textContent(); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// TextContent returns the concatenated textual content from the prompt result.
func (r *PromptResult) TextContent() string {
	if r == nil {
		return ""
	}
	parts := make([]string, 0, len(r.Messages))
	for _, message := range r.Messages {
		text := message.Content.textContent()
		if text == "" {
			continue
		}
		if message.Role != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", message.Role, text))
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n")
}

// ParseSamplingContent parses a sampling content field that may contain either
// a single block or an array of blocks.
func ParseSamplingContent(raw json.RawMessage) ([]Content, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var blocks []Content
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return nil, err
		}
		return blocks, nil
	}

	var block Content
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, err
	}
	return []Content{block}, nil
}

// MarshalSamplingContent marshals a single sampling content block.
func MarshalSamplingContent(block Content) json.RawMessage {
	data, _ := json.Marshal(block)
	return data
}

// MarshalSamplingContentArray marshals multiple sampling content blocks.
func MarshalSamplingContentArray(blocks []Content) json.RawMessage {
	data, _ := json.Marshal(blocks)
	return data
}

func joinContentText(content []Content, structured any) string {
	parts := make([]string, 0, len(content))
	for _, block := range content {
		if text := block.textContent(); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		if fallback := stringifyContentFallback(structured); fallback != "" {
			parts = append(parts, fallback)
		}
	}
	return strings.Join(parts, "\n")
}

func (c Content) textContent() string {
	switch {
	case c.Text != "":
		return c.Text
	case c.Resource != nil:
		return c.Resource.textContent()
	case c.Data != nil:
		return stringifyContentFallback(c.Data)
	case c.Blob != "":
		return c.Blob
	case c.URI != "":
		return c.URI
	default:
		return ""
	}
}

func (c ResourceContents) textContent() string {
	switch {
	case c.Text != "":
		return c.Text
	case c.Data != nil:
		return stringifyContentFallback(c.Data)
	case c.Blob != "":
		return c.Blob
	case c.URI != "":
		return c.URI
	default:
		return ""
	}
}

func stringifyContentFallback(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.RawMessage:
		return string(v)
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return ""
		}
		return string(data)
	}
}

type listToolsResult struct {
	Tools []Tool `json:"tools"`
	CacheableResult
}

type listResourcesResult struct {
	Resources []Resource `json:"resources"`
	CacheableResult
}

type listResourceTemplatesResult struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
	CacheableResult
}

type listPromptsResult struct {
	Prompts []Prompt `json:"prompts"`
	CacheableResult
}
