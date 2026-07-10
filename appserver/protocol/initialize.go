package protocol

import (
	"runtime"
	"strings"
)

const (
	ProtocolVersion = "gollem.appserver.v1"
	SchemaVersion   = "gollem.appserver.schema.v1"
)

// ImplementationInfo identifies the app-server implementation.
type ImplementationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ClientInfo identifies a Codex-compatible app-server client.
type ClientInfo struct {
	Name    string  `json:"name"`
	Title   *string `json:"title,omitempty"`
	Version string  `json:"version"`
}

// InitializeParams is sent by the client as the first request on a connection.
type InitializeParams struct {
	ClientInfo   ClientInfo              `json:"clientInfo"`
	Capabilities *InitializeCapabilities `json:"capabilities,omitempty"`
}

// InitializeCapabilities are client-advertised connection capabilities.
type InitializeCapabilities struct {
	ExperimentalAPI                bool            `json:"experimentalApi,omitempty"`
	RequestAttestation             bool            `json:"requestAttestation,omitempty"`
	MCPServerOpenAIFormElicitation bool            `json:"mcpServerOpenaiFormElicitation,omitempty"`
	OptOutNotificationMethods      []string        `json:"optOutNotificationMethods,omitempty"`
	Experimental                   map[string]bool `json:"experimental,omitempty"`
}

// InitializeResponse is returned after a successful initialize request.
type InitializeResponse struct {
	UserAgent       string             `json:"userAgent"`
	CodexHome       string             `json:"codexHome"`
	PlatformFamily  string             `json:"platformFamily"`
	PlatformOS      string             `json:"platformOs"`
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      ImplementationInfo `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	Methods         []MethodInfo       `json:"methods,omitempty"`
}

// ServerCapabilities advertises app-server features available on the current build.
type ServerCapabilities struct {
	ProtocolSchema bool     `json:"protocolSchema"`
	Unavailable    bool     `json:"unavailable"`
	Experimental   []string `json:"experimental,omitempty"`
}

// DefaultInitializeResponse returns the Codex-compatible server identity plus
// Gollem protocol capabilities and method metadata.
func DefaultInitializeResponse(server ImplementationInfo, appHome string) InitializeResponse {
	return InitializeResponse{
		UserAgent:       implementationUserAgent(server),
		CodexHome:       appHome,
		PlatformFamily:  initializePlatformFamily(runtime.GOOS),
		PlatformOS:      initializePlatformOS(runtime.GOOS),
		ProtocolVersion: ProtocolVersion,
		ServerInfo:      server,
		Capabilities: ServerCapabilities{
			ProtocolSchema: true,
			Unavailable:    true,
		},
		Methods: Methods(),
	}
}

func implementationUserAgent(info ImplementationInfo) string {
	if strings.TrimSpace(info.Version) == "" {
		return info.Name
	}
	return info.Name + "/" + info.Version
}

func initializePlatformFamily(goos string) string {
	if goos == "windows" {
		return "windows"
	}
	return "unix"
}

func initializePlatformOS(goos string) string {
	if goos == "darwin" {
		return "macos"
	}
	return goos
}
