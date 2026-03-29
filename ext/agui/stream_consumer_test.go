package agui

import (
	"encoding/json"
	"errors"
	"iter"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestConsumeStream_TextAndReasoningLifecycle(t *testing.T) {
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	collector := collectEvents(adapter)

	events := func(yield func(core.ModelResponseStreamEvent, error) bool) {
		if !yield(core.PartStartEvent{Index: 0, Part: core.ThinkingPart{}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 0, Delta: core.ThinkingPartDelta{ContentDelta: "think-1"}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 0, Delta: core.ThinkingPartDelta{ContentDelta: "think-2"}}, nil) {
			return
		}
		if !yield(core.PartEndEvent{Index: 0}, nil) {
			return
		}
		if !yield(core.PartStartEvent{Index: 1, Part: core.TextPart{}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 1, Delta: core.TextPartDelta{ContentDelta: "hello"}}, nil) {
			return
		}
		yield(core.PartEndEvent{Index: 1}, nil)
	}

	if err := ConsumeStream(adapter, events); err != nil {
		fatalUnexpectedErr(t, err)
	}
	adapter.Close()

	raw := collector.all()
	if len(raw) != 9 {
		t.Fatalf("event count = %d, want 9", len(raw))
	}

	types := make([]string, len(raw))
	for i, ev := range raw {
		types[i] = parseType(ev)
	}

	wantTypes := []string{
		AGUIReasoningStart,
		AGUIReasoningMessageStart,
		AGUIReasoningMessageContent,
		AGUIReasoningMessageContent,
		AGUIReasoningMessageEnd,
		AGUIReasoningEnd,
		AGUITextMessageStart,
		AGUITextMessageContent,
		AGUITextMessageEnd,
	}
	for i := range wantTypes {
		if types[i] != wantTypes[i] {
			t.Fatalf("event[%d] type = %q, want %q", i, types[i], wantTypes[i])
		}
	}

	reasonID := parseField(raw[0], "messageId")
	if reasonID == nil || reasonID == "" {
		t.Fatal("reasoning messageId should be non-empty")
	}
	for _, idx := range []int{1, 2, 3, 4, 5} {
		if got := parseField(raw[idx], "messageId"); got != reasonID {
			t.Fatalf("reasoning event[%d] messageId = %v, want %v", idx, got, reasonID)
		}
	}

	textID := parseField(raw[6], "messageId")
	if textID == nil || textID == "" {
		t.Fatal("text messageId should be non-empty")
	}
	for _, idx := range []int{7, 8} {
		if got := parseField(raw[idx], "messageId"); got != textID {
			t.Fatalf("text event[%d] messageId = %v, want %v", idx, got, textID)
		}
	}
	if textID == reasonID {
		t.Fatal("text and reasoning parts should not reuse the same messageId")
	}
}

func TestConsumeStream_StableIDsAcrossTurnsAndPartIndices(t *testing.T) {
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	collector := collectEvents(adapter)
	bus := core.NewEventBus()
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1})
	if err := ConsumeStream(adapter, func(yield func(core.ModelResponseStreamEvent, error) bool) {
		yield(core.PartDeltaEvent{Index: 0, Delta: core.TextPartDelta{ContentDelta: "first"}}, nil)
		yield(core.PartEndEvent{Index: 0}, nil)
	}); err != nil {
		fatalUnexpectedErr(t, err)
	}
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1})

	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 2})
	if err := ConsumeStream(adapter, func(yield func(core.ModelResponseStreamEvent, error) bool) {
		yield(core.PartDeltaEvent{Index: 0, Delta: core.TextPartDelta{ContentDelta: "second"}}, nil)
		yield(core.PartEndEvent{Index: 0}, nil)
	}); err != nil {
		fatalUnexpectedErr(t, err)
	}
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 2})
	adapter.Close()

	raw := collector.all()
	var textStarts []json.RawMessage
	for _, ev := range raw {
		if parseType(ev) == AGUITextMessageStart {
			textStarts = append(textStarts, ev)
		}
	}
	if len(textStarts) != 2 {
		t.Fatalf("text start count = %d, want 2", len(textStarts))
	}

	firstID := parseField(textStarts[0], "messageId")
	secondID := parseField(textStarts[1], "messageId")
	if firstID == nil || secondID == nil || firstID == "" || secondID == "" {
		t.Fatal("text message IDs should be non-empty")
	}
	if firstID == secondID {
		t.Fatalf("message IDs reused across turns: %v", firstID)
	}
}

func TestConsumeStream_PropagatesIteratorErrorAndClosesActiveParts(t *testing.T) {
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	collector := collectEvents(adapter)
	wantErr := errors.New("boom")

	err := ConsumeStream(adapter, func(yield func(core.ModelResponseStreamEvent, error) bool) {
		if !yield(core.PartDeltaEvent{Index: 0, Delta: core.TextPartDelta{ContentDelta: "partial"}}, nil) {
			return
		}
		yield(nil, wantErr)
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	adapter.Close()

	raw := collector.all()
	if len(raw) != 3 {
		t.Fatalf("event count = %d, want 3", len(raw))
	}
	if got := parseType(raw[0]); got != AGUITextMessageStart {
		t.Fatalf("event[0] type = %q, want %q", got, AGUITextMessageStart)
	}
	if got := parseType(raw[1]); got != AGUITextMessageContent {
		t.Fatalf("event[1] type = %q, want %q", got, AGUITextMessageContent)
	}
	if got := parseType(raw[2]); got != AGUITextMessageEnd {
		t.Fatalf("event[2] type = %q, want %q", got, AGUITextMessageEnd)
	}
}

func TestConsumeStream_InterleavedTextPartsBufferUntilPartEnd(t *testing.T) {
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	collector := collectEvents(adapter)

	err := ConsumeStream(adapter, func(yield func(core.ModelResponseStreamEvent, error) bool) {
		if !yield(core.PartStartEvent{Index: 0, Part: core.TextPart{}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 0, Delta: core.TextPartDelta{ContentDelta: "hello"}}, nil) {
			return
		}
		if !yield(core.PartStartEvent{Index: 1, Part: core.TextPart{}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 1, Delta: core.TextPartDelta{ContentDelta: " queued"}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 0, Delta: core.TextPartDelta{ContentDelta: " world"}}, nil) {
			return
		}
		if !yield(core.PartEndEvent{Index: 0}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 1, Delta: core.TextPartDelta{ContentDelta: "!"}}, nil) {
			return
		}
		yield(core.PartEndEvent{Index: 1}, nil)
	})
	if err != nil {
		fatalUnexpectedErr(t, err)
	}
	adapter.Close()

	raw := collector.all()
	wantTypes := []string{
		AGUITextMessageStart,
		AGUITextMessageContent,
		AGUITextMessageContent,
		AGUITextMessageEnd,
		AGUITextMessageStart,
		AGUITextMessageContent,
		AGUITextMessageContent,
		AGUITextMessageEnd,
	}
	if len(raw) != len(wantTypes) {
		t.Fatalf("event count = %d, want %d", len(raw), len(wantTypes))
	}
	for i, want := range wantTypes {
		if got := parseType(raw[i]); got != want {
			t.Fatalf("event[%d] type = %q, want %q", i, got, want)
		}
	}

	firstID := parseField(raw[0], "messageId")
	secondID := parseField(raw[4], "messageId")
	if firstID == nil || secondID == nil || firstID == "" || secondID == "" {
		t.Fatal("text message IDs should be non-empty")
	}
	if firstID == secondID {
		t.Fatalf("interleaved text parts reused message ID %v", firstID)
	}
	for _, idx := range []int{1, 2, 3} {
		if got := parseField(raw[idx], "messageId"); got != firstID {
			t.Fatalf("first text event[%d] messageId = %v, want %v", idx, got, firstID)
		}
	}
	for _, idx := range []int{5, 6, 7} {
		if got := parseField(raw[idx], "messageId"); got != secondID {
			t.Fatalf("second text event[%d] messageId = %v, want %v", idx, got, secondID)
		}
	}
	if got := parseField(raw[5], "delta"); got != " queued" {
		t.Fatalf("buffered text delta = %v, want %q", got, " queued")
	}
	if got := parseField(raw[6], "delta"); got != "!" {
		t.Fatalf("post-activation text delta = %v, want %q", got, "!")
	}
}

func TestConsumeStream_InterleavedReasoningPartsBufferUntilPartEnd(t *testing.T) {
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	collector := collectEvents(adapter)

	err := ConsumeStream(adapter, func(yield func(core.ModelResponseStreamEvent, error) bool) {
		if !yield(core.PartStartEvent{Index: 0, Part: core.ThinkingPart{}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 0, Delta: core.ThinkingPartDelta{ContentDelta: "alpha"}}, nil) {
			return
		}
		if !yield(core.PartStartEvent{Index: 1, Part: core.ThinkingPart{}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 1, Delta: core.ThinkingPartDelta{ContentDelta: " queued"}}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 0, Delta: core.ThinkingPartDelta{ContentDelta: " beta"}}, nil) {
			return
		}
		if !yield(core.PartEndEvent{Index: 0}, nil) {
			return
		}
		if !yield(core.PartDeltaEvent{Index: 1, Delta: core.ThinkingPartDelta{ContentDelta: "!"}}, nil) {
			return
		}
		yield(core.PartEndEvent{Index: 1}, nil)
	})
	if err != nil {
		fatalUnexpectedErr(t, err)
	}
	adapter.Close()

	raw := collector.all()
	wantTypes := []string{
		AGUIReasoningStart,
		AGUIReasoningMessageStart,
		AGUIReasoningMessageContent,
		AGUIReasoningMessageContent,
		AGUIReasoningMessageEnd,
		AGUIReasoningEnd,
		AGUIReasoningStart,
		AGUIReasoningMessageStart,
		AGUIReasoningMessageContent,
		AGUIReasoningMessageContent,
		AGUIReasoningMessageEnd,
		AGUIReasoningEnd,
	}
	if len(raw) != len(wantTypes) {
		t.Fatalf("event count = %d, want %d", len(raw), len(wantTypes))
	}
	for i, want := range wantTypes {
		if got := parseType(raw[i]); got != want {
			t.Fatalf("event[%d] type = %q, want %q", i, got, want)
		}
	}

	firstID := parseField(raw[0], "messageId")
	secondID := parseField(raw[6], "messageId")
	if firstID == nil || secondID == nil || firstID == "" || secondID == "" {
		t.Fatal("reasoning message IDs should be non-empty")
	}
	if firstID == secondID {
		t.Fatalf("interleaved reasoning parts reused message ID %v", firstID)
	}
	for _, idx := range []int{1, 2, 3, 4, 5} {
		if got := parseField(raw[idx], "messageId"); got != firstID {
			t.Fatalf("first reasoning event[%d] messageId = %v, want %v", idx, got, firstID)
		}
	}
	for _, idx := range []int{7, 8, 9, 10, 11} {
		if got := parseField(raw[idx], "messageId"); got != secondID {
			t.Fatalf("second reasoning event[%d] messageId = %v, want %v", idx, got, secondID)
		}
	}
	if got := parseField(raw[8], "delta"); got != " queued" {
		t.Fatalf("buffered reasoning delta = %v, want %q", got, " queued")
	}
	if got := parseField(raw[9], "delta"); got != "!" {
		t.Fatalf("post-activation reasoning delta = %v, want %q", got, "!")
	}
}

func fatalUnexpectedErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

var _ iter.Seq2[core.ModelResponseStreamEvent, error]
