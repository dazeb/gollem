package agui

import (
	"iter"

	"github.com/fugue-labs/gollem/core"
)

type streamPartKind uint8

const (
	streamPartKindUnknown streamPartKind = iota
	streamPartKindText
	streamPartKindReasoning
)

type streamPartState struct {
	kind      streamPartKind
	messageID string
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
	activeTextIndex := -1
	activeReasoningIndex := -1

	for event, err := range events {
		if err != nil {
			closeAllStreamParts(adapter, parts, &activeTextIndex, &activeReasoningIndex)
			return err
		}
		if event == nil {
			continue
		}

		switch ev := event.(type) {
		case core.PartStartEvent:
			switch part := ev.Part.(type) {
			case core.TextPart:
				state := ensureStreamPart(adapter, parts, ev.Index, streamPartKindText, &activeTextIndex, &activeReasoningIndex)
				if adapter != nil && part.Content != "" {
					adapter.EmitTextDelta(state.messageID, part.Content)
				}
			case core.ThinkingPart:
				state := ensureStreamPart(adapter, parts, ev.Index, streamPartKindReasoning, &activeTextIndex, &activeReasoningIndex)
				if adapter != nil && part.Content != "" {
					adapter.EmitReasoningDelta(state.messageID, part.Content)
				}
			}

		case core.PartDeltaEvent:
			switch delta := ev.Delta.(type) {
			case core.TextPartDelta:
				state := ensureStreamPart(adapter, parts, ev.Index, streamPartKindText, &activeTextIndex, &activeReasoningIndex)
				if adapter != nil {
					adapter.EmitTextDelta(state.messageID, delta.ContentDelta)
				}
			case core.ThinkingPartDelta:
				state := ensureStreamPart(adapter, parts, ev.Index, streamPartKindReasoning, &activeTextIndex, &activeReasoningIndex)
				if adapter != nil {
					adapter.EmitReasoningDelta(state.messageID, delta.ContentDelta)
				}
			}

		case core.PartEndEvent:
			closeStreamPart(adapter, parts, ev.Index, &activeTextIndex, &activeReasoningIndex)
		}
	}

	closeAllStreamParts(adapter, parts, &activeTextIndex, &activeReasoningIndex)
	return nil
}

func ensureStreamPart(
	adapter *Adapter,
	parts map[int]streamPartState,
	index int,
	kind streamPartKind,
	activeTextIndex *int,
	activeReasoningIndex *int,
) streamPartState {
	if state, ok := parts[index]; ok {
		if state.kind == kind {
			activateStreamPart(adapter, parts, index, kind, activeTextIndex, activeReasoningIndex)
			return state
		}
		closeStreamPart(adapter, parts, index, activeTextIndex, activeReasoningIndex)
	}

	activateStreamPart(adapter, parts, index, kind, activeTextIndex, activeReasoningIndex)
	state := streamPartState{
		kind:      kind,
		messageID: nextStreamMessageID(adapter),
	}
	parts[index] = state
	return state
}

func activateStreamPart(
	adapter *Adapter,
	parts map[int]streamPartState,
	index int,
	kind streamPartKind,
	activeTextIndex *int,
	activeReasoningIndex *int,
) {
	switch kind {
	case streamPartKindText:
		if *activeTextIndex != -1 && *activeTextIndex != index {
			closeStreamPart(adapter, parts, *activeTextIndex, activeTextIndex, activeReasoningIndex)
		}
		*activeTextIndex = index
	case streamPartKindReasoning:
		if *activeReasoningIndex != -1 && *activeReasoningIndex != index {
			closeStreamPart(adapter, parts, *activeReasoningIndex, activeTextIndex, activeReasoningIndex)
		}
		*activeReasoningIndex = index
	}
}

func closeStreamPart(
	adapter *Adapter,
	parts map[int]streamPartState,
	index int,
	activeTextIndex *int,
	activeReasoningIndex *int,
) {
	state, ok := parts[index]
	if !ok {
		return
	}
	delete(parts, index)

	switch state.kind {
	case streamPartKindText:
		if *activeTextIndex == index {
			*activeTextIndex = -1
		}
	case streamPartKindReasoning:
		if *activeReasoningIndex == index {
			*activeReasoningIndex = -1
		}
	}

	emitStreamPartEnd(adapter, state)
}

func closeAllStreamParts(
	adapter *Adapter,
	parts map[int]streamPartState,
	activeTextIndex *int,
	activeReasoningIndex *int,
) {
	for index := range parts {
		closeStreamPart(adapter, parts, index, activeTextIndex, activeReasoningIndex)
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
