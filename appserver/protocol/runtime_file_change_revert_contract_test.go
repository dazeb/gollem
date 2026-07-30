package protocol

import (
	"reflect"
	"strings"
	"testing"
)

func TestFileChangeRevertContractIsClosedGeneratedAndBound(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	params := defs["FileChangeRevertParams"].(Schema)
	if params["type"] != "object" || params["additionalProperties"] != false ||
		!reflect.DeepEqual(schemaRequiredNames(params), []string{"threadId", "itemId", "idempotencyKey"}) {
		t.Fatalf("file-change revert params schema = %#v", params)
	}
	result := defs["FileChangeRevertResult"].(Schema)
	if result["type"] != "object" || result["additionalProperties"] != false {
		t.Fatalf("file-change revert result is not closed: %#v", result)
	}
	properties := result["properties"].(Schema)
	if properties["marker"].(Schema)["$ref"] != "#/$defs/TimelineItem" ||
		!reflect.DeepEqual(schemaRequiredNames(result), []string{
			"threadId",
			"turnId",
			"itemId",
			"idempotencyKey",
			"path",
			"beforeExists",
			"afterExists",
			"revertedAt",
			"marker",
			"reused",
		}) {
		t.Fatalf("file-change revert result schema = %#v", result)
	}
	evidence := defs["FileChangeArtifactEvidence"].(Schema)
	evidenceProperties := evidence["properties"].(Schema)
	if _, ok := evidenceProperties["revertSnapshotAvailable"]; !ok {
		t.Fatalf("file-change evidence lacks revert capability: %#v", evidenceProperties)
	}
	if _, ok := evidenceProperties["revertUnavailableReason"]; !ok {
		t.Fatalf("file-change evidence lacks unavailable reason: %#v", evidenceProperties)
	}

	var binding WireTypeBinding
	for _, candidate := range WireTypeBindings() {
		if candidate.Method == "item/fileChange/revert" {
			binding = candidate
			break
		}
	}
	wantBinding := WireTypeBinding{
		Method:  "item/fileChange/revert",
		Surface: SurfaceGollemExtension,
		Params:  []string{"FileChangeRevertParams"},
		Result:  []string{"FileChangeRevertResult"},
	}
	if !reflect.DeepEqual(binding, wantBinding) {
		t.Fatalf("file-change revert binding = %#v, want %#v", binding, wantBinding)
	}
	method, ok := LookupMethod("item/fileChange/revert")
	if !ok || method.State != MethodImplemented || method.Surface != SurfaceGollemExtension {
		t.Fatalf("file-change revert method = %#v, found=%v", method, ok)
	}

	generated, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	source := string(generated)
	for _, want := range []string{
		`export type FileChangeRevertParams = {`,
		`"idempotencyKey": string;`,
		`export type FileChangeRevertResult = {`,
		`"revertSnapshotAvailable"?: boolean;`,
		`"revertUnavailableReason"?: string;`,
		`"item/fileChange/revert": FileChangeRevertParams;`,
		`"item/fileChange/revert": FileChangeRevertResult;`,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("generated TypeScript missing %q", want)
		}
	}
}
