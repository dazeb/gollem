package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExactLiveNotificationSchemasAndBindings(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"ContextCompactedNotification",
		"DeprecationNoticeNotification",
		"ThreadTokenUsage",
		"ThreadTokenUsageUpdatedNotification",
		"TurnDiffUpdatedNotification",
		// Keep the Gollem v1 names available to existing generated clients.
		"ThreadCompactedNotificationParams",
		"ThreadTokenUsageUpdatedNotificationParams",
		"TokenUsage",
		"TurnDiffUpdatedNotificationParams",
	} {
		if _, ok := defs[name]; !ok {
			t.Errorf("schema missing %s", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	assertSchemaRequired(t, defs["ContextCompactedNotification"].(Schema), "threadId", "turnId")
	assertSchemaRequired(t, defs["DeprecationNoticeNotification"].(Schema), "summary", "details")
	assertSchemaRequired(t, defs["ThreadTokenUsage"].(Schema), "total", "last", "modelContextWindow")
	assertSchemaRequired(t, defs["ThreadTokenUsageUpdatedNotification"].(Schema), "threadId", "turnId", "tokenUsage")
	assertSchemaRequired(t, defs["TurnDiffUpdatedNotification"].(Schema), "threadId", "turnId", "diff")
	assertExactLiveNotificationNullableField(t, defs["DeprecationNoticeNotification"].(Schema), "details")
	assertExactLiveNotificationNullableField(t, defs["ThreadTokenUsage"].(Schema), "modelContextWindow")
	tokenUsageProperty := defs["ThreadTokenUsageUpdatedNotification"].(Schema)["properties"].(Schema)["tokenUsage"].(Schema)
	if tokenUsageProperty["$ref"] != "#/$defs/ThreadTokenUsage" {
		t.Fatalf("notification tokenUsage schema = %#v", tokenUsageProperty)
	}
	for alias, canonical := range map[string]string{
		"ThreadCompactedNotificationParams":         "ContextCompactedNotification",
		"ThreadTokenUsageUpdatedNotificationParams": "ThreadTokenUsageUpdatedNotification",
		"TokenUsage":                        "ThreadTokenUsage",
		"TurnDiffUpdatedNotificationParams": "TurnDiffUpdatedNotification",
	} {
		if got := defs[alias].(Schema)["$ref"]; got != "#/$defs/"+canonical {
			t.Errorf("%s alias = %#v", alias, defs[alias])
		}
	}

	wantBindings := map[string]string{
		"deprecationNotice":         "DeprecationNoticeNotification",
		"thread/compacted":          "ContextCompactedNotification",
		"thread/tokenUsage/updated": "ThreadTokenUsageUpdatedNotification",
		"turn/diff/updated":         "TurnDiffUpdatedNotification",
	}
	for method, want := range wantBindings {
		binding, ok := exactLiveNotificationBinding(WireTypeBindings(), method)
		if !ok {
			t.Errorf("missing %s binding", method)
			continue
		}
		if binding.Surface != SurfaceServerNotification || !reflect.DeepEqual(binding.Params, []string{want}) || len(binding.Result) != 0 {
			t.Errorf("%s binding = %+v, want server-notification params [%s]", method, binding, want)
		}
	}
}

func TestExactLiveNotificationValuesPreserveWireShape(t *testing.T) {
	usage := ThreadTokenUsage{
		Total: TokenUsageBreakdown{
			TotalTokens:           180,
			InputTokens:           120,
			CachedInputTokens:     20,
			OutputTokens:          60,
			ReasoningOutputTokens: 10,
		},
		Last: TokenUsageBreakdown{
			TotalTokens:           80,
			InputTokens:           50,
			CachedInputTokens:     10,
			OutputTokens:          30,
			ReasoningOutputTokens: 5,
		},
		ModelContextWindow: nil,
	}
	values := []struct {
		name string
		got  any
		want string
	}{
		{
			name: "context compacted",
			got:  ContextCompactedNotification{ThreadID: "thread-1", TurnID: "turn-1"},
			want: `{"threadId":"thread-1","turnId":"turn-1"}`,
		},
		{
			name: "deprecation notice",
			got:  DeprecationNoticeNotification{Summary: "deprecated", Details: nil},
			want: `{"summary":"deprecated","details":null}`,
		},
		{
			name: "thread token usage",
			got:  usage,
			want: `{"total":{"totalTokens":180,"inputTokens":120,"cachedInputTokens":20,"outputTokens":60,"reasoningOutputTokens":10},"last":{"totalTokens":80,"inputTokens":50,"cachedInputTokens":10,"outputTokens":30,"reasoningOutputTokens":5},"modelContextWindow":null}`,
		},
		{
			name: "thread token usage updated",
			got:  ThreadTokenUsageUpdatedNotification{ThreadID: "thread-1", TurnID: "turn-1", TokenUsage: usage},
			want: `{"threadId":"thread-1","turnId":"turn-1","tokenUsage":{"total":{"totalTokens":180,"inputTokens":120,"cachedInputTokens":20,"outputTokens":60,"reasoningOutputTokens":10},"last":{"totalTokens":80,"inputTokens":50,"cachedInputTokens":10,"outputTokens":30,"reasoningOutputTokens":5},"modelContextWindow":null}}`,
		},
		{
			name: "turn diff updated",
			got:  TurnDiffUpdatedNotification{ThreadID: "thread-1", TurnID: "turn-1", Diff: "diff\n"},
			want: `{"threadId":"thread-1","turnId":"turn-1","diff":"diff\n"}`,
		},
	}
	for _, tc := range values {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.got)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("wire = %s, want %s", got, tc.want)
			}
		})
	}

	for legacy, canonical := range map[reflect.Type]reflect.Type{
		reflect.TypeFor[ThreadCompactedNotificationParams]():         reflect.TypeFor[ContextCompactedNotification](),
		reflect.TypeFor[ThreadTokenUsageUpdatedNotificationParams](): reflect.TypeFor[ThreadTokenUsageUpdatedNotification](),
		reflect.TypeFor[TokenUsage]():                                reflect.TypeFor[ThreadTokenUsage](),
		reflect.TypeFor[TurnDiffUpdatedNotificationParams]():         reflect.TypeFor[TurnDiffUpdatedNotification](),
	} {
		if legacy != canonical {
			t.Errorf("legacy type %s is not an alias of %s", legacy, canonical)
		}
	}
}

func exactLiveNotificationBinding(bindings []WireTypeBinding, method string) (WireTypeBinding, bool) {
	for _, binding := range bindings {
		if binding.Method == method {
			return binding, true
		}
	}
	return WireTypeBinding{}, false
}

func assertExactLiveNotificationNullableField(t *testing.T, definition Schema, field string) {
	t.Helper()
	properties, ok := definition["properties"].(Schema)
	if !ok {
		t.Fatalf("definition properties = %T", definition["properties"])
	}
	property, ok := properties[field].(Schema)
	if !ok {
		t.Fatalf("%s schema = %T", field, properties[field])
	}
	variants, ok := property["anyOf"].([]any)
	if !ok || len(variants) != 2 {
		t.Fatalf("%s nullable variants = %#v", field, property["anyOf"])
	}
	for _, variant := range variants {
		if schema, ok := variant.(Schema); ok && schema["type"] == "null" {
			return
		}
	}
	t.Fatalf("%s is not nullable: %#v", field, property)
}
