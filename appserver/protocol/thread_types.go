package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ThreadLifecycleStatus is Gollem's durable persistence lifecycle. It is
// intentionally distinct from Codex's runtime-oriented ThreadStatus.
type ThreadLifecycleStatus string

const (
	ThreadLifecycleActive   ThreadLifecycleStatus = "active"
	ThreadLifecycleArchived ThreadLifecycleStatus = "archived"
	ThreadLifecycleDeleted  ThreadLifecycleStatus = "deleted"
)

// TurnLifecycleStatus is Gollem's durable turn execution lifecycle.
type TurnLifecycleStatus string

const (
	TurnLifecycleQueued      TurnLifecycleStatus = "queued"
	TurnLifecycleRunning     TurnLifecycleStatus = "running"
	TurnLifecycleCompleted   TurnLifecycleStatus = "completed"
	TurnLifecycleFailed      TurnLifecycleStatus = "failed"
	TurnLifecycleInterrupted TurnLifecycleStatus = "interrupted"
)

type SortDirection string

const (
	SortDirectionAsc  SortDirection = "asc"
	SortDirectionDesc SortDirection = "desc"
)

type ThreadSortKey string

const (
	ThreadSortCreatedAt ThreadSortKey = "created_at"
	ThreadSortUpdatedAt ThreadSortKey = "updated_at"
	ThreadSortRecencyAt ThreadSortKey = "recency_at"
)

type ThreadSourceKind string

const (
	ThreadSourceCLI             ThreadSourceKind = "cli"
	ThreadSourceVSCode          ThreadSourceKind = "vscode"
	ThreadSourceExec            ThreadSourceKind = "exec"
	ThreadSourceAppServer       ThreadSourceKind = "appServer"
	ThreadSourceSubAgent        ThreadSourceKind = "subAgent"
	ThreadSourceSubAgentReview  ThreadSourceKind = "subAgentReview"
	ThreadSourceSubAgentCompact ThreadSourceKind = "subAgentCompact"
	ThreadSourceSubAgentSpawn   ThreadSourceKind = "subAgentThreadSpawn"
	ThreadSourceSubAgentOther   ThreadSourceKind = "subAgentOther"
	ThreadSourceUnknown         ThreadSourceKind = "unknown"
)

// ThreadListCwdFilter preserves the public string-or-array wire union.
type ThreadListCwdFilter struct {
	paths []string
	many  bool
}

func NewThreadListCwdFilter(path string) ThreadListCwdFilter {
	return ThreadListCwdFilter{paths: []string{path}}
}

func NewThreadListCwdFilters(paths []string) ThreadListCwdFilter {
	cloned := make([]string, len(paths))
	copy(cloned, paths)
	return ThreadListCwdFilter{paths: cloned, many: true}
}

func (f ThreadListCwdFilter) Paths() []string {
	out := make([]string, len(f.paths))
	copy(out, f.paths)
	return out
}

func (f ThreadListCwdFilter) MarshalJSON() ([]byte, error) {
	if f.many {
		return json.Marshal(f.paths)
	}
	if len(f.paths) == 0 {
		return json.Marshal("")
	}
	return json.Marshal(f.paths[0])
}

func (f *ThreadListCwdFilter) UnmarshalJSON(data []byte) error {
	if f == nil {
		return errors.New("thread list cwd filter is nil")
	}
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		f.paths = []string{one}
		f.many = false
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("thread list cwd must be a string or string array: %w", err)
	}
	f.paths = make([]string, len(many))
	copy(f.paths, many)
	f.many = true
	return nil
}

// ThreadListParams accepts the public Codex discovery filters plus Gollem's
// explicit durable-lifecycle compatibility filters.
type ThreadListParams struct {
	Cursor         *string                 `json:"cursor,omitempty"`
	Limit          *uint32                 `json:"limit,omitempty"`
	SortKey        *ThreadSortKey          `json:"sortKey,omitempty"`
	SortDirection  *SortDirection          `json:"sortDirection,omitempty"`
	ModelProviders []string                `json:"modelProviders,omitempty"`
	SourceKinds    []ThreadSourceKind      `json:"sourceKinds,omitempty"`
	Archived       *bool                   `json:"archived,omitempty"`
	CWD            *ThreadListCwdFilter    `json:"cwd,omitempty"`
	UseStateDBOnly bool                    `json:"useStateDbOnly,omitempty"`
	SearchTerm     *string                 `json:"searchTerm,omitempty"`
	Statuses       []ThreadLifecycleStatus `json:"statuses,omitempty"`
	IncludeDeleted bool                    `json:"includeDeleted,omitempty"`
}

// ThreadRecord is Gollem's exported durable thread record. The status field is
// persistence lifecycle, not Codex runtime ThreadStatus.
type ThreadRecord struct {
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
	Turns              []TurnRecord          `json:"turns,omitempty" jsonschema:"nonnullable=true"`
}

type TurnRecord struct {
	ID          string              `json:"id"`
	ThreadID    string              `json:"threadId"`
	Status      TurnLifecycleStatus `json:"status"`
	Input       json.RawMessage     `json:"input,omitempty"`
	Result      json.RawMessage     `json:"result,omitempty"`
	Error       string              `json:"error,omitempty"`
	Usage       map[string]any      `json:"usage,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	StartedAt   time.Time           `json:"startedAt,omitempty"`
	CompletedAt time.Time           `json:"completedAt,omitempty"`
	Items       []TimelineItem      `json:"items,omitempty" jsonschema:"nonnullable=true"`
}

// ThreadListResponse adds Codex-compatible data/cursor fields while retaining
// the legacy Gollem threads field for protocol-v1 compatibility.
type ThreadListResponse struct {
	Data            []ThreadRecord `json:"data" jsonschema:"nonnullable=true"`
	NextCursor      *string        `json:"nextCursor,omitempty"`
	BackwardsCursor *string        `json:"backwardsCursor,omitempty"`
	Threads         []ThreadRecord `json:"threads" jsonschema:"nonnullable=true,optional=true"`
}

// ThreadReadParams keeps only threadId/includeTurns in the public contract;
// the remaining fields are named Gollem protocol-v1 extensions.
type ThreadReadParams struct {
	ThreadID     string `json:"threadId"`
	IncludeTurns *bool  `json:"includeTurns,omitempty" jsonschema:"nonnullable=true"`
	ID           string `json:"id,omitempty"`
	IncludeItems *bool  `json:"includeItems,omitempty"`
	AfterSeq     int64  `json:"afterSeq,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

func (p ThreadReadParams) EffectiveThreadID() string {
	if p.ThreadID != "" {
		return p.ThreadID
	}
	return p.ID
}

// ThreadReadResponse retains protocol-v1 top-level turns/items while also
// nesting loaded turns/items in Thread for forward client consumption.
type ThreadReadResponse struct {
	Thread ThreadRecord   `json:"thread"`
	Turns  []TurnRecord   `json:"turns,omitempty" jsonschema:"nonnullable=true"`
	Items  []TimelineItem `json:"items,omitempty" jsonschema:"nonnullable=true"`
}
