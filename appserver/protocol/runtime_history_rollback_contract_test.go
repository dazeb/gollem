package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestThreadHistoryRollbackContractIsGeneratedAndBound(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)

	params := defs["ThreadHistoryRollbackParams"].(Schema)
	if params["type"] != "object" || params["additionalProperties"] != false {
		t.Fatalf("rollback params are not closed: %#v", params)
	}
	paramProperties := params["properties"].(Schema)
	if len(paramProperties) != 3 ||
		paramProperties["numTurns"].(Schema)["minimum"] != 1 ||
		!reflect.DeepEqual(schemaRequiredNames(params), []string{"threadId", "numTurns"}) {
		t.Fatalf("rollback params schema = %#v", params)
	}

	result := defs["ThreadHistoryRollbackResult"].(Schema)
	if result["type"] != "object" || result["additionalProperties"] != false {
		t.Fatalf("rollback result is not closed: %#v", result)
	}
	resultProperties := result["properties"].(Schema)
	if resultProperties["thread"].(Schema)["$ref"] != "#/$defs/ThreadHistoryRollbackRecord" ||
		resultProperties["marker"].(Schema)["$ref"] != "#/$defs/TimelineItem" ||
		resultProperties["removedTurnIds"].(Schema)["type"] != "array" {
		t.Fatalf("rollback result schema = %#v", result)
	}
	if !reflect.DeepEqual(
		schemaRequiredNames(result),
		[]string{"thread", "removedTurnIds", "marker", "workspaceEffectsReverted"},
	) {
		t.Fatalf("rollback result required = %#v", schemaRequiredNames(result))
	}

	record := defs["ThreadHistoryRollbackRecord"].(Schema)
	recordProperties := record["properties"].(Schema)
	if record["type"] != "object" ||
		record["additionalProperties"] != false ||
		recordProperties["turns"].(Schema)["type"] != "array" {
		t.Fatalf("rollback record schema = %#v", record)
	}

	var binding WireTypeBinding
	for _, candidate := range WireTypeBindings() {
		if candidate.Method == "thread/rollback" {
			binding = candidate
			break
		}
	}
	wantBinding := WireTypeBinding{
		Method:  "thread/rollback",
		Surface: SurfaceClientRequest,
		Params:  []string{"ThreadHistoryRollbackParams"},
		Result:  []string{"ThreadHistoryRollbackResult"},
	}
	if !reflect.DeepEqual(binding, wantBinding) {
		t.Fatalf("thread/rollback binding = %#v, want %#v", binding, wantBinding)
	}

	encoded, err := json.Marshal(ThreadHistoryRollbackParams{
		ThreadID: "thread-1",
		NumTurns: 1,
	})
	if err != nil {
		t.Fatalf("Marshal params: %v", err)
	}
	if string(encoded) != `{"threadId":"thread-1","numTurns":1}` {
		t.Fatalf("encoded params = %s", encoded)
	}
	if got := (ThreadHistoryRollbackParams{ID: "legacy"}).EffectiveThreadID(); got != "legacy" {
		t.Fatalf("legacy effective thread id = %q", got)
	}

	generated, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	source := string(generated)
	for _, want := range []string{
		`export type ThreadHistoryRollbackParams = {`,
		`"numTurns": number;`,
		`export type ThreadHistoryRollbackResult = {`,
		`"removedTurnIds": Array<string>;`,
		`"workspaceEffectsReverted": boolean;`,
		`"thread/rollback": ThreadHistoryRollbackParams;`,
		`"thread/rollback": ThreadHistoryRollbackResult;`,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("generated TypeScript missing %q", want)
		}
	}
}
