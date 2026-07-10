package config

import (
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrKeyRequired = errors.New("config key is required")

type EnvLookup func(string) (string, bool)

type Option func(*Service)

func WithEnvLookup(lookup EnvLookup) Option {
	return func(s *Service) {
		if lookup != nil {
			s.env = lookup
		}
	}
}

func WithWorkDir(workDir string) Option {
	return func(s *Service) {
		s.workDir = strings.TrimSpace(workDir)
	}
}

type Service struct {
	mu           sync.RWMutex
	env          EnvLookup
	workDir      string
	values       map[string]configValue
	experiments  map[string]bool
	environments map[string]Environment
	mcpReloads   int
}

type configValue struct {
	value     json.RawMessage
	source    string
	updatedAt time.Time
}

type ReadParams struct {
	Keys          []string `json:"keys,omitempty"`
	IncludeValues *bool    `json:"includeValues,omitempty"`
}

type ReadResponse struct {
	Values  map[string]json.RawMessage `json:"values"`
	Entries []Entry                    `json:"entries"`
}

type Entry struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Source      string          `json:"source"`
	Writable    bool            `json:"writable"`
	Redacted    bool            `json:"redacted,omitempty"`
	Description string          `json:"description,omitempty"`
	UpdatedAt   time.Time       `json:"updatedAt,omitempty"`
}

type ValueWriteParams struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type BatchWriteParams struct {
	Values  map[string]json.RawMessage `json:"values,omitempty"`
	Entries []ValueWriteParams         `json:"entries,omitempty"`
}

type WriteResponse struct {
	Values  map[string]json.RawMessage `json:"values"`
	Entries []Entry                    `json:"entries"`
}

type RequirementsResponse struct {
	Requirements []Requirement `json:"requirements"`
	Data         []Requirement `json:"data"`
}

type Requirement struct {
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Satisfied   bool     `json:"satisfied"`
	ValueType   string   `json:"valueType"`
	Source      string   `json:"source"`
	Secret      bool     `json:"secret,omitempty"`
	EnvVars     []string `json:"envVars,omitempty"`
}

type EnvironmentAddParams struct {
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	WorkDir   string            `json:"workdir,omitempty"`
	WorkDir2  string            `json:"workDir,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

type EnvironmentInfoResponse struct {
	CurrentID    string        `json:"currentId"`
	Environment  Environment   `json:"environment"`
	Environments []Environment `json:"environments"`
	Data         []Environment `json:"data"`
}

type EnvironmentResponse struct {
	Environment Environment `json:"environment"`
}

type Environment struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	WorkDir       string       `json:"workdir"`
	WorkDir2      string       `json:"workDir"`
	OS            string       `json:"os"`
	Arch          string       `json:"arch"`
	Shell         string       `json:"shell,omitempty"`
	HomeSet       bool         `json:"homeSet"`
	Variables     []EnvVarInfo `json:"variables"`
	CreatedAt     time.Time    `json:"createdAt"`
	LastUpdatedAt time.Time    `json:"lastUpdatedAt"`
}

type EnvVarInfo struct {
	Name     string `json:"name"`
	Set      bool   `json:"set"`
	Redacted bool   `json:"redacted"`
	Category string `json:"category"`
}

type CollaborationModesResponse struct {
	Modes []CollaborationMode `json:"modes"`
	Data  []CollaborationMode `json:"data"`
}

type CollaborationMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

type PermissionProfilesResponse struct {
	Profiles []PermissionProfile `json:"profiles"`
	Data     []PermissionProfile `json:"data"`
}

type PermissionProfile struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Default          bool     `json:"default"`
	CanRead          bool     `json:"canRead"`
	CanWrite         bool     `json:"canWrite"`
	CanExecute       bool     `json:"canExecute"`
	RequiresApproval bool     `json:"requiresApproval"`
	Capabilities     []string `json:"capabilities"`
}

type ExperimentalFeatureListResponse struct {
	Features []ExperimentalFeature `json:"features"`
	Data     []ExperimentalFeature `json:"data"`
}

type ExperimentalFeature struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Stable      bool   `json:"stable"`
}

type ExperimentalFeatureSetParams struct {
	ID      string `json:"id,omitempty"`
	Feature string `json:"feature,omitempty"`
	Enabled bool   `json:"enabled"`
}

type ExperimentalFeatureSetResponse struct {
	Feature ExperimentalFeature `json:"feature"`
}

type MCPReloadResponse struct {
	Reloaded bool   `json:"reloaded"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	Count    int    `json:"count"`
}

func NewService(opts ...Option) *Service {
	s := &Service{
		env:          os.LookupEnv,
		values:       map[string]configValue{},
		experiments:  map[string]bool{},
		environments: map[string]Environment{},
	}
	for _, opt := range opts {
		opt(s)
	}
	s.installDefaults(time.Now().UTC())
	return s
}

func (s *Service) Read(params ReadParams) ReadResponse {
	s = ensureService(s)
	includeValues := params.IncludeValues == nil || *params.IncludeValues
	keys := params.Keys

	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(keys) == 0 {
		keys = make([]string, 0, len(s.values))
		for key := range s.values {
			keys = append(keys, key)
		}
		slices.Sort(keys)
	}
	entries := make([]Entry, 0, len(keys))
	values := make(map[string]json.RawMessage, len(keys))
	for _, key := range keys {
		key = normalizeKey(key)
		value, ok := s.values[key]
		if !ok {
			continue
		}
		entry := s.entryLocked(key, value, includeValues)
		entries = append(entries, entry)
		values[key] = append(json.RawMessage(nil), entry.Value...)
	}
	return ReadResponse{Values: values, Entries: entries}
}

func (s *Service) WriteValue(params ValueWriteParams) (WriteResponse, error) {
	s = ensureService(s)
	key := normalizeKey(params.Key)
	if key == "" {
		return WriteResponse{}, ErrKeyRequired
	}
	value := normalizeValue(params.Value)
	now := time.Now().UTC()

	s.mu.Lock()
	s.values[key] = configValue{value: value, source: "runtime", updatedAt: now}
	entry := s.entryLocked(key, s.values[key], true)
	s.mu.Unlock()

	return WriteResponse{
		Values:  map[string]json.RawMessage{key: append(json.RawMessage(nil), entry.Value...)},
		Entries: []Entry{entry},
	}, nil
}

func (s *Service) BatchWrite(params BatchWriteParams) (WriteResponse, error) {
	s = ensureService(s)
	writes := append([]ValueWriteParams(nil), params.Entries...)
	for key, value := range params.Values {
		writes = append(writes, ValueWriteParams{Key: key, Value: value})
	}
	if len(writes) == 0 {
		return WriteResponse{Values: map[string]json.RawMessage{}, Entries: []Entry{}}, nil
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	entries := make([]Entry, 0, len(writes))
	values := make(map[string]json.RawMessage, len(writes))
	for _, write := range writes {
		key := normalizeKey(write.Key)
		if key == "" {
			return WriteResponse{}, ErrKeyRequired
		}
		s.values[key] = configValue{value: normalizeValue(write.Value), source: "runtime", updatedAt: now}
		entry := s.entryLocked(key, s.values[key], true)
		entries = append(entries, entry)
		values[key] = append(json.RawMessage(nil), entry.Value...)
	}
	return WriteResponse{Values: values, Entries: entries}, nil
}

func (s *Service) Requirements() RequirementsResponse {
	s = ensureService(s)
	requirements := []Requirement{
		envRequirement("openai.apiKey", "OpenAI API key", "Configures OpenAI models.", []string{"OPENAI_API_KEY", "CHATGPT_ACCESS_TOKEN"}, s.env),
		envRequirement("anthropic.apiKey", "Anthropic API key", "Configures Anthropic models.", []string{"ANTHROPIC_API_KEY"}, s.env),
		envRequirement("vertex.project", "Vertex AI project", "Configures Google Vertex AI models.", []string{"GOOGLE_CLOUD_PROJECT"}, s.env),
		{
			ID:          "workspace.root",
			Key:         "workspace.root",
			Name:        "Workspace root",
			Description: "Workspace path used to scope filesystem and process operations.",
			Required:    true,
			Satisfied:   strings.TrimSpace(s.workDir) != "",
			ValueType:   "path",
			Source:      "app-server",
		},
	}
	return RequirementsResponse{Requirements: cloneRequirements(requirements), Data: cloneRequirements(requirements)}
}

func (s *Service) EnvironmentInfo() EnvironmentInfoResponse {
	s = ensureService(s)
	current := s.currentEnvironment()

	s.mu.RLock()
	stored := make([]Environment, 0, len(s.environments)+1)
	stored = append(stored, current)
	for _, env := range s.environments {
		stored = append(stored, cloneEnvironment(env))
	}
	s.mu.RUnlock()
	return EnvironmentInfoResponse{
		CurrentID:    current.ID,
		Environment:  current,
		Environments: cloneEnvironments(stored),
		Data:         cloneEnvironments(stored),
	}
}

func (s *Service) AddEnvironment(params EnvironmentAddParams) (EnvironmentResponse, error) {
	s = ensureService(s)
	id := normalizeKey(params.ID)
	if id == "" {
		id = "environment-" + time.Now().UTC().Format("20060102150405")
	}
	workDir := firstNonEmpty(params.WorkDir, params.WorkDir2, s.workDir)
	variables := mergeEnvMaps(params.Variables, params.Env)
	now := time.Now().UTC()
	env := Environment{
		ID:            id,
		Name:          firstNonEmpty(strings.TrimSpace(params.Name), id),
		WorkDir:       workDir,
		WorkDir2:      workDir,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Shell:         lookupFirst(s.env, "SHELL", "COMSPEC"),
		HomeSet:       lookupFirst(s.env, "HOME", "USERPROFILE") != "",
		Variables:     envVarInfosFromMap(variables),
		CreatedAt:     now,
		LastUpdatedAt: now,
	}
	s.mu.Lock()
	s.environments[id] = env
	s.mu.Unlock()
	return EnvironmentResponse{Environment: cloneEnvironment(env)}, nil
}

func (s *Service) CollaborationModes() CollaborationModesResponse {
	modes := []CollaborationMode{
		{ID: "default", Name: "Default", Description: "Agent may plan, execute, verify, and report progress within configured safeguards.", Default: true},
		{ID: "plan", Name: "Plan", Description: "Agent proposes a plan before implementation work begins.", Default: false},
		{ID: "review", Name: "Review", Description: "Agent inspects changes and prioritizes findings, risks, and test gaps.", Default: false},
	}
	return CollaborationModesResponse{Modes: append([]CollaborationMode(nil), modes...), Data: append([]CollaborationMode(nil), modes...)}
}

func (s *Service) PermissionProfiles() PermissionProfilesResponse {
	profiles := []PermissionProfile{
		{ID: "read-only", Name: "Read only", Description: "Read files and metadata without writes, commands, or Git mutations.", CanRead: true, Capabilities: []string{"fs:read", "git:status", "git:diff"}},
		{ID: "workspace-write", Name: "Workspace write", Description: "Read and mutate files inside the configured workspace with approval for writes.", Default: true, CanRead: true, CanWrite: true, RequiresApproval: true, Capabilities: []string{"fs:read", "fs:write", "git:status", "git:diff"}},
		{ID: "full-access", Name: "Full access", Description: "Allow filesystem, process, and Git mutations through explicit approval gates.", CanRead: true, CanWrite: true, CanExecute: true, RequiresApproval: true, Capabilities: []string{"fs:*", "process:*", "git:*", "worktree:*"}},
	}
	return PermissionProfilesResponse{Profiles: clonePermissionProfiles(profiles), Data: clonePermissionProfiles(profiles)}
}

func (s *Service) ExperimentalFeatures() ExperimentalFeatureListResponse {
	s = ensureService(s)
	features := []ExperimentalFeature{
		{ID: "websocket-transport", Name: "WebSocket transport", Description: "Browser-friendly app-server transport.", Enabled: true, Stable: false},
		{ID: "filesystem-watch", Name: "Filesystem watch", Description: "Workspace-scoped file watching.", Enabled: true, Stable: false},
		{ID: "turn-runtime", Name: "Turn runtime", Description: "Provider-neutral asynchronous Gollem turns.", Enabled: true, Stable: false},
		{ID: "interaction-requests", Name: "Interaction requests", Description: "Server-to-client user input, dynamic tool, and MCP elicitation requests.", Enabled: true, Stable: false},
		{ID: "config-service", Name: "Config service", Description: "App-server configuration, environment, and capability discovery.", Enabled: true, Stable: false},
	}
	s.mu.RLock()
	for i := range features {
		if enabled, ok := s.experiments[features[i].ID]; ok {
			features[i].Enabled = enabled
		}
	}
	s.mu.RUnlock()
	return ExperimentalFeatureListResponse{Features: append([]ExperimentalFeature(nil), features...), Data: append([]ExperimentalFeature(nil), features...)}
}

func (s *Service) SetExperimentalFeature(params ExperimentalFeatureSetParams) (ExperimentalFeatureSetResponse, error) {
	s = ensureService(s)
	id := normalizeKey(firstNonEmpty(params.ID, params.Feature))
	if id == "" {
		return ExperimentalFeatureSetResponse{}, errors.New("feature id is required")
	}
	s.mu.Lock()
	s.experiments[id] = params.Enabled
	s.mu.Unlock()
	for _, feature := range s.ExperimentalFeatures().Features {
		if feature.ID == id {
			return ExperimentalFeatureSetResponse{Feature: feature}, nil
		}
	}
	return ExperimentalFeatureSetResponse{
		Feature: ExperimentalFeature{
			ID:          id,
			Name:        id,
			Description: "Runtime-defined experimental feature flag.",
			Enabled:     params.Enabled,
			Stable:      false,
		},
	}, nil
}

func (s *Service) ReloadMCPServers() MCPReloadResponse {
	s = ensureService(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcpReloads++
	return MCPReloadResponse{
		Reloaded: false,
		Status:   "no-op",
		Reason:   "no MCP server registry is configured in this Gollem app-server build",
		Count:    s.mcpReloads,
	}
}

func (s *Service) installDefaults(now time.Time) {
	if s.workDir != "" {
		s.values["workspace.root"] = configValue{value: marshalRaw(s.workDir), source: "app-server", updatedAt: now}
	}
	s.values["provider.default"] = configValue{value: marshalRaw("openai"), source: "default", updatedAt: now}
	s.values["reasoning.effort"] = configValue{value: marshalRaw("medium"), source: "default", updatedAt: now}
	s.values["mutation.approvals.required"] = configValue{value: marshalRaw(true), source: "default", updatedAt: now}
	s.values["appserver.config.version"] = configValue{value: marshalRaw(1), source: "app-server", updatedAt: now}
}

func (s *Service) entryLocked(key string, value configValue, includeValue bool) Entry {
	redacted := isSecretKey(key)
	raw := append(json.RawMessage(nil), value.value...)
	if !includeValue || redacted {
		raw = json.RawMessage("null")
	}
	return Entry{
		Key:         key,
		Value:       raw,
		Source:      value.source,
		Writable:    true,
		Redacted:    redacted,
		Description: descriptionForKey(key),
		UpdatedAt:   value.updatedAt,
	}
}

func (s *Service) currentEnvironment() Environment {
	now := time.Now().UTC()
	env := Environment{
		ID:       "current",
		Name:     "Current process",
		WorkDir:  s.workDir,
		WorkDir2: s.workDir,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Shell:    lookupFirst(s.env, "SHELL", "COMSPEC"),
		HomeSet:  lookupFirst(s.env, "HOME", "USERPROFILE") != "",
		Variables: []EnvVarInfo{
			envVarInfo("OPENAI_API_KEY", "provider", s.env),
			envVarInfo("CHATGPT_ACCESS_TOKEN", "provider", s.env),
			envVarInfo("ANTHROPIC_API_KEY", "provider", s.env),
			envVarInfo("GOOGLE_CLOUD_PROJECT", "provider", s.env),
			envVarInfo("GOOGLE_APPLICATION_CREDENTIALS", "provider", s.env),
		},
		CreatedAt:     now,
		LastUpdatedAt: now,
	}
	return env
}

func envRequirement(id, name, description string, envVars []string, lookup EnvLookup) Requirement {
	return Requirement{
		ID:          id,
		Key:         id,
		Name:        name,
		Description: description,
		Required:    false,
		Satisfied:   anyEnvSet(lookup, envVars...),
		ValueType:   "env",
		Source:      "environment",
		Secret:      true,
		EnvVars:     append([]string(nil), envVars...),
	}
}

func envVarInfo(name, category string, lookup EnvLookup) EnvVarInfo {
	_, ok := lookup(name)
	return EnvVarInfo{Name: name, Set: ok, Redacted: true, Category: category}
}

func envVarInfosFromMap(values map[string]string) []EnvVarInfo {
	infos := make([]EnvVarInfo, 0, len(values))
	for name := range values {
		infos = append(infos, EnvVarInfo{Name: name, Set: true, Redacted: true, Category: "custom"})
	}
	slices.SortFunc(infos, func(a, b EnvVarInfo) int { return strings.Compare(a.Name, b.Name) })
	return infos
}

func anyEnvSet(lookup EnvLookup, names ...string) bool {
	for _, name := range names {
		if value, ok := lookup(name); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func lookupFirst(lookup EnvLookup, names ...string) string {
	for _, name := range names {
		if value, ok := lookup(name); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, " ", "-")
	return key
}

func normalizeValue(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage("null")
	}
	return append(json.RawMessage(nil), value...)
}

func marshalRaw(value any) json.RawMessage {
	out, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return out
}

func isSecretKey(key string) bool {
	key = strings.ToLower(key)
	for _, marker := range []string{"secret", "token", "password", "api_key", "apikey", "credential"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func descriptionForKey(key string) string {
	switch key {
	case "workspace.root":
		return "Workspace root used for scoped filesystem and process operations."
	case "provider.default":
		return "Default provider id for new turns when a request does not specify one."
	case "reasoning.effort":
		return "Default reasoning effort for provider-neutral model controls."
	case "mutation.approvals.required":
		return "Whether mutation surfaces require explicit approval by default."
	case "appserver.config.version":
		return "Configuration service schema version."
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mergeEnvMaps(left, right map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		out[key] = value
	}
	return out
}

func cloneRequirements(in []Requirement) []Requirement {
	out := append([]Requirement(nil), in...)
	for i := range out {
		out[i].EnvVars = append([]string(nil), out[i].EnvVars...)
	}
	return out
}

func cloneEnvironment(in Environment) Environment {
	in.Variables = append([]EnvVarInfo(nil), in.Variables...)
	return in
}

func cloneEnvironments(in []Environment) []Environment {
	out := make([]Environment, 0, len(in))
	for _, env := range in {
		out = append(out, cloneEnvironment(env))
	}
	return out
}

func clonePermissionProfiles(in []PermissionProfile) []PermissionProfile {
	out := append([]PermissionProfile(nil), in...)
	for i := range out {
		out[i].Capabilities = append([]string(nil), out[i].Capabilities...)
	}
	return out
}

func ensureService(s *Service) *Service {
	if s != nil {
		return s
	}
	return NewService()
}
