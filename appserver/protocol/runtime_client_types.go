package protocol

import (
	"encoding/json"
	"time"
)

// ProviderCatalogEntry is Gollem's provider-neutral live catalog record.
// It remains distinct from provider implementation request payloads.
type ProviderCatalogEntry struct {
	ID              string                      `json:"id"`
	Name            string                      `json:"name"`
	Package         string                      `json:"package"`
	Description     string                      `json:"description"`
	Configured      bool                        `json:"configured"`
	Hidden          bool                        `json:"hidden"`
	DefaultModelID  string                      `json:"defaultModelId"`
	RequiredEnvVars []string                    `json:"requiredEnvVars,omitempty"`
	OptionalEnvVars []string                    `json:"optionalEnvVars,omitempty"`
	AuthModes       []string                    `json:"authModes,omitempty"`
	Capabilities    ProviderCatalogCapabilities `json:"capabilities"`
	Models          []ModelCatalogEntry         `json:"models,omitempty"`
}

type ProviderCatalogCapabilities struct {
	ProviderID               string   `json:"providerId,omitempty"`
	Configured               bool     `json:"configured"`
	NamespaceTools           bool     `json:"namespaceTools"`
	ImageGeneration          bool     `json:"imageGeneration"`
	WebSearch                bool     `json:"webSearch"`
	ToolCalls                bool     `json:"toolCalls"`
	StructuredOutput         bool     `json:"structuredOutput"`
	Vision                   bool     `json:"vision"`
	Streaming                bool     `json:"streaming"`
	PromptCache              bool     `json:"promptCache"`
	ToolSearch               bool     `json:"toolSearch"`
	Reasoning                bool     `json:"reasoning"`
	ReasoningEfforts         []string `json:"reasoningEfforts,omitempty"`
	ReasoningSummaries       bool     `json:"reasoningSummaries,omitempty"`
	AdaptiveThinking         bool     `json:"adaptiveThinking,omitempty"`
	ManualThinking           bool     `json:"manualThinking,omitempty"`
	RequiresConfigurationEnv []string `json:"requiresConfigurationEnv,omitempty"`
}

type ModelCatalogCapabilities struct {
	ToolCalls        bool `json:"toolCalls"`
	StructuredOutput bool `json:"structuredOutput"`
	Vision           bool `json:"vision"`
	Streaming        bool `json:"streaming"`
	PromptCache      bool `json:"promptCache"`
	ToolSearch       bool `json:"toolSearch"`
	Reasoning        bool `json:"reasoning"`
}

type ModelCatalogReasoningEffortOption struct {
	ReasoningEffort string `json:"reasoningEffort"`
	Description     string `json:"description"`
}

type ModelCatalogServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ModelCatalogUpgradeInfo struct {
	Model             string  `json:"model"`
	UpgradeCopy       *string `json:"upgradeCopy"`
	ModelLink         *string `json:"modelLink"`
	MigrationMarkdown *string `json:"migrationMarkdown"`
}

type ModelCatalogAvailabilityNux struct {
	Message string `json:"message"`
}

// ModelCatalogEntry extends the exact standalone Model value with Gollem
// provider identity, executable capabilities, and runtime token limits.
type ModelCatalogEntry struct {
	ID                        string                              `json:"id"`
	ProviderID                string                              `json:"providerId"`
	Model                     string                              `json:"model"`
	Upgrade                   *string                             `json:"upgrade"`
	UpgradeInfo               *ModelCatalogUpgradeInfo            `json:"upgradeInfo"`
	AvailabilityNux           *ModelCatalogAvailabilityNux        `json:"availabilityNux"`
	DisplayName               string                              `json:"displayName"`
	Description               string                              `json:"description"`
	Hidden                    bool                                `json:"hidden"`
	SupportedReasoningEfforts []ModelCatalogReasoningEffortOption `json:"supportedReasoningEfforts"`
	DefaultReasoningEffort    string                              `json:"defaultReasoningEffort"`
	InputModalities           []string                            `json:"inputModalities"`
	SupportsPersonality       bool                                `json:"supportsPersonality"`
	AdditionalSpeedTiers      []string                            `json:"additionalSpeedTiers"`
	ServiceTiers              []ModelCatalogServiceTier           `json:"serviceTiers"`
	DefaultServiceTier        *string                             `json:"defaultServiceTier"`
	IsDefault                 bool                                `json:"isDefault"`
	Capabilities              ModelCatalogCapabilities            `json:"capabilities"`
	MaxContextTokens          int                                 `json:"maxContextTokens,omitempty"`
	MaxOutputTokens           int                                 `json:"maxOutputTokens,omitempty"`
}

type ProviderListParams struct {
	IncludeHidden  *bool `json:"includeHidden,omitempty"`
	ConfiguredOnly bool  `json:"configuredOnly,omitempty"`
}

type ProviderListResponse struct {
	Data      []ProviderCatalogEntry `json:"data"`
	Providers []ProviderCatalogEntry `json:"providers"`
}

type ModelCatalogListParams struct {
	Cursor        *string  `json:"cursor,omitempty"`
	Limit         *uint32  `json:"limit,omitempty"`
	IncludeHidden *bool    `json:"includeHidden,omitempty"`
	ProviderID    string   `json:"providerId,omitempty"`
	ProviderIDs   []string `json:"providerIds,omitempty"`
}

type ModelCatalogListResponse struct {
	Data       []ModelCatalogEntry `json:"data"`
	NextCursor *string             `json:"nextCursor"`
}

type RuntimeModelParams struct {
	ProviderID       string          `json:"providerId,omitempty"`
	Provider         string          `json:"provider,omitempty"`
	Model            string          `json:"model,omitempty"`
	MaxTokens        *int            `json:"maxTokens,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"topP,omitempty"`
	ThinkingBudget   *int            `json:"thinkingBudget,omitempty"`
	AdaptiveThinking *bool           `json:"adaptiveThinking,omitempty"`
	ReasoningEffort  *string         `json:"reasoningEffort,omitempty"`
	StopSequences    []string        `json:"stopSequences,omitempty"`
	Settings         map[string]any  `json:"settings,omitempty"`
	Input            json.RawMessage `json:"input,omitempty"`
}

type ThreadRunStartParams struct {
	Title     string         `json:"title,omitempty"`
	Workspace string         `json:"workspace,omitempty"`
	Prompt    string         `json:"prompt,omitempty"`
	Message   string         `json:"message,omitempty"`
	Text      string         `json:"text,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	RuntimeModelParams
}

type ThreadRunStartResult struct {
	Thread ThreadRecord `json:"thread"`
	Turn   TurnRecord   `json:"turn"`
}

// ThreadHistoryRollbackParams is Gollem's typed history-only rollback request.
// The legacy id alias remains available for protocol-v1 clients, while typed
// clients use the required threadId field.
type ThreadHistoryRollbackParams struct {
	ThreadID string `json:"threadId"`
	NumTurns int    `json:"numTurns"`
	ID       string `json:"id,omitempty"`
}

func (p ThreadHistoryRollbackParams) EffectiveThreadID() string {
	if p.ThreadID != "" {
		return p.ThreadID
	}
	return p.ID
}

// ThreadHistoryRollbackRecord preserves the existing rollback response shape,
// including the legacy nullable name alias and fully loaded remaining turns.
type ThreadHistoryRollbackRecord struct {
	ID                 string                `json:"id"`
	Title              string                `json:"title,omitempty"`
	Workspace          string                `json:"workspace,omitempty"`
	Status             ThreadLifecycleStatus `json:"status"`
	ForkedFromThreadID string                `json:"forkedFromThreadId,omitempty"`
	Settings           map[string]any        `json:"settings,omitempty"`
	Metadata           map[string]any        `json:"metadata,omitempty"`
	CreatedAt          time.Time             `json:"createdAt"`
	UpdatedAt          time.Time             `json:"updatedAt"`
	ArchivedAt         time.Time             `json:"archivedAt,omitempty"`
	DeletedAt          time.Time             `json:"deletedAt,omitempty"`
	Name               *string               `json:"name"`
	Turns              []TurnRecord          `json:"turns" jsonschema:"nonnullable=true"`
}

// ThreadHistoryRollbackResult is explicit that this operation prunes durable
// history only. It never claims to undo workspace, process, Git, or provider
// side effects.
type ThreadHistoryRollbackResult struct {
	Thread                   ThreadHistoryRollbackRecord `json:"thread"`
	RemovedTurnIDs           []string                    `json:"removedTurnIds" jsonschema:"nonnullable=true"`
	Marker                   TimelineItem                `json:"marker"`
	WorkspaceEffectsReverted bool                        `json:"workspaceEffectsReverted"`
}

type TurnRunStartParams struct {
	ID       string         `json:"id,omitempty"`
	ThreadID string         `json:"threadId,omitempty"`
	Prompt   string         `json:"prompt,omitempty"`
	Message  string         `json:"message,omitempty"`
	Text     string         `json:"text,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	RuntimeModelParams
}

type TurnRunStartResult struct {
	Thread ThreadRecord `json:"thread"`
	Turn   TurnRecord   `json:"turn"`
}

// TurnRunRetryParams is Gollem's exact idempotent retry request. Legacy
// untyped clients may omit idempotencyKey at the handler boundary, but typed
// clients must supply one so a lost response cannot duplicate model work.
type TurnRunRetryParams struct {
	TurnID         string         `json:"turnId"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Prompt         string         `json:"prompt,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	RuntimeModelParams
}

type TurnRunRetryResult struct {
	Turn           TurnRecord `json:"turn"`
	SourceTurnID   string     `json:"sourceTurnId"`
	IdempotencyKey string     `json:"idempotencyKey"`
	Reused         bool       `json:"reused"`
}

type TurnRunInterruptParams struct {
	ID       string `json:"id,omitempty"`
	TurnID   string `json:"turnId,omitempty"`
	ThreadID string `json:"threadId,omitempty"`
}

type TurnRunInterruptResult struct {
	OK     bool        `json:"ok"`
	TurnID string      `json:"turnId"`
	Turn   *TurnRecord `json:"turn,omitempty"`
}

type RuntimeThreadNotification struct {
	ThreadID string                `json:"threadId"`
	Status   ThreadLifecycleStatus `json:"status,omitempty"`
	Thread   *ThreadRecord         `json:"thread,omitempty"`
	At       time.Time             `json:"at"`
}

type RuntimeTurnNotification struct {
	ThreadID string              `json:"threadId"`
	TurnID   string              `json:"turnId"`
	Status   TurnLifecycleStatus `json:"status,omitempty"`
	Turn     *TurnRecord         `json:"turn,omitempty"`
	At       time.Time           `json:"at"`
}

type RuntimeDeltaNotification struct {
	ThreadID string    `json:"threadId"`
	TurnID   string    `json:"turnId"`
	Delta    string    `json:"delta"`
	Index    int       `json:"index,omitempty"`
	At       time.Time `json:"at"`
}
