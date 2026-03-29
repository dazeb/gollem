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

const noActiveStreamPart = -1

type streamPartState struct {
	kind          streamPartKind
	messageID     string
	sawStart      bool
	emitted       bool
	closed        bool
	queued        bool
	pendingDeltas []string
}

type streamKindState struct {
	activeIndex int
	queue       []int
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
	parts := make(map[int]*streamPartState)
	kinds := map[streamPartKind]*streamKindState{
		streamPartKindText:      {activeIndex: noActiveStreamPart},
		streamPartKindReasoning: {activeIndex: noActiveStreamPart},
	}

	for event, err := range events {
		syncStreamPartsWithAdapter(adapter, parts, kinds)

		if err != nil {
			syncStreamPartsWithAdapter(adapter, parts, kinds)
			closeAllStreamParts(adapter, parts, kinds)
			return err
		}
		if event == nil {
			continue
		}

		switch ev := event.(type) {
		case core.PartStartEvent:
			switch part := ev.Part.(type) {
			case core.TextPart:
				state := ensureStreamPart(adapter, parts, kinds, ev.Index, streamPartKindText)
				state.sawStart = true
				if part.Content != "" {
					appendStreamPartDelta(adapter, parts, kinds, ev.Index, streamPartKindText, part.Content)
				}
			case core.ThinkingPart:
				state := ensureStreamPart(adapter, parts, kinds, ev.Index, streamPartKindReasoning)
				state.sawStart = true
				if part.Content != "" {
					appendStreamPartDelta(adapter, parts, kinds, ev.Index, streamPartKindReasoning, part.Content)
				}
			}

		case core.PartDeltaEvent:
			switch delta := ev.Delta.(type) {
			case core.TextPartDelta:
				appendStreamPartDelta(adapter, parts, kinds, ev.Index, streamPartKindText, delta.ContentDelta)
			case core.ThinkingPartDelta:
				appendStreamPartDelta(adapter, parts, kinds, ev.Index, streamPartKindReasoning, delta.ContentDelta)
			}

		case core.PartEndEvent:
			closeStreamPart(adapter, parts, kinds, ev.Index)
		}
	}

	syncStreamPartsWithAdapter(adapter, parts, kinds)
	closeAllStreamParts(adapter, parts, kinds)
	return nil
}

func ensureStreamPart(
	adapter *Adapter,
	parts map[int]*streamPartState,
	kinds map[streamPartKind]*streamKindState,
	index int,
	kind streamPartKind,
) *streamPartState {
	if state, ok := parts[index]; ok {
		if state.kind == kind {
			return state
		}
		closeAllStreamParts(adapter, parts, kinds)
	}

	state := &streamPartState{
		kind:      kind,
		messageID: nextStreamMessageID(adapter),
	}
	parts[index] = state

	kindState := getStreamKindState(kinds, kind)
	if kindState.activeIndex == noActiveStreamPart {
		kindState.activeIndex = index
	} else if kindState.activeIndex != index {
		kindState.queue = append(kindState.queue, index)
		state.queued = true
	}

	return state
}

func appendStreamPartDelta(
	adapter *Adapter,
	parts map[int]*streamPartState,
	kinds map[streamPartKind]*streamKindState,
	index int,
	kind streamPartKind,
	delta string,
) {
	state := ensureStreamPart(adapter, parts, kinds, index, kind)
	if delta != "" {
		state.pendingDeltas = append(state.pendingDeltas, delta)
	}
	advanceStreamKind(adapter, parts, kinds, kind)
}

func closeStreamPart(
	adapter *Adapter,
	parts map[int]*streamPartState,
	kinds map[streamPartKind]*streamKindState,
	index int,
) {
	state, ok := parts[index]
	if !ok {
		return
	}
	state.closed = true
	advanceStreamKind(adapter, parts, kinds, state.kind)
}

func closeAllStreamParts(
	adapter *Adapter,
	parts map[int]*streamPartState,
	kinds map[streamPartKind]*streamKindState,
) {
	for _, state := range parts {
		state.closed = true
	}
	advanceStreamKind(adapter, parts, kinds, streamPartKindText)
	advanceStreamKind(adapter, parts, kinds, streamPartKindReasoning)
}

func advanceStreamKind(
	adapter *Adapter,
	parts map[int]*streamPartState,
	kinds map[streamPartKind]*streamKindState,
	kind streamPartKind,
) {
	kindState := getStreamKindState(kinds, kind)

	for {
		if kindState.activeIndex == noActiveStreamPart {
			nextIndex, ok := dequeueNextStreamPart(parts, kindState, kind)
			if !ok {
				return
			}
			kindState.activeIndex = nextIndex
		}

		state, ok := parts[kindState.activeIndex]
		if !ok || state.kind != kind {
			kindState.activeIndex = noActiveStreamPart
			continue
		}

		emitStreamPartBufferedDeltas(adapter, state)
		if !state.closed {
			return
		}

		finalizeStreamPart(adapter, *state)
		delete(parts, kindState.activeIndex)
		kindState.activeIndex = noActiveStreamPart
	}
}

func getStreamKindState(
	kinds map[streamPartKind]*streamKindState,
	kind streamPartKind,
) *streamKindState {
	state, ok := kinds[kind]
	if !ok {
		state = &streamKindState{activeIndex: noActiveStreamPart}
		kinds[kind] = state
	}
	return state
}

func dequeueNextStreamPart(
	parts map[int]*streamPartState,
	kindState *streamKindState,
	kind streamPartKind,
) (int, bool) {
	for len(kindState.queue) > 0 {
		index := kindState.queue[0]
		kindState.queue = kindState.queue[1:]

		state, ok := parts[index]
		if !ok || state.kind != kind {
			continue
		}
		state.queued = false
		return index, true
	}
	return 0, false
}

func emitStreamPartBufferedDeltas(adapter *Adapter, state *streamPartState) {
	if len(state.pendingDeltas) == 0 {
		return
	}
	if adapter == nil || state.messageID == "" {
		state.pendingDeltas = nil
		state.emitted = true
		return
	}
	for _, delta := range state.pendingDeltas {
		switch state.kind {
		case streamPartKindText:
			adapter.EmitTextDelta(state.messageID, delta)
		case streamPartKindReasoning:
			adapter.EmitReasoningDelta(state.messageID, delta)
		}
	}
	state.pendingDeltas = nil
	state.emitted = true
}

func nextStreamMessageID(adapter *Adapter) string {
	if adapter == nil {
		return ""
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.nextMessageID()
}

func finalizeStreamPart(adapter *Adapter, state streamPartState) {
	if state.emitted {
		emitStreamPartEnd(adapter, state)
		return
	}
	if state.sawStart {
		emitEmptyStreamPartLifecycle(adapter, state)
	}
}

func emitEmptyStreamPartLifecycle(adapter *Adapter, state streamPartState) {
	if adapter == nil || state.messageID == "" {
		return
	}

	switch state.kind {
	case streamPartKindText:
		adapter.EmitTextDelta(state.messageID, "")
	case streamPartKindReasoning:
		adapter.EmitReasoningDelta(state.messageID, "")
	}

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
		if adapter.activeMessageID != state.messageID {
			adapter.mu.Unlock()
			return
		}
		batch.enqueue(aguiTextMessageEnd{
			Type: AGUITextMessageEnd, Timestamp: ts, MessageID: state.messageID,
		})
		adapter.activeMessageID = ""
	case streamPartKindReasoning:
		if adapter.activeReasoningID != state.messageID {
			adapter.mu.Unlock()
			return
		}
		batch.enqueue(aguiReasoningMessageEnd{
			Type: AGUIReasoningMessageEnd, Timestamp: ts, MessageID: state.messageID,
		})
		batch.enqueue(aguiReasoningEnd{
			Type: AGUIReasoningEnd, Timestamp: ts, MessageID: state.messageID,
		})
		adapter.activeReasoningID = ""
	}

	adapter.mu.Unlock()
	batch.send()
}

func syncStreamPartsWithAdapter(
	adapter *Adapter,
	parts map[int]*streamPartState,
	kinds map[streamPartKind]*streamKindState,
) {
	if adapter == nil || len(parts) == 0 {
		return
	}

	for {
		retired := false
		for index, state := range parts {
			if state == nil || !state.emitted {
				continue
			}
			if streamPartStillActive(adapter, *state) {
				continue
			}
			retireStreamPart(adapter, parts, kinds, index)
			retired = true
			break
		}
		if !retired {
			return
		}
	}
}

func retireStreamPart(
	adapter *Adapter,
	parts map[int]*streamPartState,
	kinds map[streamPartKind]*streamKindState,
	index int,
) {
	state, ok := parts[index]
	if !ok || state == nil {
		return
	}
	delete(parts, index)

	kindState := getStreamKindState(kinds, state.kind)
	if kindState.activeIndex == index {
		kindState.activeIndex = noActiveStreamPart
	} else {
		removeQueuedStreamPart(kindState, index)
	}

	advanceStreamKind(adapter, parts, kinds, state.kind)
}

func removeQueuedStreamPart(kindState *streamKindState, index int) {
	if kindState == nil || len(kindState.queue) == 0 {
		return
	}

	queue := kindState.queue[:0]
	for _, queuedIndex := range kindState.queue {
		if queuedIndex == index {
			continue
		}
		queue = append(queue, queuedIndex)
	}
	kindState.queue = queue
}

func streamPartStillActive(adapter *Adapter, state streamPartState) bool {
	if adapter == nil || state.messageID == "" {
		return false
	}

	adapter.mu.Lock()
	defer adapter.mu.Unlock()

	switch state.kind {
	case streamPartKindText:
		return adapter.activeMessageID == state.messageID
	case streamPartKindReasoning:
		return adapter.activeReasoningID == state.messageID
	default:
		return false
	}
}
