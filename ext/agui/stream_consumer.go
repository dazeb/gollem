package agui

import (
	"iter"
	"sort"

	"github.com/fugue-labs/gollem/core"
)

type streamPartKind uint8

const (
	streamPartKindUnknown streamPartKind = iota
	streamPartKindText
	streamPartKindReasoning
)

type streamPartState struct {
	kind          streamPartKind
	messageID     string
	sawStart      bool
	everStarted   bool
	activeSegment bool
}

// ConsumeStream bridges a RunStream event iterator into AG-UI adapter events.
//
// It consumes the real StreamEvents iterator contract:
//
//	for event, err := range stream.StreamEvents() { ... }
//
// Text and thinking parts are mapped onto the adapter's text/reasoning delta
// emitters, while PartEndEvent boundaries are translated into the matching AG-UI
// end events. Message IDs are allocated once per streamed part and reused for all
// deltas of that part, remaining unique across later parts and turns.
func ConsumeStream(adapter *Adapter, events iter.Seq2[core.ModelResponseStreamEvent, error]) error {
	parts := make(map[int]streamPartState)

	for event, err := range events {
		if err != nil {
			closeAllStreamParts(adapter, parts)
			return err
		}
		if event == nil {
			continue
		}

		switch ev := event.(type) {
		case core.PartStartEvent:
			switch part := ev.Part.(type) {
			case core.TextPart:
				state := ensureStreamPart(adapter, parts, ev.Index, streamPartKindText)
				state.sawStart = true
				parts[ev.Index] = state
				if part.Content != "" {
					emitStreamPartDelta(adapter, parts, ev.Index, streamPartKindText, part.Content)
				}
			case core.ThinkingPart:
				state := ensureStreamPart(adapter, parts, ev.Index, streamPartKindReasoning)
				state.sawStart = true
				parts[ev.Index] = state
				if part.Content != "" {
					emitStreamPartDelta(adapter, parts, ev.Index, streamPartKindReasoning, part.Content)
				}
			}

		case core.PartDeltaEvent:
			switch delta := ev.Delta.(type) {
			case core.TextPartDelta:
				emitStreamPartDelta(adapter, parts, ev.Index, streamPartKindText, delta.ContentDelta)
			case core.ThinkingPartDelta:
				emitStreamPartDelta(adapter, parts, ev.Index, streamPartKindReasoning, delta.ContentDelta)
			}

		case core.PartEndEvent:
			closeStreamPart(adapter, parts, ev.Index)
		}
	}

	closeAllStreamParts(adapter, parts)
	return nil
}

func ensureStreamPart(
	adapter *Adapter,
	parts map[int]streamPartState,
	index int,
	kind streamPartKind,
) streamPartState {
	if state, ok := parts[index]; ok {
		if state.kind == kind {
			return state
		}
		closeStreamPart(adapter, parts, index)
	}

	state := streamPartState{
		kind:      kind,
		messageID: nextStreamMessageID(adapter),
	}
	parts[index] = state
	return state
}

func emitStreamPartDelta(
	adapter *Adapter,
	parts map[int]streamPartState,
	index int,
	kind streamPartKind,
	delta string,
) {
	state := ensureStreamPart(adapter, parts, index, kind)
	if adapter != nil && state.messageID != "" {
		closeActiveStreamPartSegment(adapter, parts, kind, state.messageID)
		switch kind {
		case streamPartKindText:
			adapter.EmitTextDelta(state.messageID, delta)
		case streamPartKindReasoning:
			adapter.EmitReasoningDelta(state.messageID, delta)
		}
		state.everStarted = true
		state.activeSegment = true
	}
	parts[index] = state
}

func closeStreamPart(adapter *Adapter, parts map[int]streamPartState, index int) {
	state, ok := parts[index]
	if !ok {
		return
	}
	delete(parts, index)

	if state.activeSegment {
		emitStreamPartEnd(adapter, state)
		return
	}
	if state.sawStart && !state.everStarted {
		emitEmptyStreamPartLifecycle(adapter, parts, state)
	}
}

func closeAllStreamParts(adapter *Adapter, parts map[int]streamPartState) {
	indices := make([]int, 0, len(parts))
	for index := range parts {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		closeStreamPart(adapter, parts, index)
	}
}

func closeActiveStreamPartSegment(
	adapter *Adapter,
	parts map[int]streamPartState,
	kind streamPartKind,
	keepMessageID string,
) {
	if adapter == nil {
		return
	}

	adapter.mu.Lock()
	batch := adapter.beginEmit()
	var activeMessageID string

	switch kind {
	case streamPartKindText:
		activeMessageID = adapter.activeMessageID
		if activeMessageID != "" && activeMessageID != keepMessageID {
			batch.enqueue(aguiTextMessageEnd{
				Type: AGUITextMessageEnd, Timestamp: nowMillis(), MessageID: activeMessageID,
			})
			adapter.activeMessageID = ""
		}
	case streamPartKindReasoning:
		activeMessageID = adapter.activeReasoningID
		if activeMessageID != "" && activeMessageID != keepMessageID {
			ts := nowMillis()
			batch.enqueue(aguiReasoningMessageEnd{
				Type: AGUIReasoningMessageEnd, Timestamp: ts, MessageID: activeMessageID,
			})
			batch.enqueue(aguiReasoningEnd{
				Type: AGUIReasoningEnd, Timestamp: ts, MessageID: activeMessageID,
			})
			adapter.activeReasoningID = ""
		}
	}

	adapter.mu.Unlock()
	batch.send()

	if activeMessageID == "" || activeMessageID == keepMessageID {
		return
	}
	for partIndex, state := range parts {
		if state.kind == kind && state.messageID == activeMessageID {
			state.activeSegment = false
			parts[partIndex] = state
			return
		}
	}
}

func nextStreamMessageID(adapter *Adapter) string {
	if adapter == nil {
		return ""
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.nextMessageID()
}

func emitEmptyStreamPartLifecycle(
	adapter *Adapter,
	parts map[int]streamPartState,
	state streamPartState,
) {
	if adapter == nil || state.messageID == "" {
		return
	}

	closeActiveStreamPartSegment(adapter, parts, state.kind, state.messageID)

	switch state.kind {
	case streamPartKindText:
		adapter.EmitTextDelta(state.messageID, "")
	case streamPartKindReasoning:
		adapter.EmitReasoningDelta(state.messageID, "")
	}

	state.everStarted = true
	state.activeSegment = true
	emitStreamPartEnd(adapter, state)
}

func emitStreamPartEnd(adapter *Adapter, state streamPartState) {
	if adapter == nil || state.messageID == "" {
		return
	}

	adapter.mu.Lock()
	batch := adapter.beginEmit()
	ts := nowMillis()

	switch state.kind {
	case streamPartKindText:
		batch.enqueue(aguiTextMessageEnd{
			Type: AGUITextMessageEnd, Timestamp: ts, MessageID: state.messageID,
		})
		if adapter.activeMessageID == state.messageID {
			adapter.activeMessageID = ""
		}
	case streamPartKindReasoning:
		batch.enqueue(aguiReasoningMessageEnd{
			Type: AGUIReasoningMessageEnd, Timestamp: ts, MessageID: state.messageID,
		})
		batch.enqueue(aguiReasoningEnd{
			Type: AGUIReasoningEnd, Timestamp: ts, MessageID: state.messageID,
		})
		if adapter.activeReasoningID == state.messageID {
			adapter.activeReasoningID = ""
		}
	}

	adapter.mu.Unlock()
	batch.send()
}
