package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	ItemTypeDynamicToolCall   = "dynamicToolCall"
	ItemTypeCommandExecution  = "commandExecution"
	ItemTypeFileChange        = "fileChange"
	ItemTypeMCPToolCall       = "mcpToolCall"
	ItemTypeContextCompaction = "contextCompaction"

	ItemStatusInProgress = "inProgress"
	ItemStatusCompleted  = "completed"
	ItemStatusFailed     = "failed"
	ItemStatusDeclined   = "declined"
)

type CommandExecutionStatus string

const (
	CommandExecutionStatusInProgress CommandExecutionStatus = ItemStatusInProgress
	CommandExecutionStatusCompleted  CommandExecutionStatus = ItemStatusCompleted
	CommandExecutionStatusFailed     CommandExecutionStatus = ItemStatusFailed
	CommandExecutionStatusDeclined   CommandExecutionStatus = ItemStatusDeclined
)

type DynamicToolCallStatus string

const (
	DynamicToolCallStatusInProgress DynamicToolCallStatus = ItemStatusInProgress
	DynamicToolCallStatusCompleted  DynamicToolCallStatus = ItemStatusCompleted
	DynamicToolCallStatusFailed     DynamicToolCallStatus = ItemStatusFailed
)

type McpToolCallStatus string

const (
	McpToolCallStatusInProgress McpToolCallStatus = ItemStatusInProgress
	McpToolCallStatusCompleted  McpToolCallStatus = ItemStatusCompleted
	McpToolCallStatusFailed     McpToolCallStatus = ItemStatusFailed
)

// TimelineItem is the durable item envelope returned by thread item APIs.
// Payload contains one of the concrete item types below when Kind is known.
type TimelineItem struct {
	ID           string          `json:"id"`
	ThreadID     string          `json:"threadId"`
	TurnID       string          `json:"turnId,omitempty"`
	ParentItemID string          `json:"parentItemId,omitempty"`
	Seq          int64           `json:"seq"`
	Kind         string          `json:"kind"`
	Status       string          `json:"status,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type DynamicToolCallItem struct {
	Type         string                       `json:"type" jsonschema:"enum=dynamicToolCall"`
	ID           string                       `json:"id,omitempty"`
	Namespace    *string                      `json:"namespace"`
	Tool         string                       `json:"tool"`
	Arguments    any                          `json:"arguments"`
	Status       string                       `json:"status" jsonschema:"enum=inProgress|completed|failed"`
	ContentItems []DynamicToolCallContentItem `json:"contentItems"`
	Success      *bool                        `json:"success"`
	DurationMS   *int64                       `json:"durationMs"`
}

type DynamicToolCallContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolPayloadSummary struct {
	Omitted bool   `json:"omitted"`
	Reason  string `json:"reason"`
	Bytes   int    `json:"bytes"`
	SHA256  string `json:"sha256"`
}

type DynamicToolCallItemStartedNotificationParams struct {
	Item        DynamicToolCallItem `json:"item"`
	ThreadID    string              `json:"threadId"`
	TurnID      string              `json:"turnId"`
	StartedAtMS int64               `json:"startedAtMs"`
}

type DynamicToolCallItemCompletedNotificationParams struct {
	Item          DynamicToolCallItem `json:"item"`
	ThreadID      string              `json:"threadId"`
	TurnID        string              `json:"turnId"`
	CompletedAtMS int64               `json:"completedAtMs"`
}

type CommandExecutionItem struct {
	Type             string                   `json:"type" jsonschema:"enum=commandExecution"`
	ID               string                   `json:"id,omitempty"`
	Command          string                   `json:"command"`
	CWD              string                   `json:"cwd"`
	ProcessID        *string                  `json:"processId"`
	Source           string                   `json:"source" jsonschema:"enum=agent|userShell"`
	Status           string                   `json:"status" jsonschema:"enum=inProgress|completed|failed|declined"`
	CommandActions   []CommandExecutionAction `json:"commandActions"`
	AggregatedOutput *string                  `json:"aggregatedOutput"`
	ExitCode         *int                     `json:"exitCode"`
	DurationMS       *int64                   `json:"durationMs"`
	StartedAt        time.Time                `json:"startedAt"`
	CompletedAt      *time.Time               `json:"completedAt"`
}

type CommandExecutionAction struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type CommandExecutionItemStartedNotificationParams struct {
	Item        CommandExecutionItem `json:"item"`
	ThreadID    string               `json:"threadId"`
	TurnID      string               `json:"turnId"`
	StartedAtMS int64                `json:"startedAtMs"`
}

type CommandExecutionItemCompletedNotificationParams struct {
	Item          CommandExecutionItem `json:"item"`
	ThreadID      string               `json:"threadId"`
	TurnID        string               `json:"turnId"`
	CompletedAtMS int64                `json:"completedAtMs"`
}

type CommandExecutionOutputDeltaNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

type CommandExecutionOutputDeltaNotificationParams = CommandExecutionOutputDeltaNotification

type PatchApplyStatus string

const (
	PatchApplyStatusInProgress PatchApplyStatus = ItemStatusInProgress
	PatchApplyStatusCompleted  PatchApplyStatus = ItemStatusCompleted
	PatchApplyStatusFailed     PatchApplyStatus = ItemStatusFailed
	PatchApplyStatusDeclined   PatchApplyStatus = ItemStatusDeclined
)

type FileChangeItem struct {
	Type     string                       `json:"type" jsonschema:"enum=fileChange"`
	ID       string                       `json:"id,omitempty"`
	Changes  []FileUpdateChange           `json:"changes"`
	Status   PatchApplyStatus             `json:"status"`
	Evidence []FileChangeArtifactEvidence `json:"evidence,omitempty"`
}

type FileUpdateChange struct {
	Path string          `json:"path"`
	Kind PatchChangeKind `json:"kind"`
	Diff string          `json:"diff"`
}

type PatchChangeKind struct {
	Type     string  `json:"type" jsonschema:"enum=add|delete|update"`
	MovePath *string `json:"-"`
}

func (k PatchChangeKind) MarshalJSON() ([]byte, error) {
	switch k.Type {
	case "add", "delete":
		if k.MovePath != nil {
			return nil, fmt.Errorf("patch change kind %q cannot include a move path", k.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: k.Type})
	case "update":
		return json.Marshal(struct {
			Type           string  `json:"type"`
			PublicMovePath *string `json:"move_path"`
			LegacyMovePath *string `json:"movePath"`
		}{Type: k.Type, PublicMovePath: k.MovePath, LegacyMovePath: k.MovePath})
	default:
		return nil, fmt.Errorf("unknown patch change kind %q", k.Type)
	}
}

func (k *PatchChangeKind) UnmarshalJSON(data []byte) error {
	if k == nil {
		return errors.New("decode patch change kind into nil receiver")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	for name := range payload {
		switch name {
		case "type", "move_path", "movePath":
		default:
			return fmt.Errorf("unknown patch change kind field %q", name)
		}
	}
	typeRaw, ok := payload["type"]
	if !ok {
		return errors.New("patch change kind requires type")
	}
	var kind string
	if err := json.Unmarshal(typeRaw, &kind); err != nil {
		return fmt.Errorf("decode patch change kind type: %w", err)
	}
	publicRaw, hasPublic := payload["move_path"]
	legacyRaw, hasLegacy := payload["movePath"]
	publicMovePath, err := decodeNullableMovePath(publicRaw, hasPublic, "move_path")
	if err != nil {
		return err
	}
	legacyMovePath, err := decodeNullableMovePath(legacyRaw, hasLegacy, "movePath")
	if err != nil {
		return err
	}

	switch kind {
	case "add", "delete":
		if hasPublic || hasLegacy {
			return fmt.Errorf("patch change kind %q cannot include a move path", kind)
		}
		*k = PatchChangeKind{Type: kind}
		return nil
	case "update":
		if !hasPublic && !hasLegacy {
			return fmt.Errorf("patch change kind %q requires move_path or movePath", kind)
		}
		movePath := legacyMovePath
		if hasPublic {
			movePath = publicMovePath
		}
		*k = PatchChangeKind{Type: kind, MovePath: movePath}
		return nil
	default:
		return fmt.Errorf("unknown patch change kind %q", kind)
	}
}

func decodeNullableMovePath(raw json.RawMessage, present bool, name string) (*string, error) {
	if !present {
		return nil, nil
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode patch change kind %s: %w", name, err)
	}
	return value, nil
}

type FileChangeArtifactEvidence struct {
	Path                 string `json:"path"`
	Operation            string `json:"operation"`
	Bytes                int64  `json:"bytes"`
	BeforeSHA256         string `json:"beforeSha256,omitempty"`
	AfterSHA256          string `json:"afterSha256,omitempty"`
	DiffTruncated        bool   `json:"diffTruncated,omitempty"`
	DiffOmittedReason    string `json:"diffOmittedReason,omitempty"`
	ContentEncoding      string `json:"contentEncoding,omitempty"`
	ContentTruncated     bool   `json:"contentTruncated,omitempty"`
	ContentOmittedReason string `json:"contentOmittedReason,omitempty"`
}

type FileChangeItemStartedNotificationParams struct {
	Item        FileChangeItem `json:"item"`
	ThreadID    string         `json:"threadId"`
	TurnID      string         `json:"turnId"`
	StartedAtMS int64          `json:"startedAtMs"`
}

type FileChangeItemCompletedNotificationParams struct {
	Item          FileChangeItem `json:"item"`
	ThreadID      string         `json:"threadId"`
	TurnID        string         `json:"turnId"`
	CompletedAtMS int64          `json:"completedAtMs"`
}

type FileChangePatchUpdatedNotification struct {
	ThreadID string             `json:"threadId"`
	TurnID   string             `json:"turnId"`
	ItemID   string             `json:"itemId"`
	Changes  []FileUpdateChange `json:"changes" jsonschema:"nonnullable=true"`
}

func (n FileChangePatchUpdatedNotification) MarshalJSON() ([]byte, error) {
	type notification FileChangePatchUpdatedNotification
	if n.Changes == nil {
		n.Changes = []FileUpdateChange{}
	}
	return json.Marshal(notification(n))
}

type FileChangePatchUpdatedNotificationParams = FileChangePatchUpdatedNotification

type MCPToolCallItem struct {
	Type              string             `json:"type" jsonschema:"enum=mcpToolCall"`
	ID                string             `json:"id,omitempty"`
	Server            string             `json:"server"`
	Tool              string             `json:"tool"`
	Status            string             `json:"status" jsonschema:"enum=inProgress|completed|failed"`
	Arguments         any                `json:"arguments"`
	AppContext        any                `json:"appContext"`
	MCPAppResourceURI *string            `json:"mcpAppResourceUri"`
	PluginID          *string            `json:"pluginId"`
	Result            *MCPToolCallResult `json:"result"`
	Error             *McpToolCallError  `json:"error"`
	DurationMS        *int64             `json:"durationMs"`
}

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type MCPToolCallResult struct {
	Content           []MCPContent `json:"content"`
	StructuredContent any          `json:"structuredContent"`
	Meta              any          `json:"_meta"`
}

type McpToolCallError struct {
	Message string `json:"message"`
}

type MCPToolCallError = McpToolCallError

type MCPToolCallItemStartedNotificationParams struct {
	Item        MCPToolCallItem `json:"item"`
	ThreadID    string          `json:"threadId"`
	TurnID      string          `json:"turnId"`
	StartedAtMS int64           `json:"startedAtMs"`
}

type MCPToolCallItemCompletedNotificationParams struct {
	Item          MCPToolCallItem `json:"item"`
	ThreadID      string          `json:"threadId"`
	TurnID        string          `json:"turnId"`
	CompletedAtMS int64           `json:"completedAtMs"`
}

type McpToolCallProgressNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Message  string `json:"message"`
}

type MCPToolCallProgressNotificationParams = McpToolCallProgressNotification

type ThreadCompactStartParams struct {
	ID       string `json:"id,omitempty"`
	ThreadID string `json:"threadId,omitempty"`
}

type ThreadCompactStartResponse struct{}

type ContextCompactionItem struct {
	Type      string    `json:"type" jsonschema:"enum=contextCompaction"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type ContextCompactedNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type DeprecationNoticeNotification struct {
	Summary string  `json:"summary" jsonschema:"description=Concise summary of what is deprecated."`
	Details *string `json:"details" jsonschema:"description=Optional extra guidance such as migration steps or rationale."`
}

type ThreadTokenUsage struct {
	Total              TokenUsageBreakdown `json:"total"`
	Last               TokenUsageBreakdown `json:"last"`
	ModelContextWindow *int64              `json:"modelContextWindow"`
}

type TokenUsageBreakdown struct {
	TotalTokens           int64 `json:"totalTokens"`
	InputTokens           int64 `json:"inputTokens"`
	CachedInputTokens     int64 `json:"cachedInputTokens"`
	OutputTokens          int64 `json:"outputTokens"`
	ReasoningOutputTokens int64 `json:"reasoningOutputTokens"`
}

type ThreadTokenUsageUpdatedNotification struct {
	ThreadID   string           `json:"threadId"`
	TurnID     string           `json:"turnId"`
	TokenUsage ThreadTokenUsage `json:"tokenUsage"`
}

type TurnDiffUpdatedNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Diff     string `json:"diff"`
}

// Gollem v1 compatibility aliases. New bindings use the exact public names.
type ThreadCompactedNotificationParams = ContextCompactedNotification
type ThreadTokenUsageUpdatedNotificationParams = ThreadTokenUsageUpdatedNotification
type TokenUsage = ThreadTokenUsage
type TurnDiffUpdatedNotificationParams = TurnDiffUpdatedNotification

type ItemLifecycleNotificationParams struct {
	ThreadID string        `json:"threadId"`
	TurnID   string        `json:"turnId,omitempty"`
	ItemID   string        `json:"itemId,omitempty"`
	Item     *TimelineItem `json:"item,omitempty"`
	At       time.Time     `json:"at"`
}

type ApprovalRequestBase struct {
	ThreadID    string `json:"threadId"`
	TurnID      string `json:"turnId"`
	ItemID      string `json:"itemId"`
	StartedAtMS int64  `json:"startedAtMs"`
	Reason      string `json:"reason,omitempty"`
}

type FileChangeApprovalRequestParams struct {
	ApprovalRequestBase
	GrantRoot   *string `json:"grantRoot"`
	Operation   string  `json:"operation,omitempty"`
	Path        string  `json:"path,omitempty"`
	Destination string  `json:"destination,omitempty"`
	Destructive bool    `json:"destructive,omitempty"`
}

type FileChangeApprovalDecision string

const (
	FileChangeApprovalAccept           FileChangeApprovalDecision = "accept"
	FileChangeApprovalAcceptForSession FileChangeApprovalDecision = "acceptForSession"
	FileChangeApprovalDecline          FileChangeApprovalDecision = "decline"
	FileChangeApprovalCancel           FileChangeApprovalDecision = "cancel"
)

type FileChangeRequestApprovalResponse struct {
	Decision FileChangeApprovalDecision `json:"decision"`
}

type CommandExecutionApprovalRequestParams struct {
	ApprovalRequestBase
	ApprovalID         string                             `json:"approvalId,omitempty"`
	Command            string                             `json:"command,omitempty"`
	Args               []string                           `json:"args,omitempty"`
	CWD                string                             `json:"cwd,omitempty"`
	Operation          string                             `json:"operation,omitempty"`
	Signal             string                             `json:"signal,omitempty"`
	Destructive        bool                               `json:"destructive,omitempty"`
	CommandActions     []CommandExecutionAction           `json:"commandActions,omitempty"`
	AvailableDecisions []CommandExecutionApprovalDecision `json:"availableDecisions,omitempty" jsonschema:"nonnullable=true"`
}

type PermissionsApprovalRequestParams struct {
	ApprovalRequestBase
	CWD         string         `json:"cwd"`
	Operation   string         `json:"operation,omitempty"`
	Path        string         `json:"path,omitempty"`
	Branch      string         `json:"branch,omitempty"`
	Base        string         `json:"base,omitempty"`
	Message     string         `json:"message,omitempty"`
	Pathspecs   []string       `json:"pathspecs,omitempty"`
	Permissions map[string]any `json:"permissions"`
}

type ApprovalRespondParams struct {
	RequestID string `json:"requestId"`
	ID        string `json:"id,omitempty"`
	Approved  bool   `json:"approved"`
	Message   string `json:"message,omitempty"`
}

type ApprovalRespondResult struct {
	OK        bool   `json:"ok"`
	RequestID string `json:"requestId"`
	Approved  bool   `json:"approved"`
}

type ServerRequestResolvedNotification struct {
	ThreadID  string    `json:"threadId"`
	RequestID RequestId `json:"requestId"`
}

type ServerRequestResolvedNotificationParams = ServerRequestResolvedNotification

type DaemonShutdownState struct {
	Requested bool   `json:"requested"`
	Restart   bool   `json:"restart"`
	Reason    string `json:"reason,omitempty"`
}

type DaemonStatus struct {
	Status            string              `json:"status" jsonschema:"enum=running|stopping"`
	Name              string              `json:"name"`
	Version           string              `json:"version"`
	ProtocolVersion   string              `json:"protocolVersion"`
	PID               int                 `json:"pid"`
	StartedAt         time.Time           `json:"startedAt"`
	UptimeMillis      int64               `json:"uptimeMillis"`
	Transport         string              `json:"transport,omitempty"`
	WorkDir           string              `json:"workDir,omitempty"`
	StorePath         string              `json:"storePath,omitempty"`
	ShutdownRequested bool                `json:"shutdownRequested"`
	RestartRequested  bool                `json:"restartRequested"`
	Shutdown          DaemonShutdownState `json:"shutdown,omitempty"`
}

type DaemonVersion struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	ProtocolVersion string `json:"protocolVersion"`
	GoVersion       string `json:"goVersion"`
	GOOS            string `json:"goos"`
	GOARCH          string `json:"goarch"`
}

type DaemonStartResult struct {
	OK             bool         `json:"ok"`
	AlreadyRunning bool         `json:"alreadyRunning"`
	Status         DaemonStatus `json:"status"`
}

type DaemonStopResult struct {
	OK       bool         `json:"ok"`
	Stopping bool         `json:"stopping"`
	Restart  bool         `json:"restart"`
	Status   DaemonStatus `json:"status"`
}

type DaemonShutdownParams struct {
	Reason string `json:"reason,omitempty"`
}
