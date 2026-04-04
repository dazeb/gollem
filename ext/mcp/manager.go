package mcp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// ToolSource is the interface for any MCP client that can provide tools.
// Client, SSEClient, and HTTPClient implement this interface.
type ToolSource interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error)
}

// ResourceSource is implemented by clients that support MCP resources.
type ResourceSource interface {
	ListResources(ctx context.Context) ([]Resource, error)
	ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error)
	ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error)
}

// PromptSource is implemented by clients that support MCP prompts.
type PromptSource interface {
	ListPrompts(ctx context.Context) ([]Prompt, error)
	GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptResult, error)
}

// NotificationSource is implemented by clients that can surface server notifications.
type NotificationSource interface {
	OnNotification(method string, handler NotificationHandler) func()
}

// Ensure client types implement the protocol interfaces.
var (
	_ ToolSource         = (*Client)(nil)
	_ ToolSource         = (*SSEClient)(nil)
	_ ToolSource         = (*HTTPClient)(nil)
	_ ResourceSource     = (*Client)(nil)
	_ ResourceSource     = (*SSEClient)(nil)
	_ ResourceSource     = (*HTTPClient)(nil)
	_ PromptSource       = (*Client)(nil)
	_ PromptSource       = (*SSEClient)(nil)
	_ PromptSource       = (*HTTPClient)(nil)
	_ NotificationSource = (*Client)(nil)
	_ NotificationSource = (*SSEClient)(nil)
	_ NotificationSource = (*HTTPClient)(nil)
)

// ServerConfig configures an MCP server within the manager.
type ServerConfig struct {
	Name   string
	Source ToolSource
}

type managedServer struct {
	source     ToolSource
	unregister []func()

	cacheMu                sync.RWMutex
	tools                  []Tool
	toolsValid             bool
	resources              []Resource
	resourcesValid         bool
	resourceTemplates      []ResourceTemplate
	resourceTemplatesValid bool
	prompts                []Prompt
	promptsValid           bool
}

func (s *managedServer) invalidateTools() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.tools = nil
	s.toolsValid = false
}

func (s *managedServer) invalidateResources() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.resources = nil
	s.resourcesValid = false
	s.resourceTemplates = nil
	s.resourceTemplatesValid = false
}

func (s *managedServer) invalidatePrompts() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.prompts = nil
	s.promptsValid = false
}

func (s *managedServer) cachedToolsCopy() ([]Tool, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if !s.toolsValid {
		return nil, false
	}
	return append([]Tool(nil), s.tools...), true
}

func (s *managedServer) cachedResourcesCopy() ([]Resource, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if !s.resourcesValid {
		return nil, false
	}
	return append([]Resource(nil), s.resources...), true
}

func (s *managedServer) cachedResourceTemplatesCopy() ([]ResourceTemplate, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if !s.resourceTemplatesValid {
		return nil, false
	}
	return append([]ResourceTemplate(nil), s.resourceTemplates...), true
}

func (s *managedServer) cachedPromptsCopy() ([]Prompt, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if !s.promptsValid {
		return nil, false
	}
	return append([]Prompt(nil), s.prompts...), true
}

func (s *managedServer) storeTools(tools []Tool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.tools = append([]Tool(nil), tools...)
	s.toolsValid = true
}

func (s *managedServer) storeResources(resources []Resource) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.resources = append([]Resource(nil), resources...)
	s.resourcesValid = true
}

func (s *managedServer) storeResourceTemplates(templates []ResourceTemplate) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.resourceTemplates = append([]ResourceTemplate(nil), templates...)
	s.resourceTemplatesValid = true
}

func (s *managedServer) storePrompts(prompts []Prompt) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.prompts = append([]Prompt(nil), prompts...)
	s.promptsValid = true
}

// Manager aggregates tools, resources, and prompts from multiple MCP servers,
// applying prefix-based namespacing to avoid collisions.
type Manager struct {
	mu      sync.Mutex
	servers map[string]*managedServer
	sep     string
}

// ManagerOption configures the manager.
type ManagerOption func(*managerConfig)

type managerConfig struct {
	sep string
}

// WithSeparator sets the separator used between server name and tool/prompt name.
// Default is "__" (double underscore).
func WithSeparator(sep string) ManagerOption {
	return func(c *managerConfig) {
		c.sep = sep
	}
}

// NewManager creates a new multi-server MCP manager.
func NewManager(opts ...ManagerOption) *Manager {
	cfg := &managerConfig{sep: "__"}
	for _, opt := range opts {
		opt(cfg)
	}

	return &Manager{
		servers: make(map[string]*managedServer),
		sep:     cfg.sep,
	}
}

// AddServer registers an MCP server with the manager under the given name.
func (m *Manager) AddServer(name string, source ToolSource) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.servers[name]; exists {
		return fmt.Errorf("mcp: server %q already registered", name)
	}

	server := &managedServer{source: source}
	if notifier, ok := source.(NotificationSource); ok {
		server.unregister = append(server.unregister,
			notifier.OnNotification("notifications/tools/list_changed", func(Notification) {
				server.invalidateTools()
			}),
			notifier.OnNotification("notifications/resources/list_changed", func(Notification) {
				server.invalidateResources()
			}),
			notifier.OnNotification("notifications/prompts/list_changed", func(Notification) {
				server.invalidatePrompts()
			}),
		)
	}

	m.servers[name] = server
	return nil
}

// RemoveServer unregisters an MCP server. If the source implements io.Closer,
// it is also closed.
func (m *Manager) RemoveServer(name string) error {
	m.mu.Lock()
	server, exists := m.servers[name]
	if exists {
		delete(m.servers, name)
	}
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("mcp: server %q not found", name)
	}

	for _, unregister := range server.unregister {
		unregister()
	}

	if closer, ok := server.source.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Tools returns all registered tools with namespaced names (for example,
// "server__tool_name"). Tool lists are cached until the server emits a
// list_changed notification or the server is removed.
func (m *Manager) Tools(ctx context.Context) ([]core.Tool, error) {
	servers := m.snapshotServers()

	type result struct {
		name  string
		tools []Tool
		err   error
	}

	ch := make(chan result, len(servers))
	var wg sync.WaitGroup

	for name, server := range servers {
		wg.Add(1)
		go func(name string, server *managedServer) {
			defer wg.Done()
			tools, err := m.serverTools(ctx, server)
			ch <- result{name: name, tools: tools, err: err}
		}(name, server)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var allTools []core.Tool
	var errs []error

	for r := range ch {
		if r.err != nil {
			errs = append(errs, fmt.Errorf("server %q: %w", r.name, r.err))
			continue
		}
		for _, mt := range r.tools {
			allTools = append(allTools, m.convertManagedTool(r.name, mt, servers[r.name].source))
		}
	}

	if len(errs) > 0 && len(allTools) == 0 {
		return nil, fmt.Errorf("mcp: all servers failed: %v", errs)
	}
	return allTools, nil
}

// ListResources aggregates resources from all registered MCP servers.
func (m *Manager) ListResources(ctx context.Context) ([]Resource, error) {
	servers := m.snapshotServers()

	type result struct {
		name      string
		resources []Resource
		skipped   bool
		err       error
	}

	ch := make(chan result, len(servers))
	var wg sync.WaitGroup

	for name, server := range servers {
		wg.Add(1)
		go func(name string, server *managedServer) {
			defer wg.Done()
			resourceSource, ok := server.source.(ResourceSource)
			if !ok {
				ch <- result{name: name, skipped: true}
				return
			}
			resources, err := m.serverResources(ctx, server, resourceSource)
			ch <- result{name: name, resources: attachResourceServer(resources, name), err: err}
		}(name, server)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []Resource
	var errs []error
	active := 0

	for r := range ch {
		if r.skipped {
			continue
		}
		active++
		if r.err != nil {
			errs = append(errs, fmt.Errorf("server %q: %w", r.name, r.err))
			continue
		}
		all = append(all, r.resources...)
	}

	if len(errs) > 0 && active > 0 && len(all) == 0 {
		return nil, fmt.Errorf("mcp: all resource-capable servers failed: %v", errs)
	}
	return all, nil
}

// ReadResource reads a resource from a named server.
func (m *Manager) ReadResource(ctx context.Context, serverName, uri string) (*ReadResourceResult, error) {
	server, err := m.lookupServer(serverName)
	if err != nil {
		return nil, err
	}
	resourceSource, ok := server.source.(ResourceSource)
	if !ok {
		return nil, fmt.Errorf("mcp: server %q does not support resources", serverName)
	}
	return resourceSource.ReadResource(ctx, uri)
}

// ListResourceTemplates aggregates resource templates from all registered servers.
func (m *Manager) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	servers := m.snapshotServers()

	type result struct {
		name      string
		templates []ResourceTemplate
		skipped   bool
		err       error
	}

	ch := make(chan result, len(servers))
	var wg sync.WaitGroup

	for name, server := range servers {
		wg.Add(1)
		go func(name string, server *managedServer) {
			defer wg.Done()
			resourceSource, ok := server.source.(ResourceSource)
			if !ok {
				ch <- result{name: name, skipped: true}
				return
			}
			templates, err := m.serverResourceTemplates(ctx, server, resourceSource)
			ch <- result{name: name, templates: attachTemplateServer(templates, name, m.sep), err: err}
		}(name, server)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []ResourceTemplate
	var errs []error
	active := 0

	for r := range ch {
		if r.skipped {
			continue
		}
		active++
		if r.err != nil {
			errs = append(errs, fmt.Errorf("server %q: %w", r.name, r.err))
			continue
		}
		all = append(all, r.templates...)
	}

	if len(errs) > 0 && active > 0 && len(all) == 0 {
		return nil, fmt.Errorf("mcp: all resource-template-capable servers failed: %v", errs)
	}
	return all, nil
}

// ListPrompts aggregates prompts from all registered MCP servers.
func (m *Manager) ListPrompts(ctx context.Context) ([]Prompt, error) {
	servers := m.snapshotServers()

	type result struct {
		name    string
		prompts []Prompt
		skipped bool
		err     error
	}

	ch := make(chan result, len(servers))
	var wg sync.WaitGroup

	for name, server := range servers {
		wg.Add(1)
		go func(name string, server *managedServer) {
			defer wg.Done()
			promptSource, ok := server.source.(PromptSource)
			if !ok {
				ch <- result{name: name, skipped: true}
				return
			}
			prompts, err := m.serverPrompts(ctx, server, promptSource)
			ch <- result{name: name, prompts: attachPromptServer(prompts, name, m.sep), err: err}
		}(name, server)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []Prompt
	var errs []error
	active := 0

	for r := range ch {
		if r.skipped {
			continue
		}
		active++
		if r.err != nil {
			errs = append(errs, fmt.Errorf("server %q: %w", r.name, r.err))
			continue
		}
		all = append(all, r.prompts...)
	}

	if len(errs) > 0 && active > 0 && len(all) == 0 {
		return nil, fmt.Errorf("mcp: all prompt-capable servers failed: %v", errs)
	}
	return all, nil
}

// GetPrompt resolves a namespaced prompt from the appropriate server.
func (m *Manager) GetPrompt(ctx context.Context, namespacedName string, args map[string]string) (*PromptResult, error) {
	serverName, promptName, ok := m.ParseToolName(namespacedName)
	if !ok {
		return nil, fmt.Errorf("mcp: prompt name %q is not namespaced", namespacedName)
	}

	server, err := m.lookupServer(serverName)
	if err != nil {
		return nil, err
	}
	promptSource, ok := server.source.(PromptSource)
	if !ok {
		return nil, fmt.Errorf("mcp: server %q does not support prompts", serverName)
	}
	return promptSource.GetPrompt(ctx, promptName, args)
}

func (m *Manager) serverTools(ctx context.Context, server *managedServer) ([]Tool, error) {
	if tools, ok := server.cachedToolsCopy(); ok {
		return tools, nil
	}
	tools, err := server.source.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	server.storeTools(tools)
	return append([]Tool(nil), tools...), nil
}

func (m *Manager) serverResources(ctx context.Context, server *managedServer, source ResourceSource) ([]Resource, error) {
	if resources, ok := server.cachedResourcesCopy(); ok {
		return resources, nil
	}
	resources, err := source.ListResources(ctx)
	if err != nil {
		return nil, err
	}
	server.storeResources(resources)
	return append([]Resource(nil), resources...), nil
}

func (m *Manager) serverResourceTemplates(ctx context.Context, server *managedServer, source ResourceSource) ([]ResourceTemplate, error) {
	if templates, ok := server.cachedResourceTemplatesCopy(); ok {
		return templates, nil
	}
	templates, err := source.ListResourceTemplates(ctx)
	if err != nil {
		return nil, err
	}
	server.storeResourceTemplates(templates)
	return append([]ResourceTemplate(nil), templates...), nil
}

func (m *Manager) serverPrompts(ctx context.Context, server *managedServer, source PromptSource) ([]Prompt, error) {
	if prompts, ok := server.cachedPromptsCopy(); ok {
		return prompts, nil
	}
	prompts, err := source.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	server.storePrompts(prompts)
	return append([]Prompt(nil), prompts...), nil
}

func (m *Manager) convertManagedTool(serverName string, mt Tool, source ToolSource) core.Tool {
	description := mt.Description
	if description != "" {
		description = fmt.Sprintf("[%s] %s", serverName, description)
	} else {
		description = fmt.Sprintf("[%s]", serverName)
	}
	return buildCoreTool(serverName+m.sep+mt.Name, description, mt.InputSchema, source, mt.Name)
}

// ServerNames returns the names of all registered servers.
func (m *Manager) ServerNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	return names
}

// ParseToolName splits a namespaced tool or prompt name into server name and inner name.
func (m *Manager) ParseToolName(namespacedName string) (serverName, toolName string, ok bool) {
	idx := strings.Index(namespacedName, m.sep)
	if idx < 0 {
		return "", namespacedName, false
	}
	return namespacedName[:idx], namespacedName[idx+len(m.sep):], true
}

// Close closes all registered servers that implement io.Closer.
func (m *Manager) Close() error {
	m.mu.Lock()
	servers := make(map[string]*managedServer, len(m.servers))
	for name, server := range m.servers {
		servers[name] = server
	}
	m.servers = make(map[string]*managedServer)
	m.mu.Unlock()

	var errs []error
	for name, server := range servers {
		for _, unregister := range server.unregister {
			unregister()
		}
		if closer, ok := server.source.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("server %q: %w", name, err))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("mcp: errors closing servers: %v", errs)
	}
	return nil
}

func (m *Manager) snapshotServers() map[string]*managedServer {
	m.mu.Lock()
	defer m.mu.Unlock()

	servers := make(map[string]*managedServer, len(m.servers))
	for name, server := range m.servers {
		servers[name] = server
	}
	return servers
}

func (m *Manager) lookupServer(name string) (*managedServer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	server, ok := m.servers[name]
	if !ok {
		return nil, fmt.Errorf("mcp: server %q not found", name)
	}
	return server, nil
}

func attachResourceServer(resources []Resource, server string) []Resource {
	out := make([]Resource, len(resources))
	for i, resource := range resources {
		resource.Server = server
		out[i] = resource
	}
	return out
}

func attachTemplateServer(templates []ResourceTemplate, server, sep string) []ResourceTemplate {
	out := make([]ResourceTemplate, len(templates))
	for i, template := range templates {
		template.Server = server
		if template.Name != "" {
			template.OriginalName = template.Name
			template.Name = server + sep + template.Name
		}
		out[i] = template
	}
	return out
}

func attachPromptServer(prompts []Prompt, server, sep string) []Prompt {
	out := make([]Prompt, len(prompts))
	for i, prompt := range prompts {
		prompt.Server = server
		prompt.OriginalName = prompt.Name
		prompt.Name = server + sep + prompt.Name
		out[i] = prompt
	}
	return out
}
