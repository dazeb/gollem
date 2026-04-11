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

// PoolChangeEventType is the string tag returned by
// PoolChangeEvent.RuntimeEventType.
const PoolChangeEventType = "toolproxy.pool_change"

// ModeReason identifies why a given PrepareFuncFor call decided to
// defer (or not defer) tools. Matches the named reasons Claude Code
// emits on its tengu_tool_search_mode_decision event so downstream
// dashboards can share a vocabulary across projects.
type ModeReason string

const (
	// ReasonAlwaysDefer — Mode is ModeAlways; deferral always kicks in.
	ReasonAlwaysDefer ModeReason = "always_defer"
	// ReasonAutoAboveThreshold — ModeAuto and deferred-tool size met or
	// exceeded the configured threshold.
	ReasonAutoAboveThreshold ModeReason = "auto_above_threshold"
	// ReasonAutoBelowThreshold — ModeAuto and deferred-tool size was
	// under the threshold, so the proxy passed the full list through.
	ReasonAutoBelowThreshold ModeReason = "auto_below_threshold"
	// ReasonModeOff — Mode is ModeOff; proxy is a no-op for this request.
	ReasonModeOff ModeReason = "mode_off"
)

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
	HasMatches         bool
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
// based on threshold), the named reason, and the inventory at decision
// time. Useful for observability: you can see deferral kicking in/out
// as the tool catalog grows or shrinks.
type ModeDecisionEvent struct {
	RunID             string
	ParentRunID       string
	Mode              Mode
	Reason            ModeReason
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

// PoolChangeEvent is published when SystemPromptFuncFor detects that
// the set of announceable deferred tools has changed between calls.
// Reports additions and removals so subscribers can log churn or
// invalidate downstream caches. The Initial flag distinguishes the
// first full announcement of a run from an incremental delta —
// useful for telemetry that wants to count runs separately from
// mid-run catalog changes.
type PoolChangeEvent struct {
	RunID       string
	ParentRunID string
	ToolName    string
	Added       []string
	Removed     []string
	Initial     bool
	OccurredAt  time.Time
}

// RuntimeEventType implements the core.RuntimeEvent interface.
func (e PoolChangeEvent) RuntimeEventType() string { return PoolChangeEventType }

// RuntimeRunID implements the core.RuntimeEvent interface.
func (e PoolChangeEvent) RuntimeRunID() string { return e.RunID }

// RuntimeParentRunID implements the core.RuntimeEvent interface.
func (e PoolChangeEvent) RuntimeParentRunID() string { return e.ParentRunID }

// RuntimeOccurredAt implements the core.RuntimeEvent interface.
func (e PoolChangeEvent) RuntimeOccurredAt() time.Time { return e.OccurredAt }

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

// publishPoolChange is the pool-change equivalent of
// publishSearchOutcome. Same nil-safety contract.
func publishPoolChange(rc *core.RunContext, event PoolChangeEvent) {
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
