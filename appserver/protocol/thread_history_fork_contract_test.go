package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestThreadHistoryForkContractIsGeneratedAndBound(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	params := defs["ThreadHistoryForkParams"].(Schema)
	assertClosedObjectSchema(t, params, "threadId", "idempotencyKey")
	assertRecoverySchemaKeys(t, params, "threadId", "idempotencyKey")

	result := defs["ThreadHistoryForkResult"].(Schema)
	assertClosedObjectSchema(
		t,
		result,
		"thread",
		"sourceThreadId",
		"idempotencyKey",
		"reused",
		"historyCopied",
		"fileChangeRecoveryTransferred",
	)
	assertRecoverySchemaKeys(
		t,
		result,
		"thread",
		"sourceThreadId",
		"idempotencyKey",
		"reused",
		"historyCopied",
		"fileChangeRecoveryTransferred",
	)

	var binding WireTypeBinding
	for _, candidate := range WireTypeBindings() {
		if candidate.Method == "thread/fork" {
			binding = candidate
			break
		}
	}
	if binding.Method == "" {
		t.Fatal("thread/fork binding is missing")
	}
	if binding.Surface != SurfaceClientRequest ||
		!reflect.DeepEqual(binding.Params, []string{"ThreadHistoryForkParams"}) ||
		!reflect.DeepEqual(binding.Result, []string{"ThreadHistoryForkResult"}) {
		t.Fatalf("thread/fork binding = %#v", binding)
	}

	encoded, err := json.Marshal(ThreadHistoryForkParams{
		ThreadID:       "thread-1",
		IdempotencyKey: "fork-1",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != `{"threadId":"thread-1","idempotencyKey":"fork-1"}` {
		t.Fatalf("encoded params = %s", encoded)
	}
	for _, input := range []string{
		`{}`,
		`{"threadId":"thread-1"}`,
		`{"idempotencyKey":"fork-1"}`,
		`{"threadId":null,"idempotencyKey":"fork-1"}`,
		`{"threadId":"thread-1","idempotencyKey":null}`,
		`{"threadId":1,"idempotencyKey":"fork-1"}`,
		`{"threadId":"thread-1","idempotencyKey":1}`,
		`{"threadId":"thread-1","idempotencyKey":"fork-1","includeItems":true}`,
		`{"threadId":"thread-1","threadId":"thread-2","idempotencyKey":"fork-1"}`,
		`{"threadId":"thread-1","idempotencyKey":"fork-1","idempotencyKey":"fork-2"}`,
		`{"threadId":"thread-1","idempotencyKey":"fork-1"} {}`,
	} {
		var params ThreadHistoryForkParams
		if err := json.Unmarshal([]byte(input), &params); err == nil {
			t.Errorf("ThreadHistoryForkParams accepted %s", input)
		}
	}
	var nilParams *ThreadHistoryForkParams
	if err := nilParams.UnmarshalJSON([]byte(`{"threadId":"thread-1","idempotencyKey":"fork-1"}`)); err == nil {
		t.Fatal("nil ThreadHistoryForkParams receiver succeeded")
	}

	generated, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	ts := string(generated)
	for _, want := range []string{
		`export type ThreadHistoryForkParams = {`,
		`"idempotencyKey": string;`,
		`"threadId": string;`,
		`export type ThreadHistoryForkResult = {`,
		`"fileChangeRecoveryTransferred": boolean;`,
		`"historyCopied": boolean;`,
		`"reused": boolean;`,
		`"sourceThreadId": string;`,
		`"thread/fork": ThreadHistoryForkParams;`,
	} {
		if !strings.Contains(ts, want) {
			t.Fatalf("generated TypeScript missing %q", want)
		}
	}
}
