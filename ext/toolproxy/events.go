package toolproxy

import (
	"time"

	"github.com/fugue-labs/gollem/core"
)

// SearchOutcomeEventType is the string tag returned by
// SearchOutcomeEvent.RuntimeEventType.
const SearchOutcomeEventType = "toolproxy.search_outcome"

// ModeDecisionEventType is the string tag returned by
// ModeDecisionEvent.RuntimeEventType.
const ModeDecisionEventType = "toolproxy.mode_decision"

// SearchQueryKind indicates how a tool_search call was interpreted.
type SearchQueryKind string

const (
	// SearchQueryKindSelect is a direct multi-select (`select:A,B,C`).
	SearchQueryKindSelect SearchQueryKind = "select"
	// SearchQueryKindKeyword is a scored keyword search.
	SearchQueryKindKeyword SearchQueryKind = "keyword"
)

// SearchOutcomeEvent is published every time tool_search runs. It
// reports the query, how many matches were returned, and metadata
// useful for dashboards (TotalDeferredTools, MaxResults). Subscribers
// can reconstruct search-quality metrics — match rate, average match
// count per call, how often selections exceed max_results, etc.
type SearchOutcomeEvent struct {
	RunID              string
	ParentRunID        string
	ToolName           string
	Query              string
	QueryKind          SearchQueryKind
	MatchCount         int
	TotalDeferredTools int
	MaxResults         int
	OccurredAt         time.Time
}

// RuntimeEventType implements the core.RuntimeEvent interface.
func (e SearchOutcomeEvent) RuntimeEventType() string { return SearchOutcomeEventType }

// RuntimeRunID implements the core.RuntimeEvent interface.
func (e SearchOutcomeEvent) RuntimeRunID() string { return e.RunID }

// RuntimeParentRunID implements the core.RuntimeEvent interface.
func (e SearchOutcomeEvent) RuntimeParentRunID() string { return e.ParentRunID }

// RuntimeOccurredAt implements the core.RuntimeEvent interface.
func (e SearchOutcomeEvent) RuntimeOccurredAt() time.Time { return e.OccurredAt }

// ModeDecisionEvent is published every time PrepareFuncFor decides
// whether to defer tools for a given request. It reports the effective
// mode, whether deferral actually happened (in auto mode this can flip
// based on threshold), and the inventory at decision time. Useful for
// observability: you can see deferral kicking in/out as the tool
// catalog grows or shrinks.
type ModeDecisionEvent struct {
	RunID             string
	ParentRunID       string
	Mode              Mode
	Deferred          bool
	DeferredToolCount int
	InlineToolCount   int
	EstimatedTokens   int // only populated when Mode == ModeAuto
	Threshold         int // only populated when Mode == ModeAuto
	OccurredAt        time.Time
}

// RuntimeEventType implements the core.RuntimeEvent interface.
func (e ModeDecisionEvent) RuntimeEventType() string { return ModeDecisionEventType }

// RuntimeRunID implements the core.RuntimeEvent interface.
func (e ModeDecisionEvent) RuntimeRunID() string { return e.RunID }

// RuntimeParentRunID implements the core.RuntimeEvent interface.
func (e ModeDecisionEvent) RuntimeParentRunID() string { return e.ParentRunID }

// RuntimeOccurredAt implements the core.RuntimeEvent interface.
func (e ModeDecisionEvent) RuntimeOccurredAt() time.Time { return e.OccurredAt }

// publishSearchOutcome is a nil-safe helper that sends a
// SearchOutcomeEvent on the given RunContext's EventBus. If either is
// nil, it is a no-op.
func publishSearchOutcome(rc *core.RunContext, event SearchOutcomeEvent) {
	if rc == nil || rc.EventBus == nil {
		return
	}
	if event.RunID == "" {
		event.RunID = rc.RunID
	}
	if event.ParentRunID == "" {
		event.ParentRunID = rc.ParentRunID
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	core.Publish(rc.EventBus, event)
}

// publishModeDecision is the mode-decision equivalent of
// publishSearchOutcome. Same nil-safety contract.
func publishModeDecision(rc *core.RunContext, event ModeDecisionEvent) {
	if rc == nil || rc.EventBus == nil {
		return
	}
	if event.RunID == "" {
		event.RunID = rc.RunID
	}
	if event.ParentRunID == "" {
		event.ParentRunID = rc.ParentRunID
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	core.Publish(rc.EventBus, event)
}
